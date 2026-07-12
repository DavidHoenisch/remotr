package registry

import (
	"encoding/json"
	"time"
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

// StateReportItem is one structured resource Check outcome.
type StateReportItem struct {
	Address         string            `json:"address"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Provider        string            `json:"provider,omitempty"`
	Status          StateReportStatus `json:"status,omitempty"`
	ReasonCode      string            `json:"reasonCode,omitempty"`
	DesiredSummary  string            `json:"desiredSummary,omitempty"`
	ObservedSummary string            `json:"observedSummary,omitempty"`
}

// StateReportActivation is one deferred activation requested by a resource.
type StateReportActivation struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
}

// StateReportApplyItem is the redacted mutation outcome for one resource.
type StateReportApplyItem struct {
	Address         string                  `json:"address"`
	Name            string                  `json:"name"`
	Provider        string                  `json:"provider,omitempty"`
	Status          string                  `json:"status"`
	ReasonCode      string                  `json:"reasonCode,omitempty"`
	DesiredSummary  string                  `json:"desiredSummary,omitempty"`
	ObservedSummary string                  `json:"observedSummary,omitempty"`
	Activation      []StateReportActivation `json:"activation,omitempty"`
	RebootRequired  string                  `json:"rebootRequired,omitempty"`
	RollbackClass   string                  `json:"rollbackClass,omitempty"`
	RollbackStatus  string                  `json:"rollbackStatus,omitempty"`
	Diagnostics     []string                `json:"diagnostics,omitempty"`
}

// StateReportPayload is the stored form of agent compliance telemetry.
type StateReportPayload struct {
	SchemaVersion int                    `json:"schemaVersion,omitempty"`
	InCompliance  bool                   `json:"inCompliance"`
	Items         []StateReportItem      `json:"items"`
	Apply         []StateReportApplyItem `json:"apply,omitempty"`
}

// StateReport is compliance evidence for one endpoint.
type StateReport struct {
	EndpointID   string                 `json:"endpoint_id"`
	Fleet        string                 `json:"fleet"`
	ReleaseRef   string                 `json:"release_ref,omitempty"`
	Digest       string                 `json:"digest,omitempty"`
	ReportedAt   time.Time              `json:"reported_at,omitempty"`
	InCompliance bool                   `json:"in_compliance"`
	Status       StateReportStatus      `json:"status"`
	Items        []StateReportItem      `json:"items"`
	Apply        []StateReportApplyItem `json:"apply,omitempty"`
	ApplyFailure *ApplyFailureSummary   `json:"apply_failure,omitempty"`
}

// HasReport reports whether the endpoint has stored check evidence.
func (r StateReport) HasReport() bool {
	return !r.ReportedAt.IsZero()
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
		return StateReportPayload{Items: []StateReportItem{}, Apply: []StateReportApplyItem{}}, nil
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
	return payload, nil
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
