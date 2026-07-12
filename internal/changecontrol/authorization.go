package changecontrol

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

const defaultRolloutValidity = 30 * 24 * time.Hour

// RecurringWindow is a weekly UTC execution window. StartMinuteUTC is minutes
// after midnight and Duration may cross midnight.
type RecurringWindow struct {
	Weekdays       []time.Weekday `json:"weekdays"`
	StartMinuteUTC int            `json:"start_minute_utc"`
	Duration       time.Duration  `json:"duration"`
}

func (w RecurringWindow) contains(at time.Time) bool {
	at = at.UTC()
	for dayOffset := 0; dayOffset >= -1; dayOffset-- {
		day := at.AddDate(0, 0, dayOffset)
		if !weekdayIncluded(w.Weekdays, day.Weekday()) {
			continue
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(w.StartMinuteUTC) * time.Minute)
		if !at.Before(start) && at.Before(start.Add(w.Duration)) {
			return true
		}
	}
	return false
}

type RolloutSpec struct {
	ValidFrom        time.Time         `json:"valid_from,omitempty"`
	ValidUntil       time.Time         `json:"valid_until,omitempty"`
	AttemptLimit     int               `json:"attempt_limit"`
	MaxConcurrency   int               `json:"max_concurrency"`
	ExecutionWindows []RecurringWindow `json:"execution_windows,omitempty"`
}

type RolloutAuthorization struct {
	ID               string            `json:"id"`
	ChangeRequestID  string            `json:"change_request_id"`
	Fleet            string            `json:"fleet"`
	ResourceHashes   map[string]string `json:"resource_hashes"`
	FrozenTargets    []TargetEvidence  `json:"frozen_targets"`
	ValidFrom        time.Time         `json:"valid_from"`
	ValidUntil       time.Time         `json:"valid_until"`
	AttemptLimit     int               `json:"attempt_limit"`
	MaxConcurrency   int               `json:"max_concurrency"`
	ExecutionWindows []RecurringWindow `json:"execution_windows,omitempty"`
	AuthorizedBy     string            `json:"authorized_by"`
	Justification    string            `json:"justification"`
	AuthorizedAt     time.Time         `json:"authorized_at"`
}

type BaselineAuthorization struct {
	ID                 string           `json:"id"`
	ChangeRequestID    string           `json:"change_request_id"`
	Fleet              string           `json:"fleet"`
	ResourceAddress    string           `json:"resource_address"`
	DesiredHash        string           `json:"desired_hash"`
	Risk               models.RiskClass `json:"risk"`
	Provider           string           `json:"provider"`
	AuthorizedBy       string           `json:"authorized_by"`
	AuthorizedAt       time.Time        `json:"authorized_at"`
	InvalidatedAt      time.Time        `json:"invalidated_at,omitempty"`
	InvalidationReason string           `json:"invalidation_reason,omitempty"`
	AuditHistory       []AuditEntry     `json:"audit_history"`
}

func (b BaselineAuthorization) Active() bool { return b.InvalidatedAt.IsZero() }

func (r *Registry) AuthorizeRollout(changeRequestID string, spec RolloutSpec, actorID, justification string) (RolloutAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return RolloutAuthorization{}, fmt.Errorf("change request %q not found", changeRequestID)
	}
	if _, exists := r.rollouts[changeRequestID]; exists {
		return RolloutAuthorization{}, fmt.Errorf("change request %q already has rollout authorization", changeRequestID)
	}
	now := r.now().UTC()
	if spec.ValidFrom.IsZero() {
		spec.ValidFrom = now
	} else {
		spec.ValidFrom = spec.ValidFrom.UTC()
	}
	if spec.ValidUntil.IsZero() {
		spec.ValidUntil = spec.ValidFrom.Add(defaultRolloutValidity)
	} else {
		spec.ValidUntil = spec.ValidUntil.UTC()
	}
	if !spec.ValidUntil.After(spec.ValidFrom) {
		return RolloutAuthorization{}, fmt.Errorf("rollout validity must end after it starts")
	}
	if spec.AttemptLimit <= 0 {
		spec.AttemptLimit = 1
	}
	if spec.MaxConcurrency <= 0 {
		spec.MaxConcurrency = 1
	}
	for _, window := range spec.ExecutionWindows {
		if window.StartMinuteUTC < 0 || window.StartMinuteUTC >= 24*60 || window.Duration <= 0 || len(window.Weekdays) == 0 {
			return RolloutAuthorization{}, fmt.Errorf("invalid recurring execution window")
		}
	}
	authorization := RolloutAuthorization{
		ID: r.newID(), ChangeRequestID: changeRequestID, Fleet: request.Fleet,
		ResourceHashes: cloneHashes(request.ResourceHashes), FrozenTargets: append([]TargetEvidence(nil), request.FrozenTargets...),
		ValidFrom: spec.ValidFrom, ValidUntil: spec.ValidUntil, AttemptLimit: spec.AttemptLimit, MaxConcurrency: spec.MaxConcurrency,
		ExecutionWindows: cloneWindows(spec.ExecutionWindows), AuthorizedBy: actorID, Justification: strings.TrimSpace(justification), AuthorizedAt: now,
	}
	request.AuthorizationState = AuthorizationActive
	request.AuditHistory = append(request.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: AuditRolloutAuthorized, Details: justification})
	r.requests[changeRequestID] = request
	r.rollouts[changeRequestID] = cloneRollout(authorization)
	return cloneRollout(authorization), nil
}

