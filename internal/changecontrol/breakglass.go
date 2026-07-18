package changecontrol

import (
	"fmt"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const defaultBreakGlassValidity = time.Hour

type BreakGlassSpec struct {
	ChangeRequestID   string        `json:"change_request_id"`
	Fleet             string        `json:"fleet"`
	EndpointIDs       []string      `json:"endpoint_ids"`
	FleetScope        bool          `json:"fleet_scope,omitempty"`
	Justification     string        `json:"justification"`
	ExternalReference string        `json:"external_reference"`
	AttemptLimit      int           `json:"attempt_limit,omitempty"`
	Validity          time.Duration `json:"validity,omitempty"`
}

type BreakGlassAuthorization struct {
	ID                string            `json:"id"`
	ChangeRequestID   string            `json:"change_request_id,omitempty"`
	Fleet             string            `json:"fleet"`
	EndpointIDs       []string          `json:"endpoint_ids"`
	FleetScope        bool              `json:"fleet_scope"`
	ResourceHashes    map[string]string `json:"resource_hashes"`
	Risk              models.RiskClass  `json:"risk"`
	Justification     string            `json:"justification"`
	ExternalReference string            `json:"external_reference"`
	Operators         []string          `json:"operators"`
	AttemptLimit      int               `json:"attempt_limit"`
	Attempts          int               `json:"attempts"`
	CreatedAt         time.Time         `json:"created_at"`
	ExpiresAt         time.Time         `json:"expires_at"`
	Revoked           bool              `json:"revoked"`
	AuditHistory      []AuditEntry      `json:"audit_history"`
}

func (r *Registry) CreateBreakGlass(spec BreakGlassSpec, actorID, secondOperatorID string) (BreakGlassAuthorization, error) {
	request, ok := r.Get(spec.ChangeRequestID)
	if !ok {
		return BreakGlassAuthorization{}, fmt.Errorf("canonical Change request %q not found", spec.ChangeRequestID)
	}
	if err := validateCanonicalBreakGlassRequest(request); err != nil {
		return BreakGlassAuthorization{}, err
	}
	if !r.canBreakGlass(actorID, request.Fleet, request.Risk) {
		return BreakGlassAuthorization{}, fmt.Errorf("actor is not authorized for break glass")
	}
	if spec.Fleet != "" || len(spec.EndpointIDs) == 0 || strings.TrimSpace(spec.Justification) == "" || strings.TrimSpace(spec.ExternalReference) == "" {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass requires a canonical request, exact targets, justification, and external reference; fleet is server-derived")
	}
	if !spec.FleetScope && len(spec.EndpointIDs) != 1 {
		return BreakGlassAuthorization{}, fmt.Errorf("endpoint break glass is limited to one endpoint")
	}
	if spec.FleetScope || request.Risk == models.RiskDestructive {
		if secondOperatorID == "" || secondOperatorID == actorID || !r.canBreakGlass(secondOperatorID, request.Fleet, request.Risk) {
			return BreakGlassAuthorization{}, fmt.Errorf("fleet or destructive break glass requires a second distinct authorized operator")
		}
	}
	seenTargets := make(map[string]struct{}, len(spec.EndpointIDs))
	for _, endpointID := range spec.EndpointIDs {
		if _, exists := seenTargets[endpointID]; exists {
			return BreakGlassAuthorization{}, fmt.Errorf("duplicate break-glass endpoint %q", endpointID)
		}
		if !canonicalBreakGlassTargetReady(request, endpointID) {
			return BreakGlassAuthorization{}, fmt.Errorf("endpoint %q lacks canonical current safety evidence", endpointID)
		}
		seenTargets[endpointID] = struct{}{}
	}
	if spec.AttemptLimit == 0 {
		spec.AttemptLimit = 1
	}
	if spec.AttemptLimit < 1 {
		return BreakGlassAuthorization{}, fmt.Errorf("attempt limit must be positive")
	}
	if spec.Validity == 0 {
		spec.Validity = defaultBreakGlassValidity
	}
	if spec.Validity <= 0 || spec.Validity > defaultBreakGlassValidity {
		return BreakGlassAuthorization{}, fmt.Errorf("validity must be at most 60 minutes")
	}
	now := r.now().UTC()
	operators := []string{actorID}
	if secondOperatorID != "" {
		operators = append(operators, secondOperatorID)
	}
	authorization := BreakGlassAuthorization{ID: r.newID(), ChangeRequestID: request.ID, Fleet: request.Fleet, EndpointIDs: append([]string(nil), spec.EndpointIDs...), FleetScope: spec.FleetScope, ResourceHashes: cloneHashes(request.ResourceHashes), Risk: request.Risk, Justification: spec.Justification, ExternalReference: spec.ExternalReference, Operators: operators, AttemptLimit: spec.AttemptLimit, CreatedAt: now, ExpiresAt: now.Add(spec.Validity), AuditHistory: []AuditEntry{{At: now, ActorID: actorID, Action: AuditBreakGlassCreated, Details: spec.ExternalReference}}}
	if strings.TrimSpace(authorization.ID) == "" {
		return BreakGlassAuthorization{}, fmt.Errorf("break-glass authorization id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.breakGlass[authorization.ID]; exists {
		return BreakGlassAuthorization{}, fmt.Errorf("duplicate break-glass authorization id %q", authorization.ID)
	}
	previous := r.snapshotLocked()
	r.breakGlass[authorization.ID] = cloneBreakGlass(authorization)
	if err := r.persistLocked(previous); err != nil {
		return BreakGlassAuthorization{}, err
	}
	return cloneBreakGlass(authorization), nil
}

func (r *Registry) UseBreakGlass(id string, preflight PreflightReport) (BreakGlassAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.breakGlass[id]
	if !ok {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization %q not found", id)
	}
	now := r.now().UTC()
	if !now.Before(a.ExpiresAt) {
		previous := r.snapshotLocked()
		a.AuditHistory = append(a.AuditHistory, AuditEntry{At: now, Action: AuditBreakGlassExpired})
		r.breakGlass[id] = a
		if err := r.persistLocked(previous); err != nil {
			return BreakGlassAuthorization{}, err
		}
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization expired")
	}
	request, requestExists := r.requests[a.ChangeRequestID]
	if a.ChangeRequestID == "" || !requestExists || preflight.ChangeRequestID != a.ChangeRequestID || !preflight.Ready || preflight.Reason != "" ||
		a.Revoked || a.Attempts >= a.AttemptLimit || !containsString(a.EndpointIDs, preflight.EndpointID) || !equalHashes(a.ResourceHashes, preflight.ResourceHashes) ||
		!canonicalBreakGlassTargetReady(request, preflight.EndpointID) {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization is not valid for this attempt")
	}
	previous := r.snapshotLocked()
	a.Attempts++
	a.AuditHistory = append(a.AuditHistory, AuditEntry{At: now, Action: AuditBreakGlassUsed, Details: preflight.EndpointID})
	r.breakGlass[id] = a
	if err := r.persistLocked(previous); err != nil {
		return BreakGlassAuthorization{}, err
	}
	return cloneBreakGlass(a), nil
}

func (r *Registry) RevokeBreakGlass(id, actorID string) (BreakGlassAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.breakGlass[id]
	if !ok {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization %q not found", id)
	}
	previous := r.snapshotLocked()
	a.Revoked = true
	a.AuditHistory = append(a.AuditHistory, AuditEntry{At: r.now().UTC(), ActorID: actorID, Action: AuditBreakGlassRevoked})
	r.breakGlass[id] = a
	if err := r.persistLocked(previous); err != nil {
		return BreakGlassAuthorization{}, err
	}
	return cloneBreakGlass(a), nil
}

func validateCanonicalBreakGlassRequest(request ChangeRequest) error {
	if request.HashContractVersion != effectivehash.SchemaVersion || len(request.Resources) == 0 || len(request.ResourceHashes) != len(request.Resources) {
		return fmt.Errorf("break glass requires a canonical version-%d Change request", effectivehash.SchemaVersion)
	}
	for _, resource := range request.Resources {
		if request.ResourceHashes[resource.Address] != resource.DesiredHash || effectivehash.Validate(resource.DesiredHash) != nil || strings.TrimSpace(resource.Provider) == "" || strings.TrimSpace(resource.ProviderRevision) == "" {
			return fmt.Errorf("break glass resource %q lacks canonical hash and provider evidence", resource.Address)
		}
		switch executor.RollbackClass(resource.RollbackClass) {
		case executor.RollbackNone, executor.RollbackBestEffort, executor.RollbackTransactional:
		default:
			return fmt.Errorf("break glass resource %q has invalid rollback evidence", resource.Address)
		}
		for _, effect := range resource.PredictedEffects {
			if err := effect.Validate(); err != nil {
				return fmt.Errorf("break glass resource %q has unsafe effect evidence: %w", resource.Address, err)
			}
		}
	}
	return nil
}

func canonicalBreakGlassTargetReady(request ChangeRequest, endpointID string) bool {
	for _, target := range request.FrozenTargets {
		if target.EndpointID != endpointID || !target.Compatible || !target.PreflightReady || len(target.ResourcePreflights) == 0 {
			continue
		}
		preflights := make(map[string]ResourcePreflightEvidence, len(target.ResourcePreflights))
		for _, evidence := range target.ResourcePreflights {
			preflights[evidence.Address] = evidence
		}
		ready := true
		for _, resource := range request.Resources {
			if !resource.Risk.RequiresPreflight() {
				continue
			}
			evidence, ok := preflights[resource.Address]
			if !ok || !evidence.Ready || evidence.Reason != "" {
				ready = false
				break
			}
		}
		return ready
	}
	return false
}

func equalHashes(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func cloneBreakGlass(a BreakGlassAuthorization) BreakGlassAuthorization {
	a.EndpointIDs = append([]string(nil), a.EndpointIDs...)
	a.ResourceHashes = cloneHashes(a.ResourceHashes)
	a.Operators = append([]string(nil), a.Operators...)
	a.AuditHistory = append([]AuditEntry(nil), a.AuditHistory...)
	return a
}
