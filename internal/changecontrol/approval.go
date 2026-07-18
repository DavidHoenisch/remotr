package changecontrol

import (
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

const SingleOperatorDestructiveWarning = "destructive changes require only one operator approval"

type Approval struct {
	OperatorID    string    `json:"operator_id"`
	ApprovedAt    time.Time `json:"approved_at"`
	Justification string    `json:"justification,omitempty"`
}

type ApprovalPolicy struct {
	Global map[models.RiskClass]int            `json:"global,omitempty"`
	Fleet  map[string]map[models.RiskClass]int `json:"fleet,omitempty"`
}

func (p ApprovalPolicy) threshold(fleet string, risk models.RiskClass) int {
	if overrides := p.Fleet[fleet]; overrides != nil && overrides[risk] > 0 {
		return overrides[risk]
	}
	if p.Global[risk] > 0 {
		return p.Global[risk]
	}
	if risk == models.RiskDestructive {
		return 2
	}
	return 1
}

type ApprovalResult struct {
	Ready         bool                 `json:"ready"`
	ApprovalCount int                  `json:"approval_count"`
	Required      int                  `json:"required"`
	Authorization RolloutAuthorization `json:"authorization,omitempty"`
}

func (r *Registry) ApproveRollout(changeRequestID string, spec RolloutSpec, actorID, justification string) (ApprovalResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return ApprovalResult{}, fmt.Errorf("change request %q not found", changeRequestID)
	}
	if !r.canApprove(actorID, request.Fleet, request.Risk) {
		return ApprovalResult{}, fmt.Errorf("operator %q is not permitted to approve %s risk", actorID, request.Risk)
	}
	if request.LegacyMigration != nil {
		return ApprovalResult{}, fmt.Errorf("legacy Change request %q is visible but non-enforcing; explicit regeneration is required", changeRequestID)
	}
	for _, approval := range request.Approvals {
		if approval.OperatorID == actorID {
			count := len(request.Approvals)
			return ApprovalResult{Ready: request.AuthorizationState == AuthorizationActive, ApprovalCount: count, Required: request.RequiredApprovals}, nil
		}
	}
	previous := r.snapshotLocked()
	request.Approvals = append(request.Approvals, Approval{OperatorID: actorID, ApprovedAt: r.now().UTC(), Justification: justification})
	r.requests[changeRequestID] = request
	count := len(request.Approvals)
	required := request.RequiredApprovals
	if count < required {
		if err := r.persistLocked(previous); err != nil {
			return ApprovalResult{}, err
		}
		return ApprovalResult{ApprovalCount: count, Required: required}, nil
	}
	authorization, err := r.finalizeRolloutLocked(changeRequestID, spec, actorID, justification)
	if err != nil {
		r.restoreLocked(previous)
		return ApprovalResult{}, err
	}
	if err := r.persistLocked(previous); err != nil {
		return ApprovalResult{}, err
	}
	return ApprovalResult{Ready: true, ApprovalCount: count, Required: required, Authorization: authorization}, nil
}

// AuthorizeRollout is the compatibility entrypoint for callers that submit
// one approval. Multi-approval callers should inspect ApproveRollout directly.
func (r *Registry) AuthorizeRollout(changeRequestID string, spec RolloutSpec, actorID, justification string) (RolloutAuthorization, error) {
	result, err := r.ApproveRollout(changeRequestID, spec, actorID, justification)
	if err != nil || !result.Ready {
		return RolloutAuthorization{}, err
	}
	return result.Authorization, nil
}

func (r *Registry) SetApprovalPolicy(policy ApprovalPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous := r.snapshotLocked()
	r.policy = cloneApprovalPolicy(policy)
	return r.persistLocked(previous)
}

func (r *Registry) PolicyWarnings() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for fleet, overrides := range r.policy.Fleet {
		if overrides[models.RiskDestructive] == 1 {
			return []string{fleet + ": " + SingleOperatorDestructiveWarning}
		}
	}
	if r.policy.Global[models.RiskDestructive] == 1 {
		return []string{SingleOperatorDestructiveWarning}
	}
	return []string{}
}

func cloneApprovalPolicy(input ApprovalPolicy) ApprovalPolicy {
	out := ApprovalPolicy{Global: make(map[models.RiskClass]int), Fleet: make(map[string]map[models.RiskClass]int)}
	for risk, threshold := range input.Global {
		out.Global[risk] = threshold
	}
	for fleet, overrides := range input.Fleet {
		copyOverrides := make(map[models.RiskClass]int, len(overrides))
		for risk, threshold := range overrides {
			copyOverrides[risk] = threshold
		}
		out.Fleet[fleet] = copyOverrides
	}
	return out
}
