package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const (
	workspaceConcurrencyLimit    = 4
	defaultWorkspaceFreshnessAge = 10 * time.Minute
)

type WorkspaceOption func(*WorkspaceService)

type WorkspaceService struct {
	connection   *ConnectionService
	now          func() time.Time
	concurrency  int
	freshnessAge time.Duration
}

type workspaceCompositionInput struct {
	identity          OperatorIdentity
	fleets            []string
	fleetsErr         error
	endpoints         []admin.Endpoint
	endpointsErr      error
	fleetReports      []admin.FleetStateReport
	fleetReportErrors []error
	changes           []admin.ChangeRequest
	changesErr        error
	activity          admin.AuditEventPage
	activityErr       error
	loadedAt          time.Time
}

func NewWorkspaceService(options ...WorkspaceOption) *WorkspaceService {
	service := &WorkspaceService{
		connection:   NewConnectionService(),
		now:          time.Now,
		concurrency:  workspaceConcurrencyLimit,
		freshnessAge: defaultWorkspaceFreshnessAge,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithWorkspaceClock(now func() time.Time) WorkspaceOption {
	return func(service *WorkspaceService) {
		if now != nil {
			service.now = now
		}
	}
}

func WithWorkspaceFreshnessThreshold(age time.Duration) WorkspaceOption {
	return func(service *WorkspaceService) {
		if age > 0 {
			service.freshnessAge = age
		}
	}
}

func (s *WorkspaceService) Load(ctx context.Context, profile ConnectionProfile) (WorkspaceView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	connected, err := s.connection.connect(ctx, profile)
	if err != nil {
		return WorkspaceView{}, err
	}
	return s.loadConnected(ctx, OperatorIdentity{
		OperatorID: connected.view.OperatorID,
		Roles:      slices.Clone(connected.view.Roles),
	}, connected.client)
}

func (s *WorkspaceService) loadConnected(ctx context.Context, identity OperatorIdentity, client *admin.Client) (WorkspaceView, error) {
	if client == nil {
		return WorkspaceView{}, ErrSessionNotConnected
	}
	var (
		fleets       []string
		fleetsErr    error
		endpoints    []admin.Endpoint
		endpointsErr error
		changes      []admin.ChangeRequest
		changesErr   error
		activity     admin.AuditEventPage
		activityErr  error
	)
	runWorkspaceTasks(ctx, s.concurrency, []func(context.Context){
		func(taskCtx context.Context) {
			fleets, fleetsErr = client.ListFleetsContext(taskCtx)
		},
		func(taskCtx context.Context) {
			endpoints, endpointsErr = client.ListEndpointsContext(taskCtx)
		},
		func(taskCtx context.Context) {
			changes, changesErr = client.ListChangeRequestsContext(taskCtx)
		},
		func(taskCtx context.Context) {
			activity, activityErr = client.ListAuditEventsContext(taskCtx, admin.AuditListOptions{Limit: 50})
		},
	})
	if cause := context.Cause(ctx); cause != nil {
		return WorkspaceView{}, cause
	}

	slices.Sort(fleets)
	fleetReports := make([]admin.FleetStateReport, len(fleets))
	fleetReportErrors := make([]error, len(fleets))
	if fleetsErr == nil {
		tasks := make([]func(context.Context), 0, len(fleets))
		for index := range fleets {
			index := index
			tasks = append(tasks, func(taskCtx context.Context) {
				fleetReports[index], fleetReportErrors[index] = client.GetFleetStateReportContext(taskCtx, fleets[index])
			})
		}
		runWorkspaceTasks(ctx, s.concurrency, tasks)
	}
	if cause := context.Cause(ctx); cause != nil {
		return WorkspaceView{}, cause
	}

	return s.composeWorkspace(workspaceCompositionInput{
		identity: OperatorIdentity{
			OperatorID: identity.OperatorID,
			Roles:      slices.Clone(identity.Roles),
		},
		fleets:            fleets,
		fleetsErr:         fleetsErr,
		endpoints:         endpoints,
		endpointsErr:      endpointsErr,
		fleetReports:      fleetReports,
		fleetReportErrors: fleetReportErrors,
		changes:           changes,
		changesErr:        changesErr,
		activity:          activity,
		activityErr:       activityErr,
		loadedAt:          s.now().UTC(),
	}), nil
}

func (s *WorkspaceService) composeWorkspace(input workspaceCompositionInput) WorkspaceView {
	stateErr, successfulStateReports := aggregateWorkspaceStateError(input.fleetReportErrors)
	if input.fleetsErr != nil {
		stateErr = input.fleetsErr
		successfulStateReports = 0
	}
	workspace := WorkspaceView{
		Operator: OperatorView{
			OperatorID: input.identity.OperatorID,
			Roles:      slices.Clone(input.identity.Roles),
		},
		Endpoints:          mapWorkspaceEndpoints(input.endpoints, input.fleetReports, input.loadedAt, s.freshnessAge),
		Fleets:             mapWorkspaceFleets(input.fleets, input.endpoints, input.fleetReports, input.loadedAt, s.freshnessAge),
		StateEvidence:      mapWorkspaceStateEvidence(input.fleetReports),
		ChangeRequests:     mapWorkspaceChanges(input.changes),
		Activity:           mapWorkspaceActivity(input.activity.Events),
		ActivityNextCursor: input.activity.NextCursor,
	}
	workspace.Sections = WorkspaceSections{
		Fleets:         workspaceSectionResult("Fleets", input.fleetsErr, len(workspace.Fleets), input.loadedAt, nil),
		Endpoints:      workspaceSectionResult("Endpoints", input.endpointsErr, len(workspace.Endpoints), input.loadedAt, latestEndpointObservation(input.endpoints)),
		State:          workspaceStateSectionResult(stateErr, successfulStateReports, len(input.fleets), input.loadedAt, latestStateObservation(input.fleetReports)),
		ChangeRequests: workspaceSectionResult("Change requests", input.changesErr, len(workspace.ChangeRequests), input.loadedAt, latestChangeObservation(input.changes)),
		Activity:       workspaceSectionResult("Activity", input.activityErr, len(workspace.Activity), input.loadedAt, latestActivityObservation(input.activity.Events)),
	}
	if input.activityErr != nil {
		workspace.Activity = []ActivityEvent{}
		workspace.ActivityNextCursor = ""
	}
	return workspace
}

func runWorkspaceTasks(ctx context.Context, limit int, tasks []func(context.Context)) {
	if len(tasks) == 0 {
		return
	}
	if limit < 1 {
		limit = 1
	}
	semaphore := make(chan struct{}, limit)
	var wait sync.WaitGroup
	for _, task := range tasks {
		task := task
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			task(ctx)
		}()
	}
	wait.Wait()
}

func mapWorkspaceEndpoints(endpoints []admin.Endpoint, reports []admin.FleetStateReport, now time.Time, freshnessAge time.Duration) []EndpointRow {
	statusByEndpoint := map[string]ComplianceStatus{}
	for _, report := range reports {
		for _, endpointReport := range report.Endpoints {
			statusByEndpoint[endpointReport.EndpointID] = mapComplianceStatus(endpointReport.Status)
		}
	}

	rows := make([]EndpointRow, 0, len(endpoints))
	for _, endpoint := range endpoints {
		labels := make([]LabelView, 0, len(endpoint.Labels))
		for key, value := range endpoint.Labels {
			labels = append(labels, LabelView{Key: key, Value: value})
		}
		slices.SortFunc(labels, func(left, right LabelView) int {
			return strings.Compare(left.Key, right.Key)
		})
		compliance, exists := statusByEndpoint[endpoint.ID]
		if !exists {
			compliance = ComplianceNotReported
		}
		freshness := FreshnessNeverReported
		var evidenceAt *string
		releaseRef := ""
		if endpoint.LastCheckIn != nil {
			observed := endpoint.LastCheckIn.At.UTC()
			if !observed.IsZero() {
				freshness = classifyWorkspaceFreshness(now, observed, freshnessAge)
				evidenceAt = timestampPointer(observed)
			}
			releaseRef = endpoint.LastCheckIn.ReleaseRef
		}
		usernames := slices.Clone(endpoint.Usernames)
		slices.Sort(usernames)
		rows = append(rows, EndpointRow{
			EndpointID:           endpoint.ID,
			Fleet:                endpoint.Fleet,
			Usernames:            usernames,
			Compliance:           compliance,
			Freshness:            freshness,
			DesiredAgentVersion:  endpoint.DesiredAgentVersion,
			ReportedAgentVersion: endpoint.ReportedAgentVersion,
			ReleaseRef:           releaseRef,
			Labels:               labels,
			EvidenceAt:           evidenceAt,
		})
	}
	slices.SortFunc(rows, func(left, right EndpointRow) int {
		return strings.Compare(left.EndpointID, right.EndpointID)
	})
	return rows
}

func classifyWorkspaceFreshness(now, observed time.Time, freshnessAge time.Duration) FreshnessStatus {
	if observed.IsZero() {
		return FreshnessNeverReported
	}
	if freshnessAge <= 0 {
		freshnessAge = defaultWorkspaceFreshnessAge
	}
	if now.UTC().Sub(observed.UTC()) <= freshnessAge {
		return FreshnessRecent
	}
	return FreshnessStale
}

func mapWorkspaceFleets(
	fleets []string,
	endpoints []admin.Endpoint,
	reports []admin.FleetStateReport,
	now time.Time,
	freshnessAge time.Duration,
) []FleetSummary {
	rows := mapWorkspaceEndpoints(endpoints, reports, now, freshnessAge)
	summaries := make([]FleetSummary, 0, len(fleets))
	for _, fleet := range fleets {
		compliance := map[string]int{}
		freshness := map[string]int{}
		agentVersions := map[string]int{}
		memberCount := 0
		for _, row := range rows {
			if row.Fleet != fleet {
				continue
			}
			memberCount++
			compliance[string(row.Compliance)]++
			freshness[string(row.Freshness)]++
			version := row.ReportedAgentVersion
			if version == "" {
				version = string(ComplianceNotReported)
			}
			agentVersions[version]++
		}
		summaries = append(summaries, FleetSummary{
			Fleet:         fleet,
			EndpointCount: memberCount,
			Compliance: statusCountsInOrder([]string{
				string(ComplianceCompliant),
				string(ComplianceDrifted),
				string(ComplianceUnsupported),
				string(ComplianceCheckFailed),
				string(ComplianceDeferred),
				string(ComplianceApplyFailed),
				string(ComplianceNotReported),
			}, compliance),
			Freshness: statusCountsInOrder([]string{
				string(FreshnessRecent),
				string(FreshnessStale),
				string(FreshnessNeverReported),
			}, freshness),
			AgentVersions: sortedStatusCounts(agentVersions),
		})
	}
	return summaries
}

func mapWorkspaceStateEvidence(reports []admin.FleetStateReport) []StateEvidence {
	evidence := []StateEvidence{}
	for _, fleetReport := range reports {
		for _, report := range fleetReport.Endpoints {
			items := make([]StateEvidenceItem, 0, len(report.Items))
			for _, item := range report.Items {
				subresults := make([]StateEvidenceSubresult, 0, len(item.Subresults))
				for _, subresult := range item.Subresults {
					subresults = append(subresults, StateEvidenceSubresult{
						Target:          subresult.Target,
						Status:          mapComplianceStatus(subresult.Status),
						ReasonCode:      subresult.ReasonCode,
						DesiredSummary:  subresult.DesiredSummary.String(),
						ObservedSummary: subresult.ObservedSummary.String(),
					})
				}
				items = append(items, StateEvidenceItem{
					Address:             item.Address,
					Name:                item.Name,
					Description:         item.Description,
					Provider:            item.Provider,
					Status:              mapComplianceStatus(item.Status),
					ReasonCode:          item.ReasonCode,
					DesiredSummary:      item.DesiredSummary.String(),
					ObservedSummary:     item.ObservedSummary.String(),
					Subresults:          subresults,
					SubresultsTruncated: item.SubresultsTruncated,
				})
			}
			evidence = append(evidence, StateEvidence{
				EndpointID: report.EndpointID,
				ReleaseRef: report.ReleaseRef,
				Digest:     report.Digest,
				Status:     mapComplianceStatus(report.Status),
				ReportedAt: formatTimestamp(report.ReportedAt),
				Items:      items,
			})
		}
	}
	slices.SortFunc(evidence, func(left, right StateEvidence) int {
		return strings.Compare(left.EndpointID, right.EndpointID)
	})
	return evidence
}

func mapWorkspaceChanges(changes []admin.ChangeRequest) []ChangeRequestSummary {
	summaries := make([]ChangeRequestSummary, 0, len(changes))
	for _, change := range changes {
		updatedAt := change.CreatedAt.UTC()
		for _, event := range change.AuditHistory {
			if event.At.After(updatedAt) {
				updatedAt = event.At.UTC()
			}
		}
		summaries = append(summaries, ChangeRequestSummary{
			ChangeRequestID:   change.ID,
			Fleet:             change.Fleet,
			ReleaseRef:        change.ReleaseRef,
			Risk:              string(change.Risk),
			Lifecycle:         string(change.AuthorizationState),
			TargetCount:       len(change.FrozenTargets),
			RequiredApprovals: change.RequiredApprovals,
			ApprovalCount:     len(change.Approvals),
			UpdatedAt:         formatTimestamp(updatedAt),
		})
	}
	slices.SortFunc(summaries, func(left, right ChangeRequestSummary) int {
		return strings.Compare(left.ChangeRequestID, right.ChangeRequestID)
	})
	return summaries
}

func mapWorkspaceActivity(events []admin.AuditEvent) []ActivityEvent {
	activity := make([]ActivityEvent, 0, len(events))
	for _, event := range events {
		activity = append(activity, mapAuditEvent(event))
	}
	return activity
}

func mapComplianceStatus(status admin.StateReportStatus) ComplianceStatus {
	switch status {
	case admin.StateCompliant:
		return ComplianceCompliant
	case admin.StateDrifted:
		return ComplianceDrifted
	case admin.StateUnsupported:
		return ComplianceUnsupported
	case admin.StateCheckFailed:
		return ComplianceCheckFailed
	case admin.StateDeferred:
		return ComplianceDeferred
	case admin.StateApplyFailed:
		return ComplianceApplyFailed
	case admin.StateNoReport:
		return ComplianceNotReported
	default:
		return ComplianceNotReported
	}
}

func sortedStatusCounts(counts map[string]int) []StatusCount {
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	slices.Sort(statuses)
	result := make([]StatusCount, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, StatusCount{Status: status, Count: counts[status]})
	}
	return result
}

