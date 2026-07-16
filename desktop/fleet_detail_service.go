package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const fleetDetailConcurrencyLimit = 3

type FleetDetailOption func(*FleetDetailService)

type FleetDetailService struct {
	connection   *ConnectionService
	now          func() time.Time
	freshnessAge time.Duration
	concurrency  int
}

func NewFleetDetailService(options ...FleetDetailOption) *FleetDetailService {
	service := &FleetDetailService{
		connection:   NewConnectionService(),
		now:          time.Now,
		freshnessAge: defaultWorkspaceFreshnessAge,
		concurrency:  fleetDetailConcurrencyLimit,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithFleetDetailClock(now func() time.Time) FleetDetailOption {
	return func(service *FleetDetailService) {
		if now != nil {
			service.now = now
		}
	}
}

func WithFleetDetailFreshnessThreshold(age time.Duration) FleetDetailOption {
	return func(service *FleetDetailService) {
		if age > 0 {
			service.freshnessAge = age
		}
	}
}

func (s *FleetDetailService) Load(ctx context.Context, profile ConnectionProfile, fleet string) (FleetDetailView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateFleetDetailName(fleet); err != nil {
		return FleetDetailView{}, err
	}
	connected, err := s.connection.connect(ctx, profile)
	if err != nil {
		return FleetDetailView{}, err
	}
	return s.LoadConnected(ctx, connected.client, fleet)
}

func (s *FleetDetailService) LoadConnected(ctx context.Context, client *admin.Client, fleet string) (FleetDetailView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		return FleetDetailView{}, ErrSessionNotConnected
	}
	if err := validateFleetDetailName(fleet); err != nil {
		return FleetDetailView{}, err
	}

	var (
		fleets       []string
		fleetsErr    error
		endpoints    []admin.Endpoint
		endpointsErr error
		report       admin.FleetStateReport
		reportErr    error
	)
	runWorkspaceTasks(ctx, s.concurrency, []func(context.Context){
		func(taskCtx context.Context) {
			fleets, fleetsErr = client.ListFleetsContext(taskCtx)
		},
		func(taskCtx context.Context) {
			endpoints, endpointsErr = client.ListEndpointsContext(taskCtx)
		},
		func(taskCtx context.Context) {
			report, reportErr = client.GetFleetStateReportContext(taskCtx, fleet)
		},
	})
	if cause := context.Cause(ctx); cause != nil {
		return FleetDetailView{}, cause
	}
	if fleetsErr != nil {
		_, classified := classifyWorkspaceSectionError("Fleets", fleetsErr)
		return FleetDetailView{}, classified
	}
	if !slices.Contains(fleets, fleet) {
		return FleetDetailView{}, &ClassifiedError{
			Kind:     ErrorUnavailable,
			Message:  "The selected Fleet is not available from the Remotr server.",
			Guidance: "Return to Fleets and select a currently available Fleet.",
		}
	}
	if endpointsErr != nil {
		_, classified := classifyWorkspaceSectionError("Fleet members", endpointsErr)
		return FleetDetailView{}, classified
	}
	if reportErr == nil && report.Fleet != fleet {
		report = admin.FleetStateReport{}
		reportErr = &ClassifiedError{
			Kind:     ErrorUnexpected,
			Message:  "State evidence did not match the selected Fleet.",
			Guidance: "Return to Fleets and select the Fleet again.",
		}
	}

	members := make([]admin.Endpoint, 0)
	for _, endpoint := range endpoints {
		if endpoint.Fleet == fleet {
			members = append(members, endpoint)
		}
	}
	loadedAt := s.now().UTC()
	reports := []admin.FleetStateReport{}
	if reportErr == nil {
		reports = append(reports, report)
	}
	mappedMembers := mapWorkspaceEndpoints(members, reports, loadedAt, s.freshnessAge)
	summaries := mapWorkspaceFleets([]string{fleet}, members, reports, loadedAt, s.freshnessAge)
	if len(summaries) != 1 {
		return FleetDetailView{}, &ClassifiedError{
			Kind:     ErrorUnexpected,
			Message:  "Fleet detail could not be composed.",
			Guidance: "Return to Fleets and select the Fleet again.",
		}
	}

	view := FleetDetailView{
		Fleet:   fleet,
		Summary: summaries[0],
		Members: mappedMembers,
		Empty:   len(mappedMembers) == 0,
	}
	if view.Empty {
		view.EmptyMessage = "No Endpoints are enrolled in this Fleet."
	}
	view.Sections = FleetDetailSections{
		Members: workspaceSectionResult("Fleet members", nil, len(mappedMembers), loadedAt, latestEndpointObservation(members)),
		State:   workspaceSectionResult("Fleet State evidence", reportErr, len(report.Endpoints), loadedAt, latestStateObservation(reports)),
	}
	return view, nil
}

func validateFleetDetailName(fleet string) error {
	if fleet == "" || fleet != strings.TrimSpace(fleet) {
		return &ClassifiedError{
			Kind:     ErrorValidation,
			Message:  "Select a valid Fleet before loading detail.",
			Guidance: "Return to Fleets and select an exact Fleet name.",
		}
	}
	return nil
}
