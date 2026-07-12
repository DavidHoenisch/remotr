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
	r.mu.RUnlock()
	if !authorized {
		return ChangeRequest{}, fmt.Errorf("change request %q has no rollout authorization", id)
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
	now := r.now().UTC()
	request.AuthorizationState = state
	request.AuditHistory = append(request.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: action})
	r.requests[id] = request
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
	requests, err := r.CreateChangeRequests(plan, actorID)
	if err != nil {
		return ChangeRequest{}, err
	}
	if len(requests) != 1 {
		return ChangeRequest{}, fmt.Errorf("baseline adoption requires at least one high-risk resource")
	}
	r.mu.Lock()
	request := r.requests[requests[0].ID]
	request.AuditHistory = append(request.AuditHistory, AuditEntry{At: r.now().UTC(), ActorID: actorID, Action: AuditBaselineAdoption})
	r.requests[request.ID] = request
	r.mu.Unlock()
	return cloneRequest(request), nil
}
