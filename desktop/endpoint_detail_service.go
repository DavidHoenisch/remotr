package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const (
	endpointDetailConcurrencyLimit = 4
	endpointDetailStateItemLimit   = 500
	endpointDetailSubresultLimit   = 100
	endpointDetailScheduleLimit    = 500
	endpointDetailFirewallLimit    = 500
	endpointDetailListValueLimit   = 100
)

var ErrObsoleteEndpointDetail = errors.New("obsolete Endpoint detail selection")

type EndpointDetailOption func(*EndpointDetailService)

type EndpointDetailService struct {
	connection   *ConnectionService
	now          func() time.Time
	freshnessAge time.Duration
	concurrency  int

	selectionMu sync.Mutex
	generation  uint64
	cancel      context.CancelCauseFunc
}

func NewEndpointDetailService(options ...EndpointDetailOption) *EndpointDetailService {
	service := &EndpointDetailService{
		connection:   NewConnectionService(),
		now:          time.Now,
		freshnessAge: defaultWorkspaceFreshnessAge,
		concurrency:  endpointDetailConcurrencyLimit,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithEndpointDetailClock(now func() time.Time) EndpointDetailOption {
	return func(service *EndpointDetailService) {
		if now != nil {
			service.now = now
		}
	}
}

func WithEndpointDetailFreshnessThreshold(age time.Duration) EndpointDetailOption {
	return func(service *EndpointDetailService) {
		if age > 0 {
			service.freshnessAge = age
		}
	}
}

func (s *EndpointDetailService) Load(ctx context.Context, profile ConnectionProfile, endpointID string) (EndpointDetailView, error) {
	selectionCtx, generation, cancel := s.beginSelection(ctx)
	if err := validateEndpointDetailID(endpointID); err != nil {
		return s.completeSelection(selectionCtx, generation, cancel, EndpointDetailView{}, err)
	}
	connected, err := s.connection.connect(selectionCtx, profile)
	if err != nil {
		return s.completeSelection(selectionCtx, generation, cancel, EndpointDetailView{}, err)
	}
	view, loadErr := s.loadSelected(selectionCtx, connected.client, endpointID)
	return s.completeSelection(selectionCtx, generation, cancel, view, loadErr)
}

func (s *EndpointDetailService) LoadConnected(ctx context.Context, client *admin.Client, endpointID string) (EndpointDetailView, error) {
	selectionCtx, generation, cancel := s.beginSelection(ctx)
	if client == nil {
		return s.completeSelection(selectionCtx, generation, cancel, EndpointDetailView{}, ErrSessionNotConnected)
	}
	if err := validateEndpointDetailID(endpointID); err != nil {
		return s.completeSelection(selectionCtx, generation, cancel, EndpointDetailView{}, err)
	}
	view, loadErr := s.loadSelected(selectionCtx, client, endpointID)
	return s.completeSelection(selectionCtx, generation, cancel, view, loadErr)
}

func (s *EndpointDetailService) beginSelection(ctx context.Context) (context.Context, uint64, context.CancelCauseFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	selectionCtx, cancel := context.WithCancelCause(ctx)
	s.selectionMu.Lock()
	if s.cancel != nil {
		s.cancel(ErrObsoleteEndpointDetail)
	}
	s.generation++
	generation := s.generation
	s.cancel = cancel
	s.selectionMu.Unlock()
	return selectionCtx, generation, cancel
}

func (s *EndpointDetailService) completeSelection(
	ctx context.Context,
	generation uint64,
	cancel context.CancelCauseFunc,
	view EndpointDetailView,
	err error,
) (EndpointDetailView, error) {
	s.selectionMu.Lock()
	current := generation == s.generation
	if current {
		s.cancel = nil
	}
	s.selectionMu.Unlock()
	if !current {
		cancel(ErrObsoleteEndpointDetail)
		return EndpointDetailView{}, ErrObsoleteEndpointDetail
	}
	if cause := context.Cause(ctx); cause != nil {
		cancel(cause)
		return EndpointDetailView{}, cause
	}
	cancel(nil)
	return view, err
}

func (s *EndpointDetailService) loadSelected(ctx context.Context, client *admin.Client, endpointID string) (EndpointDetailView, error) {
	var (
		endpoint    admin.Endpoint
		endpointErr error
		state       admin.StateReport
		stateErr    error
		schedules   admin.CronReport
		scheduleErr error
		firewall    admin.FirewallAuditReport
		firewallErr error
	)
	runWorkspaceTasks(ctx, s.concurrency, []func(context.Context){
		func(taskCtx context.Context) {
			endpoint, endpointErr = client.GetEndpointContext(taskCtx, endpointID)
		},
		func(taskCtx context.Context) {
			state, stateErr = client.GetEndpointStateReportContext(taskCtx, endpointID)
		},
		func(taskCtx context.Context) {
			schedules, scheduleErr = client.GetEndpointCronReportContext(taskCtx, endpointID)
		},
		func(taskCtx context.Context) {
			firewall, firewallErr = client.GetEndpointFirewallAuditContext(taskCtx, endpointID)
		},
	})
	if cause := context.Cause(ctx); cause != nil {
		return EndpointDetailView{}, cause
	}
	if endpointErr != nil {
		_, classified := classifyWorkspaceSectionError("Endpoint detail", endpointErr)
		return EndpointDetailView{}, classified
	}
	if endpoint.ID != endpointID {
		return EndpointDetailView{}, endpointDetailIdentityFailure("Endpoint record")
	}
	if stateErr == nil && state.EndpointID != endpointID {
		state = admin.StateReport{}
		stateErr = endpointDetailIdentityFailure("State report")
	}
	if scheduleErr == nil && schedules.EndpointID != endpointID {
		schedules = admin.CronReport{}
		scheduleErr = endpointDetailIdentityFailure("Schedule report")
	}
	if firewallErr == nil && firewall.EndpointID != endpointID {
		firewall = admin.FirewallAuditReport{}
		firewallErr = endpointDetailIdentityFailure("Firewall report")
	}

	loadedAt := s.now().UTC()
	reports := []admin.FleetStateReport{}
	if stateErr == nil {
		reports = append(reports, admin.FleetStateReport{Fleet: endpoint.Fleet, Endpoints: []admin.StateReport{state}})
	}
	headerRows := mapWorkspaceEndpoints([]admin.Endpoint{endpoint}, reports, loadedAt, s.freshnessAge)
	if len(headerRows) != 1 || headerRows[0].EndpointID != endpointID {
		return EndpointDetailView{}, endpointDetailIdentityFailure("Endpoint header")
	}

	mappedState, stateTruncated := mapEndpointDetailState(state, stateErr)
	mappedSchedules, schedulesTruncated := mapEndpointDetailSchedules(schedules, scheduleErr)
	mappedFirewall, firewallTruncated, firewallMappingErr := mapEndpointDetailFirewall(firewall, firewallErr)
	if firewallErr == nil {
		firewallErr = firewallMappingErr
	}
	mappedSystem, systemErr := mapEndpointDetailSystem(endpoint.SystemInfo)

	view := EndpointDetailView{
		Header:             headerRows[0],
		State:              mappedState,
		StateTruncated:     stateTruncated,
		Schedules:          mappedSchedules,
		SchedulesTruncated: schedulesTruncated,
		Firewall:           mappedFirewall,
		FirewallTruncated:  firewallTruncated,
		System:             mappedSystem,
	}
	view.Sections = EndpointDetailSections{
		Overview:  workspaceSectionResult("Endpoint overview", nil, 1, loadedAt, latestEndpointObservation([]admin.Endpoint{endpoint})),
		State:     workspaceSectionResult("State evidence", stateErr, endpointDetailStateItemCount(state, stateErr), loadedAt, optionalTimePointer(state.ReportedAt)),
		Schedules: workspaceSectionResult("Schedule evidence", scheduleErr, len(mappedSchedules), loadedAt, latestScheduleObservation(schedules)),
		Firewall:  workspaceSectionResult("Firewall evidence", firewallErr, len(mappedFirewall), loadedAt, optionalTimePointer(firewall.ReportedAt)),
		System:    workspaceSectionResult("System evidence", systemErr, endpointDetailSystemItemCount(endpoint.SystemInfo, systemErr), loadedAt, endpointSystemObservation(endpoint.SystemInfo)),
	}
	return view, nil
}

func validateEndpointDetailID(endpointID string) error {
	if endpointID == "" || endpointID != strings.TrimSpace(endpointID) {
		return &ClassifiedError{
			Kind:     ErrorValidation,
			Message:  "Select a valid Endpoint before loading detail.",
			Guidance: "Return to Inventory and select an exact Endpoint ID.",
		}
	}
	return nil
}

func endpointDetailIdentityFailure(section string) error {
	return &ClassifiedError{
		Kind:     ErrorUnexpected,
		Message:  section + " did not match the selected Endpoint.",
		Guidance: "Close the detail view and select the Endpoint again.",
	}
}

func mapEndpointDetailState(report admin.StateReport, reportErr error) (StateEvidence, bool) {
	if reportErr != nil {
		return StateEvidence{}, false
	}
	itemLimit := min(len(report.Items), endpointDetailStateItemLimit)
	items := make([]StateEvidenceItem, 0, itemLimit)
	for _, item := range report.Items[:itemLimit] {
		subresultLimit := min(len(item.Subresults), endpointDetailSubresultLimit)
		subresults := make([]StateEvidenceSubresult, 0, subresultLimit)
		for _, subresult := range item.Subresults[:subresultLimit] {
			subresults = append(subresults, StateEvidenceSubresult{
				Target:          subresult.Target,
				Status:          mapComplianceStatus(subresult.Status),
				ReasonCode:      subresult.ReasonCode,
				DesiredSummary:  subresult.DesiredSummary,
				ObservedSummary: subresult.ObservedSummary,
			})
		}
		items = append(items, StateEvidenceItem{
			Address:             item.Address,
			Name:                item.Name,
			Description:         item.Description,
			Provider:            item.Provider,
			Status:              mapComplianceStatus(item.Status),
			ReasonCode:          item.ReasonCode,
			DesiredSummary:      item.DesiredSummary,
			ObservedSummary:     item.ObservedSummary,
			Subresults:          subresults,
			SubresultsTruncated: item.SubresultsTruncated || len(item.Subresults) > subresultLimit,
		})
	}
	return StateEvidence{
		EndpointID: report.EndpointID,
		ReleaseRef: report.ReleaseRef,
		Digest:     report.Digest,
		Status:     mapComplianceStatus(report.Status),
		ReportedAt: formatTimestamp(report.ReportedAt),
		Items:      items,
	}, len(report.Items) > itemLimit
}

func mapEndpointDetailSchedules(report admin.CronReport, reportErr error) ([]ScheduleEvidence, bool) {
	if reportErr != nil {
		return []ScheduleEvidence{}, false
	}
	limit := min(len(report.Jobs), endpointDetailScheduleLimit)
	result := make([]ScheduleEvidence, 0, limit)
	for _, job := range report.Jobs[:limit] {
		result = append(result, ScheduleEvidence{
			Name:             job.Name,
			Schedule:         job.Schedule,
			Applicable:       job.Applicable,
			LastStatus:       job.LastStatus,
			LastMessage:      job.LastMessage,
			LastScheduledFor: formatTimestamp(job.LastScheduledFor),
			LastCompletedAt:  formatTimestamp(job.LastCompletedAt),
		})
	}
	slices.SortFunc(result, func(left, right ScheduleEvidence) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result, len(report.Jobs) > limit
}

type endpointFirewallPayload struct {
	Timestamp time.Time `json:"timestamp"`
	RuleName  string    `json:"ruleName"`
	Action    string    `json:"action"`
	Protocol  string    `json:"protocol"`
	Ports     []int     `json:"ports"`
	Sources   []string  `json:"sources"`
	Backend   string    `json:"backend"`
	WouldHave string    `json:"wouldHave"`
	Enforced  bool      `json:"enforced"`
}

func mapEndpointDetailFirewall(report admin.FirewallAuditReport, reportErr error) ([]FirewallEvidence, bool, error) {
	if reportErr != nil || len(report.Report) == 0 {
		return []FirewallEvidence{}, false, nil
	}
	var payload []endpointFirewallPayload
	if err := json.Unmarshal(report.Report, &payload); err != nil {
		return []FirewallEvidence{}, false, &ClassifiedError{
			Kind:     ErrorValidation,
			Message:  "Firewall evidence was not in the expected structured format.",
			Guidance: "Refresh after the Endpoint reports firewall evidence again.",
		}
	}
	limit := min(len(payload), endpointDetailFirewallLimit)
	result := make([]FirewallEvidence, 0, limit)
	truncated := len(payload) > limit
	for _, entry := range payload[:limit] {
		portLimit := min(len(entry.Ports), endpointDetailListValueLimit)
		sourceLimit := min(len(entry.Sources), endpointDetailListValueLimit)
		truncated = truncated || len(entry.Ports) > portLimit || len(entry.Sources) > sourceLimit
		ports := slices.Clone(entry.Ports[:portLimit])
		sources := slices.Clone(entry.Sources[:sourceLimit])
		result = append(result, FirewallEvidence{
			Timestamp: formatTimestamp(entry.Timestamp),
			RuleName:  entry.RuleName,
			Action:    entry.Action,
			Protocol:  entry.Protocol,
			Ports:     ports,
			Sources:   sources,
			Backend:   entry.Backend,
			WouldHave: entry.WouldHave,
			Enforced:  entry.Enforced,
		})
	}
	return result, truncated, nil
}

type endpointSystemPayload struct {
	Hostname  string `json:"hostname"`
	OSRelease struct {
		PrettyName string `json:"prettyName"`
		Name       string `json:"name"`
		VersionID  string `json:"versionId"`
	} `json:"osRelease"`
	CPU struct {
		ModelName string `json:"modelName"`
		CoreCount string `json:"coreCount"`
	} `json:"cpu"`
	RAM struct {
		MemTotal string `json:"memTotal"`
	} `json:"ram"`
	Kernel struct {
		Version string `json:"version"`
	} `json:"kernel"`
}

func mapEndpointDetailSystem(summary *admin.SystemInfoSummary) (SystemEvidence, error) {
	if summary == nil || len(summary.Report) == 0 {
		return SystemEvidence{}, nil
	}
	var payload endpointSystemPayload
	if err := json.Unmarshal(summary.Report, &payload); err != nil {
		return SystemEvidence{}, &ClassifiedError{
			Kind:     ErrorValidation,
			Message:  "System evidence was not in the expected structured format.",
			Guidance: "Refresh after the Endpoint reports system evidence again.",
		}
	}
	osName := payload.OSRelease.PrettyName
	if osName == "" {
		osName = strings.TrimSpace(payload.OSRelease.Name + " " + payload.OSRelease.VersionID)
	}
	return SystemEvidence{
		Hostname:   payload.Hostname,
		OS:         osName,
		Kernel:     payload.Kernel.Version,
		CPU:        payload.CPU.ModelName,
		CPUCores:   payload.CPU.CoreCount,
		Memory:     payload.RAM.MemTotal,
		Digest:     summary.Digest,
		ReportedAt: formatTimestamp(summary.ReportedAt),
	}, nil
}

func endpointDetailStateItemCount(report admin.StateReport, reportErr error) int {
	if reportErr != nil || !report.HasReport() {
		return 0
	}
	return 1
}

func endpointDetailSystemItemCount(summary *admin.SystemInfoSummary, err error) int {
	if err != nil || summary == nil || len(summary.Report) == 0 {
		return 0
	}
	return 1
}

func latestScheduleObservation(report admin.CronReport) *time.Time {
	var latest time.Time
	for _, job := range report.Jobs {
		if job.LastScheduledFor.After(latest) {
			latest = job.LastScheduledFor
		}
		if job.LastCompletedAt.After(latest) {
			latest = job.LastCompletedAt
		}
	}
	return optionalTimePointer(latest)
}

func endpointSystemObservation(summary *admin.SystemInfoSummary) *time.Time {
	if summary == nil {
		return nil
	}
	return optionalTimePointer(summary.ReportedAt)
}
