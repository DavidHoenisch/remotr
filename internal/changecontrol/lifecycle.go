package changecontrol

import (
	"fmt"
	"sort"
)

func (r *Registry) List() []ChangeRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ChangeRequest, 0, len(r.requests))
	for _, request := range r.requests {
		out = append(out, cloneRequest(request))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (r *Registry) Pause(id, actorID string) (ChangeRequest, error) {
	return r.setLifecycleState(id, actorID, AuthorizationPaused, AuditPaused)
}

func (r *Registry) Resume(id, actorID string) (ChangeRequest, error) {
	r.mu.RLock()
	_, authorized := r.rollouts[id]
	request := r.requests[id]
	r.mu.RUnlock()
	if !authorized {
		return ChangeRequest{}, fmt.Errorf("change request %q has no rollout authorization", id)
	}
	if request.LegacyMigration != nil {
		return ChangeRequest{}, fmt.Errorf("legacy Change request %q is visible but non-enforcing; explicit regeneration is required", id)
	}
	return r.setLifecycleState(id, actorID, AuthorizationActive, AuditResumed)
}

func (r *Registry) Revoke(id, actorID string) (ChangeRequest, error) {
	return r.setLifecycleState(id, actorID, AuthorizationRevoked, AuditRevoked)
}

func (r *Registry) setLifecycleState(id, actorID string, state AuthorizationState, action AuditAction) (ChangeRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[id]
	if !ok {
		return ChangeRequest{}, fmt.Errorf("change request %q not found", id)
	}
	previous := r.snapshotLocked()
	now := r.now().UTC()
	request.AuthorizationState = state
	request.AuditHistory = append(request.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: action})
	r.requests[id] = request
	if err := r.persistLocked(previous); err != nil {
		return ChangeRequest{}, err
	}
	return cloneRequest(request), nil
}

// CreateBaselineAdoption aggregates existing high-risk state into one reviewed
// Change request for a fleet.
func (r *Registry) CreateBaselineAdoption(plan FleetPlan, actorID string) (ChangeRequest, error) {
	for i := range plan.Resources {
		if plan.Resources[i].Risk.RequiresPreflight() {
			plan.Resources[i].AuthorizationGroup = "baseline-adoption"
			plan.Resources[i].BaselineEligible = true
		}
	}
	return r.createBaselineAdoption(plan, actorID)
}

// CreateCanonicalBaselineAdoption admits only server-derived canonical
// identities and preserves the provider descriptor's baseline eligibility.
func (r *Registry) CreateCanonicalBaselineAdoption(plan FleetPlan, trusted []CanonicalResourceIdentity, actorID string) (ChangeRequest, error) {
	if err := verifyCanonicalPlan(plan, trusted); err != nil {
		return ChangeRequest{}, err
	}
	for i := range plan.Resources {
		if plan.Resources[i].Risk.RequiresPreflight() {
			plan.Resources[i].AuthorizationGroup = "baseline-adoption"
		}
	}
	return r.createBaselineAdoption(plan, actorID)
}

func (r *Registry) createBaselineAdoption(plan FleetPlan, actorID string) (ChangeRequest, error) {
	requests, err := r.createChangeRequests(plan, actorID, AuditBaselineAdoption)
	if err != nil {
		return ChangeRequest{}, err
	}
	if len(requests) != 1 {
		return ChangeRequest{}, fmt.Errorf("baseline adoption requires at least one high-risk resource")
	}
	return requests[0], nil
}
