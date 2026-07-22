package registry

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// StateReportStatus classifies an endpoint's latest compliance evidence.
type StateReportStatus string

const (
	StateCompliant   StateReportStatus = "compliant"
	StateDrifted     StateReportStatus = "drifted"
	StateUnsupported StateReportStatus = "unsupported"
	StateCheckFailed StateReportStatus = "check_failed"
	StateDeferred    StateReportStatus = "deferred"
	StateApplyFailed StateReportStatus = "apply_failed"
	StateNoReport    StateReportStatus = "no_report"
)

// PlanPreflightStatus is the closed non-enforcing provider readiness state
// admitted from current authenticated endpoint reports.
type PlanPreflightStatus string

const (
	PlanPreflightNotRequired PlanPreflightStatus = "not_required"
	PlanPreflightReady       PlanPreflightStatus = "ready"
	PlanPreflightBlocked     PlanPreflightStatus = "blocked"
)

// StateReportItem is one structured resource Check outcome.
type StateReportItem struct {
	Address             string                 `json:"address"`
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	Provider            string                 `json:"provider,omitempty"`
	ProviderRevision    string                 `json:"providerRevision,omitempty"`
	EffectiveHash       string                 `json:"effectiveHash,omitempty"`
	EffectiveHashStatus string                 `json:"effectiveHashStatus,omitempty"`
	Status              StateReportStatus      `json:"status,omitempty"`
	ReasonCode          string                 `json:"reasonCode,omitempty"`
	PreflightStatus     PlanPreflightStatus    `json:"preflightStatus,omitempty"`
	PreflightReason     string                 `json:"preflightReason,omitempty"`
	DesiredSummary      executor.SafeSummary   `json:"desiredSummary,omitempty"`
	ObservedSummary     executor.SafeSummary   `json:"observedSummary,omitempty"`
	Subresults          []StateReportSubresult `json:"subresults,omitempty"`
	SubresultsTruncated bool                   `json:"subresultsTruncated,omitempty"`
}

// StateReportSubresult is one bounded, redacted target outcome nested below a
// resource state report item.
type StateReportSubresult struct {
	Target          string               `json:"target"`
	Status          StateReportStatus    `json:"status"`
	ReasonCode      string               `json:"reasonCode"`
	DesiredSummary  executor.SafeSummary `json:"desiredSummary,omitempty"`
	ObservedSummary executor.SafeSummary `json:"observedSummary,omitempty"`
}

// StateReportActivation is one deferred activation requested by a resource.
type StateReportActivation struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
}

// StateReportApplyItem is the redacted mutation outcome for one resource.
type StateReportApplyItem struct {
	Address          string                  `json:"address"`
	Name             string                  `json:"name"`
	Provider         string                  `json:"provider,omitempty"`
	ProviderRevision string                  `json:"providerRevision,omitempty"`
	EffectiveHash    string                  `json:"effectiveHash,omitempty"`
	Status           string                  `json:"status"`
	ReasonCode       string                  `json:"reasonCode,omitempty"`
	DesiredSummary   executor.SafeSummary    `json:"desiredSummary,omitempty"`
	ObservedSummary  executor.SafeSummary    `json:"observedSummary,omitempty"`
	Activation       []StateReportActivation `json:"activation,omitempty"`
	RebootRequired   string                  `json:"rebootRequired,omitempty"`
	RollbackClass    string                  `json:"rollbackClass,omitempty"`
	RollbackStatus   string                  `json:"rollbackStatus,omitempty"`
	Diagnostics      []executor.SafeSummary  `json:"diagnostics,omitempty"`
}

// StateReportScheduleRuntime is optional endpoint-local execution history. A
// failed run is operational evidence and is not a compliance classification.
type StateReportScheduleRuntime struct {
	Address           string `json:"address"`
	Name              string `json:"name"`
	Provider          string `json:"provider,omitempty"`
	Status            string `json:"status"`
	ExitCode          *int   `json:"exitCode,omitempty"`
	MissedRunBehavior string `json:"missedRunBehavior"`
}

