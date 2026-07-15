package changecontrol

import "fmt"

type OutcomeState string

const (
	OutcomeVerifiedSuccess    OutcomeState = "verified_successful"
	OutcomeFailedOrRolledBack OutcomeState = "failed_or_rolled_back"
	OutcomeCapabilityBlocked  OutcomeState = "capability_or_preflight_blocked"
	OutcomeNotSeen            OutcomeState = "not_seen"
)

type TargetOutcome struct {
	EndpointID string       `json:"endpoint_id"`
	State      OutcomeState `json:"state"`
	Reason     string       `json:"reason,omitempty"`
}

type TargetOutcomeSummary struct {
	VerifiedSuccessful int `json:"verified_successful"`
	FailedOrRolledBack int `json:"failed_or_rolled_back"`
	Blocked            int `json:"blocked"`
	NotSeen            int `json:"not_seen"`
}

type BaselinePromotionOptions struct {
	AcknowledgeExceptions bool `json:"acknowledge_exceptions"`
}

type AutomaticPromotionPolicy struct {
	CanaryStages      []int `json:"canary_stages"`
	MinimumSuccessful int   `json:"minimum_successful"`
	MaximumFailures   int   `json:"maximum_failures"`
}

func (r *Registry) RecordTargetOutcome(changeRequestID string, outcome TargetOutcome, actorID string) error {
	if !validOutcome(outcome.State) {
		return fmt.Errorf("invalid target outcome %q", outcome.State)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return fmt.Errorf("change request %q not found", changeRequestID)
	}
	found := false
	for _, target := range request.FrozenTargets {
		if target.EndpointID == outcome.EndpointID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("endpoint %q is not a frozen target", outcome.EndpointID)
	}
	previous := r.snapshotLocked()
	if request.Outcomes == nil {
		request.Outcomes = make(map[string]TargetOutcome)
	}
	request.Outcomes[outcome.EndpointID] = outcome
	request.AuditHistory = append(request.AuditHistory, AuditEntry{At: r.now().UTC(), ActorID: actorID, Action: AuditTargetOutcome, Details: outcome.EndpointID + ":" + string(outcome.State)})
	r.requests[changeRequestID] = request
	return r.persistLocked(previous)
}

func (r *Registry) OutcomeSummary(changeRequestID string) (TargetOutcomeSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return TargetOutcomeSummary{}, fmt.Errorf("change request %q not found", changeRequestID)
	}
	return summarizeOutcomes(request), nil
}

func summarizeOutcomes(request ChangeRequest) TargetOutcomeSummary {
	var summary TargetOutcomeSummary
	for _, target := range request.FrozenTargets {
		outcome, ok := request.Outcomes[target.EndpointID]
		if !ok {
			summary.NotSeen++
			continue
		}
		switch outcome.State {
		case OutcomeVerifiedSuccess:
			summary.VerifiedSuccessful++
		case OutcomeFailedOrRolledBack:
			summary.FailedOrRolledBack++
		case OutcomeCapabilityBlocked:
			summary.Blocked++
		default:
			summary.NotSeen++
		}
	}
	return summary
}

func (r *Registry) PromoteBaselineWithOptions(changeRequestID, resourceAddress, actorID string, options BaselinePromotionOptions) (BaselineAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return BaselineAuthorization{}, fmt.Errorf("change request %q not found", changeRequestID)
	}
	summary := summarizeOutcomes(request)
	if summary.VerifiedSuccessful == 0 {
		return BaselineAuthorization{}, fmt.Errorf("baseline promotion requires verified success")
	}
	if summary.FailedOrRolledBack+summary.Blocked+summary.NotSeen > 0 && !options.AcknowledgeExceptions {
		return BaselineAuthorization{}, fmt.Errorf("baseline promotion exceptions require acknowledgement")
	}
	previous := r.snapshotLocked()
	if options.AcknowledgeExceptions {
		request.AuditHistory = append(request.AuditHistory, AuditEntry{At: r.now().UTC(), ActorID: actorID, Action: AuditExceptionsAcknowledged})
		r.requests[changeRequestID] = request
	}
	baseline, err := r.promoteBaselineLocked(changeRequestID, resourceAddress, actorID)
	if err != nil {
		r.restoreLocked(previous)
		return BaselineAuthorization{}, err
	}
	if err := r.persistLocked(previous); err != nil {
		return BaselineAuthorization{}, err
	}
	return baseline, nil
}

func (r *Registry) SetAutomaticPromotionPolicy(fleet string, policy AutomaticPromotionPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.snapshotLocked()
	policy.CanaryStages = append([]int(nil), policy.CanaryStages...)
	r.automaticPromotion[fleet] = policy
	return r.persistLocked(previous)
}

func (r *Registry) TryAutomaticBaselinePromotion(changeRequestID, resourceAddress string) (BaselineAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	policy, configured := r.automaticPromotion[request.Fleet]
	if !ok {
		return BaselineAuthorization{}, fmt.Errorf("change request %q not found", changeRequestID)
	}
	if !configured || len(policy.CanaryStages) == 0 || policy.MinimumSuccessful <= 0 || policy.MaximumFailures < 0 {
		return BaselineAuthorization{}, fmt.Errorf("automatic baseline promotion policy is not fully configured")
	}
	summary := summarizeOutcomes(request)
	if summary.VerifiedSuccessful < policy.MinimumSuccessful {
		return BaselineAuthorization{}, fmt.Errorf("minimum successful evidence not met")
	}
	if summary.FailedOrRolledBack > 0 || summary.FailedOrRolledBack > policy.MaximumFailures {
		return BaselineAuthorization{}, fmt.Errorf("unresolved failure or rollback blocks automatic promotion")
	}
	previous := r.snapshotLocked()
	baseline, err := r.promoteBaselineLocked(changeRequestID, resourceAddress, "system")
	if err != nil {
		return BaselineAuthorization{}, err
	}
	if err := r.persistLocked(previous); err != nil {
		return BaselineAuthorization{}, err
	}
	return cloneBaseline(baseline), nil
}

func validOutcome(state OutcomeState) bool {
	return state == OutcomeVerifiedSuccess || state == OutcomeFailedOrRolledBack || state == OutcomeCapabilityBlocked || state == OutcomeNotSeen
}

func cloneOutcomes(input map[string]TargetOutcome) map[string]TargetOutcome {
	if input == nil {
		return nil
	}
	out := make(map[string]TargetOutcome, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
