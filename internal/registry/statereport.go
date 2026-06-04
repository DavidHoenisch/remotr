package registry

import "time"

// StateReportItem is one drift finding from an agent Check.
type StateReportItem struct {
	Address     string `json:"address"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// StateReport is compliance evidence for one endpoint.
type StateReport struct {
	EndpointID   string             `json:"endpoint_id"`
	Fleet        string             `json:"fleet"`
	ReleaseRef   string             `json:"release_ref,omitempty"`
	Digest       string             `json:"digest,omitempty"`
	ReportedAt   time.Time          `json:"reported_at,omitempty"`
	InCompliance bool               `json:"in_compliance"`
	Items        []StateReportItem  `json:"items"`
	ApplyFailure *ApplyFailureSummary `json:"apply_failure,omitempty"`
}

// HasReport reports whether the endpoint has stored check evidence.
func (r StateReport) HasReport() bool {
	return !r.ReportedAt.IsZero()
}

// FleetStateSummary counts fleet compliance buckets.
type FleetStateSummary struct {
	Total     int `json:"total"`
	Compliant int `json:"compliant"`
	Drift     int `json:"drift"`
	NoReport  int `json:"no_report"`
}

// FleetStateReport aggregates state reports for one fleet.
type FleetStateReport struct {
	Fleet     string            `json:"fleet"`
	Summary   FleetStateSummary `json:"summary"`
	Endpoints []StateReport     `json:"endpoints"`
}