// StateReportRebootSource identifies a resource whose successful Apply
// reported that the endpoint requires a reboot.
type StateReportRebootSource struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// StateReportRebootIntent describes one durable coordinated reboot operation.
// Reason is a stable, redacted code rather than provider output.
type StateReportRebootIntent struct {
	Generation        string    `json:"generation"`
	Phase             string    `json:"phase"`
	PriorBootID       string    `json:"priorBootId"`
	CurrentBootID     string    `json:"currentBootId,omitempty"`
	PreparedAt        time.Time `json:"preparedAt"`
	NotBefore         time.Time `json:"notBefore"`
	Deadline          time.Time `json:"deadline,omitempty"`
	AttemptedAt       time.Time `json:"attemptedAt,omitempty"`
	AttemptDeadline   time.Time `json:"attemptDeadline,omitempty"`
	AttemptGeneration uint64    `json:"attemptGeneration,omitempty"`
	Reason            string    `json:"reason,omitempty"`
}

// StateReportRebootCompletion is boot-ID-verified completion evidence.
type StateReportRebootCompletion struct {
	Generation        string    `json:"generation"`
	BootID            string    `json:"bootId"`
	AttemptGeneration uint64    `json:"attemptGeneration"`
	CompletedAt       time.Time `json:"completedAt"`
}

// StateReportRebootRequired is durable operational state. It does not imply
// that a reboot has been authorized or initiated.
type StateReportRebootRequired struct {
	Required          bool                         `json:"required"`
	Sources           []StateReportRebootSource    `json:"sources,omitempty"`
	Intent            *StateReportRebootIntent     `json:"intent,omitempty"`
	Completion        *StateReportRebootCompletion `json:"completion,omitempty"`
	AttemptGeneration uint64                       `json:"attemptGeneration,omitempty"`
}

// StateReportPayload is the stored form of agent compliance telemetry.
type StateReportPayload struct {
	SchemaVersion   int                          `json:"schemaVersion,omitempty"`
	InCompliance    bool                         `json:"inCompliance"`
	Items           []StateReportItem            `json:"items"`
	Apply           []StateReportApplyItem       `json:"apply,omitempty"`
	ScheduleRuntime []StateReportScheduleRuntime `json:"scheduleRuntime,omitempty"`
	RebootRequired  *StateReportRebootRequired   `json:"rebootRequired,omitempty"`
}

// StateReport is compliance evidence for one endpoint.
type StateReport struct {
	EndpointID      string                       `json:"endpoint_id"`
	Fleet           string                       `json:"fleet"`
	SchemaVersion   int                          `json:"schema_version,omitempty"`
	ReleaseRef      string                       `json:"release_ref,omitempty"`
	Digest          string                       `json:"digest,omitempty"`
	ReportedAt      time.Time                    `json:"reported_at,omitempty"`
	InCompliance    bool                         `json:"in_compliance"`
	Status          StateReportStatus            `json:"status"`
	Items           []StateReportItem            `json:"items"`
	Apply           []StateReportApplyItem       `json:"apply,omitempty"`
	ScheduleRuntime []StateReportScheduleRuntime `json:"schedule_runtime,omitempty"`
	RebootRequired  *StateReportRebootRequired   `json:"reboot_required,omitempty"`
	ApplyFailure    *ApplyFailureSummary         `json:"apply_failure,omitempty"`
}

// HasReport reports whether the endpoint has stored check evidence.
func (r StateReport) HasReport() bool {
	return !r.ReportedAt.IsZero()
}

// ApplyFailureIsCurrent reports whether a persisted failure describes the
// State report's current evidence window. Historical failures remain endpoint
// history but do not override newer or different-Release State evidence.
func ApplyFailureIsCurrent(report StateReport, failure *ApplyFailureSummary) bool {
	if failure == nil || !report.HasReport() {
		return false
	}
	if report.ReleaseRef == "" || failure.ReleaseRef == "" || report.ReleaseRef != failure.ReleaseRef {
		return false
	}
	return !failure.ReportedAt.Before(report.ReportedAt)
}

// FleetStateSummary counts fleet compliance buckets.
type FleetStateSummary struct {
	Total       int `json:"total"`
	Compliant   int `json:"compliant"`
	Drift       int `json:"drift"`
	Unsupported int `json:"unsupported"`
	CheckFailed int `json:"check_failed"`
	Deferred    int `json:"deferred"`
	ApplyFailed int `json:"apply_failed"`
	NoReport    int `json:"no_report"`
}

// FirewallAuditReport is the latest firewall audit log from an endpoint.
type FirewallAuditReport struct {
	EndpointID string          `json:"endpoint_id"`
	Digest     string          `json:"digest,omitempty"`
	ReportedAt time.Time       `json:"reported_at,omitempty"`
	Report     json.RawMessage `json:"report,omitempty"`
}

// FleetStateReport aggregates state reports for one fleet.
type FleetStateReport struct {
	Fleet     string            `json:"fleet"`
	Summary   FleetStateSummary `json:"summary"`
	Endpoints []StateReport     `json:"endpoints"`
}