func statusCountsInOrder(statuses []string, counts map[string]int) []StatusCount {
	result := make([]StatusCount, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, StatusCount{Status: status, Count: counts[status]})
	}
	return result
}

func aggregateWorkspaceStateError(errs []error) (error, int) {
	var first error
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if first == nil {
			first = err
		}
	}
	return first, successes
}

func workspaceSectionResult(name string, err error, itemCount int, loadedAt time.Time, observedAt *time.Time) SectionResult {
	if err != nil {
		state, classified := classifyWorkspaceSectionError(name, err)
		return SectionResult{
			State: state,
			Snapshot: SnapshotTimestamps{
				FailedAt: timestampPointer(loadedAt),
			},
			Error: classified,
		}
	}
	state := SectionReady
	if itemCount == 0 {
		state = SectionEmpty
	}
	return SectionResult{
		State: state,
		Snapshot: SnapshotTimestamps{
			LoadedAt:   formatTimestamp(loadedAt),
			ObservedAt: optionalTimestamp(observedAt),
		},
	}
}

func workspaceStateSectionResult(err error, successes, total int, loadedAt time.Time, observedAt *time.Time) SectionResult {
	if err == nil {
		return workspaceSectionResult("State evidence", nil, total, loadedAt, observedAt)
	}
	state, classified := classifyWorkspaceSectionError("State evidence", err)
	if successes > 0 {
		state = SectionPartial
	}
	return SectionResult{
		State: state,
		Snapshot: SnapshotTimestamps{
			LoadedAt:   formatTimestamp(loadedAt),
			ObservedAt: optionalTimestamp(observedAt),
			FailedAt:   timestampPointer(loadedAt),
		},
		Error: classified,
	}
}