func (r *Registry) RolloutActive(changeRequestID string, at time.Time) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	authorization, ok := r.rollouts[changeRequestID]
	request := r.requests[changeRequestID]
	if !ok || request.AuthorizationState != AuthorizationActive || at.Before(authorization.ValidFrom) || !at.Before(authorization.ValidUntil) {
		return false
	}
	if len(authorization.ExecutionWindows) == 0 {
		return true
	}
	for _, window := range authorization.ExecutionWindows {
		if window.contains(at) {
			return true
		}
	}
	return false
}

func (r *Registry) PromoteBaseline(changeRequestID, resourceAddress, actorID string) (BaselineAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.requests[changeRequestID]
	if !ok {
		return BaselineAuthorization{}, fmt.Errorf("change request %q not found", changeRequestID)
	}
	if request.AuthorizationState != AuthorizationActive {
		return BaselineAuthorization{}, fmt.Errorf("change request %q is not authorized", changeRequestID)
	}
	var planned ResourcePlan
	found := false
	for _, resource := range request.Resources {
		if resource.Address == resourceAddress {
			planned, found = resource, true
			break
		}
	}
	if !found {
		return BaselineAuthorization{}, fmt.Errorf("resource %q is not in change request", resourceAddress)
	}
	if !planned.BaselineEligible || planned.Risk == models.RiskDestructive {
		return BaselineAuthorization{}, fmt.Errorf("resource %q is not baseline eligible", resourceAddress)
	}
	now := r.now().UTC()
	baseline := BaselineAuthorization{
		ID: r.newID(), ChangeRequestID: changeRequestID, Fleet: request.Fleet, ResourceAddress: resourceAddress,
		DesiredHash: planned.DesiredHash, Risk: planned.Risk, Provider: planned.Provider,
		AuthorizedBy: actorID, AuthorizedAt: now,
		AuditHistory: []AuditEntry{{At: now, ActorID: actorID, Action: AuditBaselinePromoted}},
	}
	r.baselines[baselineKey(request.Fleet, resourceAddress)] = cloneBaseline(baseline)
	request.AuditHistory = append(request.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: AuditBaselinePromoted, Details: resourceAddress})
	r.requests[changeRequestID] = request
	return cloneBaseline(baseline), nil
}

func (r *Registry) BaselineAuthorizes(fleet, resourceAddress, desiredHash, provider string, preflightReady bool) bool {
	if !preflightReady {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	baseline, ok := r.baselines[baselineKey(fleet, resourceAddress)]
	return ok && baseline.Active() && baseline.DesiredHash == desiredHash && baseline.Provider == provider
}

func (r *Registry) InvalidateBaselines(fleet, resourceAddress, currentHash, actorID string) []BaselineAuthorization {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := baselineKey(fleet, resourceAddress)
	baseline, ok := r.baselines[key]
	if !ok || !baseline.Active() || baseline.DesiredHash == currentHash {
		return []BaselineAuthorization{}
	}
	now := r.now().UTC()
	baseline.InvalidatedAt = now
	baseline.InvalidationReason = "desired hash changed"
	baseline.AuditHistory = append(baseline.AuditHistory, AuditEntry{At: now, ActorID: actorID, Action: AuditBaselineInvalidated, Details: currentHash})
	r.baselines[key] = baseline
	return []BaselineAuthorization{cloneBaseline(baseline)}
}

func weekdayIncluded(days []time.Weekday, day time.Weekday) bool {
	for _, candidate := range days {
		if candidate == day {
			return true
		}
	}
	return false
}

func baselineKey(fleet, address string) string { return fleet + "\x00" + address }

func cloneHashes(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneWindows(input []RecurringWindow) []RecurringWindow {
	out := make([]RecurringWindow, len(input))
	for i, window := range input {
		out[i] = window
		out[i].Weekdays = append([]time.Weekday(nil), window.Weekdays...)
		sort.Slice(out[i].Weekdays, func(a, b int) bool { return out[i].Weekdays[a] < out[i].Weekdays[b] })
	}
	return out
}

func cloneRollout(input RolloutAuthorization) RolloutAuthorization {
	input.ResourceHashes = cloneHashes(input.ResourceHashes)
	input.FrozenTargets = append([]TargetEvidence(nil), input.FrozenTargets...)
	input.ExecutionWindows = cloneWindows(input.ExecutionWindows)
	return input
}

func cloneBaseline(input BaselineAuthorization) BaselineAuthorization {
	input.AuditHistory = append([]AuditEntry(nil), input.AuditHistory...)
	return input
}