// ParseStateReportPayload accepts both legacy unversioned drift reports and
// the versioned structured telemetry emitted by current agents.
func ParseStateReportPayload(raw []byte) (StateReportPayload, error) {
	if len(raw) == 0 {
		return StateReportPayload{Items: []StateReportItem{}, Apply: []StateReportApplyItem{}, ScheduleRuntime: []StateReportScheduleRuntime{}}, nil
	}
	var header struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return StateReportPayload{}, err
	}
	if header.SchemaVersion < 7 {
		var err error
		raw, err = stripLegacyStateSummaries(raw)
		if err != nil {
			return StateReportPayload{}, err
		}
	}
	var payload StateReportPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return StateReportPayload{}, err
	}
	if payload.Items == nil {
		payload.Items = []StateReportItem{}
	}
	if payload.Apply == nil {
		payload.Apply = []StateReportApplyItem{}
	}
	if payload.ScheduleRuntime == nil {
		payload.ScheduleRuntime = []StateReportScheduleRuntime{}
	}
	if payload.SchemaVersion == 0 {
		for i := range payload.Items {
			if payload.Items[i].Status == "" {
				payload.Items[i].Status = StateDrifted
			}
			if payload.Items[i].ReasonCode == "" {
				payload.Items[i].ReasonCode = "legacy_drift"
			}
		}
	}
	if err := payload.Validate(); err != nil {
		return StateReportPayload{}, err
	}
	return payload, nil
}

// Validate proves every durable summary was admitted through a classified
// safe type before storage or output.
func (p StateReportPayload) Validate() error {
	canonical := make(map[string]StateReportItem, len(p.Items))
	hasActivationBootstrapIdentity := false
	for i, item := range p.Items {
		if p.SchemaVersion >= 8 {
			bootstrapIdentity := false
			var err error
			if p.SchemaVersion >= 10 {
				bootstrapIdentity, err = validateActivationBootstrapHashIdentity(item.Address, item.Provider, item.ProviderRevision, item.EffectiveHash, item.EffectiveHashStatus)
			} else {
				err = validateReportHashIdentity(item.Address, item.Provider, item.ProviderRevision, item.EffectiveHash)
			}
			if err != nil {
				return fmt.Errorf("items[%d]: %w", i, err)
			}
			hasActivationBootstrapIdentity = hasActivationBootstrapIdentity || bootstrapIdentity
			if _, exists := canonical[item.Address]; exists {
				return fmt.Errorf("items[%d]: duplicate resource address %q", i, item.Address)
			}
			canonical[item.Address] = item
		}
		if p.SchemaVersion >= 9 {
			if err := validatePlanPreflight(item.PreflightStatus, item.PreflightReason); err != nil {
				return fmt.Errorf("items[%d]: %w", i, err)
			}
		}
		if err := item.DesiredSummary.Validate(); err != nil {
			return fmt.Errorf("items[%d].desiredSummary: %w", i, err)
		}
		if err := item.ObservedSummary.Validate(); err != nil {
			return fmt.Errorf("items[%d].observedSummary: %w", i, err)
		}
		for j, subresult := range item.Subresults {
			if err := subresult.DesiredSummary.Validate(); err != nil {
				return fmt.Errorf("items[%d].subresults[%d].desiredSummary: %w", i, j, err)
			}
			if err := subresult.ObservedSummary.Validate(); err != nil {
				return fmt.Errorf("items[%d].subresults[%d].observedSummary: %w", i, j, err)
			}
		}
	}
	for i, item := range p.Apply {
		if p.SchemaVersion >= 8 {
			if err := validateReportHashIdentity(item.Address, item.Provider, item.ProviderRevision, item.EffectiveHash); err != nil {
				return fmt.Errorf("apply[%d]: %w", i, err)
			}
			checked, ok := canonical[item.Address]
			if !ok || checked.EffectiveHash != item.EffectiveHash || checked.Provider != item.Provider || checked.ProviderRevision != item.ProviderRevision {
				return fmt.Errorf("apply[%d]: resource hash identity does not match Check evidence", i)
			}
		}
		if err := item.DesiredSummary.Validate(); err != nil {
			return fmt.Errorf("apply[%d].desiredSummary: %w", i, err)
		}
		if err := item.ObservedSummary.Validate(); err != nil {
			return fmt.Errorf("apply[%d].observedSummary: %w", i, err)
		}
		for j, diagnostic := range item.Diagnostics {
			if err := diagnostic.Validate(); err != nil {
				return fmt.Errorf("apply[%d].diagnostics[%d]: %w", i, j, err)
			}
		}
	}
	if p.SchemaVersion >= 10 && !hasActivationBootstrapIdentity {
		return fmt.Errorf("schema version %d requires at least one authorization-blocked effective hash", p.SchemaVersion)
	}
	return nil
}

