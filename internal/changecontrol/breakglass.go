package changecontrol

import (
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/models"
)

const defaultBreakGlassValidity = time.Hour

type BreakGlassSafeguards struct {
	SchemaValid           bool `json:"schema_valid"`
	ProviderValid         bool `json:"provider_valid"`
	RedactionEnabled      bool `json:"redaction_enabled"`
	CurrentPreflightReady bool `json:"current_preflight_ready"`
	RequiredRollbackReady bool `json:"required_rollback_ready"`
	StableDeviceIdentity  bool `json:"stable_device_identity,omitempty"`
	IrreversibleApproved  bool `json:"irreversible_approved,omitempty"`
}

type BreakGlassSpec struct {
	Fleet             string               `json:"fleet"`
	EndpointIDs       []string             `json:"endpoint_ids"`
	FleetScope        bool                 `json:"fleet_scope,omitempty"`
	ResourceHashes    map[string]string    `json:"resource_hashes"`
	Risk              models.RiskClass     `json:"risk"`
	Justification     string               `json:"justification"`
	ExternalReference string               `json:"external_reference"`
	AttemptLimit      int                  `json:"attempt_limit,omitempty"`
	Validity          time.Duration        `json:"validity,omitempty"`
	Safeguards        BreakGlassSafeguards `json:"safeguards"`
}

type BreakGlassAuthorization struct {
	ID                string            `json:"id"`
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
	if !r.canBreakGlass(actorID, spec.Fleet, spec.Risk) {
		return BreakGlassAuthorization{}, fmt.Errorf("actor is not authorized for break glass")
	}
	if spec.Fleet == "" || len(spec.EndpointIDs) == 0 || len(spec.ResourceHashes) == 0 || spec.Justification == "" || spec.ExternalReference == "" {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass requires fleet, exact targets and hashes, justification, and external reference")
	}
	if !spec.FleetScope && len(spec.EndpointIDs) != 1 {
		return BreakGlassAuthorization{}, fmt.Errorf("endpoint break glass is limited to one endpoint")
	}
	if spec.FleetScope && (secondOperatorID == "" || secondOperatorID == actorID || !r.canBreakGlass(secondOperatorID, spec.Fleet, spec.Risk)) {
		return BreakGlassAuthorization{}, fmt.Errorf("fleet break glass requires a second distinct authorized operator")
	}
	if err := validateBreakGlassSafeguards(spec.Risk, spec.Safeguards); err != nil {
		return BreakGlassAuthorization{}, err
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
	authorization := BreakGlassAuthorization{ID: r.newID(), Fleet: spec.Fleet, EndpointIDs: append([]string(nil), spec.EndpointIDs...), FleetScope: spec.FleetScope, ResourceHashes: cloneHashes(spec.ResourceHashes), Risk: spec.Risk, Justification: spec.Justification, ExternalReference: spec.ExternalReference, Operators: operators, AttemptLimit: spec.AttemptLimit, CreatedAt: now, ExpiresAt: now.Add(spec.Validity), AuditHistory: []AuditEntry{{At: now, ActorID: actorID, Action: AuditBreakGlassCreated, Details: spec.ExternalReference}}}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.breakGlass[authorization.ID] = cloneBreakGlass(authorization)
	return cloneBreakGlass(authorization), nil
}

func (r *Registry) UseBreakGlass(id, endpointID string, hashes map[string]string) (BreakGlassAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.breakGlass[id]
	if !ok {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization %q not found", id)
	}
	now := r.now().UTC()
	if !now.Before(a.ExpiresAt) {
		a.AuditHistory = append(a.AuditHistory, AuditEntry{At: now, Action: AuditBreakGlassExpired})
		r.breakGlass[id] = a
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization expired")
	}
	if a.Revoked || a.Attempts >= a.AttemptLimit || !containsString(a.EndpointIDs, endpointID) || !equalHashes(a.ResourceHashes, hashes) {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization is not valid for this attempt")
	}
	a.Attempts++
	a.AuditHistory = append(a.AuditHistory, AuditEntry{At: now, Action: AuditBreakGlassUsed, Details: endpointID})
	r.breakGlass[id] = a
	return cloneBreakGlass(a), nil
}

func (r *Registry) RevokeBreakGlass(id, actorID string) (BreakGlassAuthorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.breakGlass[id]
	if !ok {
		return BreakGlassAuthorization{}, fmt.Errorf("break glass authorization %q not found", id)
	}
	a.Revoked = true
	a.AuditHistory = append(a.AuditHistory, AuditEntry{At: r.now().UTC(), ActorID: actorID, Action: AuditBreakGlassRevoked})
	r.breakGlass[id] = a
	return cloneBreakGlass(a), nil
}

func validateBreakGlassSafeguards(risk models.RiskClass, s BreakGlassSafeguards) error {
	if !s.SchemaValid || !s.ProviderValid || !s.RedactionEnabled || !s.CurrentPreflightReady || !s.RequiredRollbackReady {
		return fmt.Errorf("break glass cannot bypass validation, redaction, preflight, or rollback safeguards")
	}
	if risk == models.RiskDestructive && (!s.StableDeviceIdentity || !s.IrreversibleApproved) {
		return fmt.Errorf("destructive break glass requires stable device identity and irreversible approval")
	}
	return nil
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