func classifyWorkspaceSectionError(name string, err error) (SectionState, *ClassifiedError) {
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return SectionFailed, classified
	}
	var responseError *admin.ResponseError
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusForbidden {
		return SectionUnavailable, &ClassifiedError{
			Kind:     ErrorAuthorization,
			Message:  "The current Operator is not authorized to load " + name + ".",
			Guidance: "Ask a Remotr administrator to review the Operator's assigned roles.",
		}
	}
	if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
		return SectionUnavailable, &ClassifiedError{
			Kind:     ErrorUnavailable,
			Message:  name + " is unavailable from this Remotr server.",
			Guidance: "Verify the selected profile and server version.",
		}
	}
	if errors.As(err, &responseError) && responseError.StatusCode >= http.StatusInternalServerError {
		return SectionUnavailable, &ClassifiedError{
			Kind:     ErrorUnavailable,
			Message:  name + " is temporarily unavailable from the Remotr server.",
			Guidance: "Refresh this section after the server is healthy.",
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return SectionFailed, &ClassifiedError{
			Kind:     ErrorConnection,
			Message:  name + " could not be reached.",
			Guidance: "Check the connection and refresh this section.",
		}
	}
	return SectionFailed, &ClassifiedError{
		Kind:     ErrorUnexpected,
		Message:  name + " could not be loaded.",
		Guidance: "Refresh this section or reconnect the active profile.",
	}
}