func validatePlanPreflight(status PlanPreflightStatus, reason string) error {
	switch status {
	case PlanPreflightNotRequired:
		if reason != "" {
			return fmt.Errorf("not-required preflight cannot have a reason")
		}
		return nil
	case PlanPreflightReady, PlanPreflightBlocked:
		if !validPlanReasonCode(reason) {
			return fmt.Errorf("ready or blocked preflight requires a stable reason code")
		}
		return nil
	default:
		return fmt.Errorf("unknown plan preflight status %q", status)
	}
}

func validPlanReasonCode(code string) bool {
	if len(code) == 0 || len(code) > 64 || code[0] < 'a' || code[0] > 'z' {
		return false
	}
	for _, character := range code {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func validateReportHashIdentity(address, provider, revision, hash string) error {
	for field, value := range map[string]string{"address": address, "provider": provider, "provider revision": revision} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > 512 {
			return fmt.Errorf("%s is required, trimmed, and bounded", field)
		}
	}
	if err := effectivehash.Validate(hash); err != nil {
		return err
	}
	return nil
}

func validateActivationBootstrapHashIdentity(address, provider, revision, hash, status string) (bool, error) {
	for field, value := range map[string]string{"address": address, "provider": provider, "provider revision": revision} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || len(value) > 512 {
			return false, fmt.Errorf("%s is required, trimmed, and bounded", field)
		}
	}
	if hash == "" {
		if status != "authorization_required" {
			return false, fmt.Errorf("missing effective hash requires authorization_required status")
		}
		return true, nil
	}
	if status != "" {
		return false, fmt.Errorf("resolved effective hash cannot carry a status")
	}
	if err := effectivehash.Validate(hash); err != nil {
		return false, err
	}
	return false, nil
}

func stripLegacyStateSummaries(raw []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if items, ok := payload["items"].([]any); ok {
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			delete(item, "desiredSummary")
			delete(item, "observedSummary")
			if subresults, ok := item["subresults"].([]any); ok {
				for _, rawSubresult := range subresults {
					subresult, _ := rawSubresult.(map[string]any)
					delete(subresult, "desiredSummary")
					delete(subresult, "observedSummary")
				}
			}
		}
	}
	if apply, ok := payload["apply"].([]any); ok {
		for _, rawItem := range apply {
			item, _ := rawItem.(map[string]any)
			delete(item, "desiredSummary")
			delete(item, "observedSummary")
			delete(item, "diagnostics")
		}
	}
	return json.Marshal(payload)
}

// ClassifyStateReport selects one mutually exclusive outcome bucket for an
// endpoint, preserving non-compliant outcomes rather than collapsing them to
// generic drift.
func ClassifyStateReport(report StateReport) StateReportStatus {
	if !report.HasReport() {
		return StateNoReport
	}
	if report.ApplyFailure != nil {
		return StateApplyFailed
	}
	for _, item := range report.Apply {
		if item.Status == "failed" {
			return StateApplyFailed
		}
	}
	var drifted, unsupported, deferred, checkFailed bool
	for _, item := range report.Items {
		switch item.Status {
		case StateCheckFailed:
			checkFailed = true
		case StateDeferred:
			deferred = true
		case StateUnsupported:
			unsupported = true
		case StateDrifted:
			drifted = true
		}
	}
	switch {
	case checkFailed:
		return StateCheckFailed
	case deferred:
		return StateDeferred
	case unsupported:
		return StateUnsupported
	case drifted || !report.InCompliance:
		return StateDrifted
	default:
		return StateCompliant
	}
}

// AddToFleetStateSummary records one endpoint's classified state.
func AddToFleetStateSummary(summary *FleetStateSummary, status StateReportStatus) {
	summary.Total++
	switch status {
	case StateCompliant:
		summary.Compliant++
	case StateDrifted:
		summary.Drift++
	case StateUnsupported:
		summary.Unsupported++
	case StateCheckFailed:
		summary.CheckFailed++
	case StateDeferred:
		summary.Deferred++
	case StateApplyFailed:
		summary.ApplyFailed++
	default:
		summary.NoReport++
	}
}
