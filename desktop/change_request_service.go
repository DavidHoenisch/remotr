package main

import (
	"context"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

const (
	changeResourceLimit    = 200
	changeTargetLimit      = 1000
	changeApprovalLimit    = 100
	changeOutcomeLimit     = 1000
	changeHistoryLimit     = 500
	changeNestedValueLimit = 100
)

type ChangeRequestService struct {
	connection *ConnectionService
}

func NewChangeRequestService() *ChangeRequestService {
	return &ChangeRequestService{connection: NewConnectionService()}
}

func (s *ChangeRequestService) List(ctx context.Context, profile ConnectionProfile) ([]ChangeRequestSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	connected, err := s.connection.connect(ctx, profile)
	if err != nil {
		return nil, err
	}
	return s.ListConnected(ctx, connected.client)
}

func (s *ChangeRequestService) ListConnected(ctx context.Context, client *admin.Client) ([]ChangeRequestSummary, error) {
	if client == nil {
		return nil, ErrSessionNotConnected
	}
	changes, err := client.ListChangeRequestsContext(ctx)
	if err != nil {
		_, classified := classifyWorkspaceSectionError("Change requests", err)
		return nil, classified
	}
	return mapWorkspaceChanges(changes), nil
}

func (s *ChangeRequestService) LoadDetail(ctx context.Context, profile ConnectionProfile, changeRequestID string) (ChangeRequestDetailView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateChangeRequestID(changeRequestID); err != nil {
		return ChangeRequestDetailView{}, err
	}
	connected, err := s.connection.connect(ctx, profile)
	if err != nil {
		return ChangeRequestDetailView{}, err
	}
	return s.LoadDetailConnected(ctx, connected.client, changeRequestID)
}

func (s *ChangeRequestService) LoadDetailConnected(ctx context.Context, client *admin.Client, changeRequestID string) (ChangeRequestDetailView, error) {
	if client == nil {
		return ChangeRequestDetailView{}, ErrSessionNotConnected
	}
	if err := validateChangeRequestID(changeRequestID); err != nil {
		return ChangeRequestDetailView{}, err
	}
	change, err := client.GetChangeRequestContext(ctx, changeRequestID)
	if err != nil {
		_, classified := classifyWorkspaceSectionError("Change request detail", err)
		return ChangeRequestDetailView{}, classified
	}
	if change.ID != changeRequestID {
		return ChangeRequestDetailView{}, &ClassifiedError{
			Kind:     ErrorUnexpected,
			Message:  "Change request evidence did not match the selected request.",
			Guidance: "Return to Change requests and select the request again.",
		}
	}
	return mapChangeRequestDetail(change), nil
}

func validateChangeRequestID(changeRequestID string) error {
	if changeRequestID == "" || changeRequestID != strings.TrimSpace(changeRequestID) {
		return &ClassifiedError{
			Kind:     ErrorValidation,
			Message:  "Select a valid Change request before loading detail.",
			Guidance: "Return to Change requests and select an exact request ID.",
		}
	}
	return nil
}

func mapChangeRequestDetail(change admin.ChangeRequest) ChangeRequestDetailView {
	summaries := mapWorkspaceChanges([]admin.ChangeRequest{change})
	view := ChangeRequestDetailView{
		ReadOnly:           true,
		ArtifactDigest:     change.ArtifactDigest,
		AuthorizationGroup: change.AuthorizationGroup,
		PolicyWarning:      change.PolicyWarning,
	}
	if len(summaries) == 1 {
		view.Summary = summaries[0]
	}

	resourceLimit := min(len(change.Resources), changeResourceLimit)
	view.Resources = make([]ChangeResourceEvidence, 0, resourceLimit)
	for _, resource := range change.Resources[:resourceLimit] {
		view.ResourcesTruncated = view.ResourcesTruncated ||
			len(resource.DependsOn) > changeNestedValueLimit ||
			len(resource.ActivationTargets) > changeNestedValueLimit ||
			len(resource.PredictedEffects) > changeNestedValueLimit
		view.Resources = append(view.Resources, ChangeResourceEvidence{
			Address:            resource.Address,
			DesiredHash:        resource.DesiredHash,
			Risk:               string(resource.Risk),
			Provider:           resource.Provider,
			AuthorizationGroup: resource.AuthorizationGroup,
			DependsOn:          boundedStringSlice(resource.DependsOn, changeNestedValueLimit),
			ActivationTargets:  boundedStringSlice(resource.ActivationTargets, changeNestedValueLimit),
			PredictedEffects:   boundedStringSlice(resource.PredictedEffects, changeNestedValueLimit),
			RollbackClass:      resource.RollbackClass,
			BaselineEligible:   resource.BaselineEligible,
		})
	}
	view.ResourcesTruncated = view.ResourcesTruncated || len(change.Resources) > resourceLimit

	targetLimit := min(len(change.FrozenTargets), changeTargetLimit)
	view.Targets = make([]ChangeTargetEvidence, 0, targetLimit)
	for _, target := range change.FrozenTargets[:targetLimit] {
		view.Targets = append(view.Targets, ChangeTargetEvidence{
			EndpointID:      target.EndpointID,
			Compatible:      target.Compatible,
			PreflightReady:  target.PreflightReady,
			PreflightReason: target.PreflightReason,
		})
	}
	view.TargetsTruncated = len(change.FrozenTargets) > targetLimit

	approvalLimit := min(len(change.Approvals), changeApprovalLimit)
	view.Approvals = make([]ChangeApprovalEvidence, 0, approvalLimit)
	for _, approval := range change.Approvals[:approvalLimit] {
		view.Approvals = append(view.Approvals, ChangeApprovalEvidence{
			OperatorID:    approval.OperatorID,
			ApprovedAt:    formatTimestamp(approval.ApprovedAt),
			Justification: approval.Justification,
		})
	}
	view.ApprovalsTruncated = len(change.Approvals) > approvalLimit

	outcomeIDs := make([]string, 0, len(change.Outcomes))
	for endpointID := range change.Outcomes {
		outcomeIDs = append(outcomeIDs, endpointID)
	}
	slices.Sort(outcomeIDs)
	outcomeLimit := min(len(outcomeIDs), changeOutcomeLimit)
	view.Outcomes = make([]ChangeOutcomeEvidence, 0, outcomeLimit)
	for _, endpointID := range outcomeIDs[:outcomeLimit] {
		outcome := change.Outcomes[endpointID]
		view.Outcomes = append(view.Outcomes, ChangeOutcomeEvidence{
			EndpointID: outcome.EndpointID,
			State:      string(outcome.State),
			Reason:     outcome.Reason,
		})
	}
	view.OutcomesTruncated = len(outcomeIDs) > outcomeLimit

	historyLimit := min(len(change.AuditHistory), changeHistoryLimit)
	view.History = make([]ChangeHistoryEvidence, 0, historyLimit)
	for _, entry := range change.AuditHistory[:historyLimit] {
		view.History = append(view.History, ChangeHistoryEvidence{
			OccurredAt: formatTimestamp(entry.At),
			ActorID:    entry.ActorID,
			Action:     string(entry.Action),
			Details:    boundedViewString(entry.Details, activityDetailValueSize),
		})
	}
	view.HistoryTruncated = len(change.AuditHistory) > historyLimit
	return view
}

func boundedStringSlice(values []string, limit int) []string {
	if limit < 0 {
		limit = 0
	}
	return slices.Clone(values[:min(len(values), limit)])
}

func boundedViewString(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