func latestEndpointObservation(endpoints []admin.Endpoint) *time.Time {
	var latest time.Time
	for _, endpoint := range endpoints {
		if endpoint.LastCheckIn != nil && endpoint.LastCheckIn.At.After(latest) {
			latest = endpoint.LastCheckIn.At.UTC()
		}
	}
	return optionalTimePointer(latest)
}

func latestStateObservation(reports []admin.FleetStateReport) *time.Time {
	var latest time.Time
	for _, fleetReport := range reports {
		for _, report := range fleetReport.Endpoints {
			if report.ReportedAt.After(latest) {
				latest = report.ReportedAt.UTC()
			}
		}
	}
	return optionalTimePointer(latest)
}

func latestChangeObservation(changes []admin.ChangeRequest) *time.Time {
	var latest time.Time
	for _, change := range changes {
		updatedAt := change.CreatedAt
		for _, event := range change.AuditHistory {
			if event.At.After(updatedAt) {
				updatedAt = event.At
			}
		}
		if updatedAt.After(latest) {
			latest = updatedAt.UTC()
		}
	}
	return optionalTimePointer(latest)
}

func latestActivityObservation(events []admin.AuditEvent) *time.Time {
	var latest time.Time
	for _, event := range events {
		if event.OccurredAt.After(latest) {
			latest = event.OccurredAt.UTC()
		}
	}
	return optionalTimePointer(latest)
}

func optionalTimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func formatTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func optionalTimestamp(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestampPointer(*value)
}

func timestampPointer(value time.Time) *string {
	formatted := formatTimestamp(value)
	if formatted == "" {
		return nil
	}
	return &formatted
}
