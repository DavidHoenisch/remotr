package sync

import (
	"encoding/json"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
)

// DriftPayload is drift telemetry reported on sync (see POST /v1/sync).
type DriftPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}

// ApplyFailurePayload is apply failure telemetry reported on sync.
type ApplyFailurePayload struct {
	ResourceAddress string `json:"resourceAddress"`
	Message         string `json:"message"`
}

// AgentUpgradeStatusPayload reports upgrade progress to the server.
type AgentUpgradeStatusPayload struct {
	Desired string `json:"desired,omitempty"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// Request is the JSON body for POST /v1/sync.
type Request struct {
	LastDigest         string                     `json:"lastDigest"`
	LastReleaseRef     string                     `json:"lastReleaseRef,omitempty"`
	Labels             map[string]string          `json:"labels,omitempty"`
	AgentVersion       string                     `json:"agentVersion,omitempty"`
	AgentUpgradeStatus *AgentUpgradeStatusPayload `json:"agentUpgradeStatus,omitempty"`
	Drift              *DriftPayload              `json:"drift,omitempty"`
	ApplyFailure       *ApplyFailurePayload       `json:"applyFailure,omitempty"`
}

// Pending holds telemetry to send on the next sync after a pipeline run.
type Pending struct {
	Labels             map[string]string
	AgentUpgradeStatus *AgentUpgradeStatusPayload
	Drift              *DriftPayload
	ApplyFailure       *ApplyFailurePayload
}

// Request builds a sync request including pending telemetry and lastDigest.
func (p *Pending) Request(lastDigest, lastReleaseRef, agentVersion string) Request {
	return Request{
		LastDigest:         lastDigest,
		LastReleaseRef:     lastReleaseRef,
		Labels:             p.Labels,
		AgentVersion:       agentVersion,
		AgentUpgradeStatus: p.AgentUpgradeStatus,
		Drift:              p.Drift,
		ApplyFailure:       p.ApplyFailure,
	}
}

// ClearSent removes telemetry fields that were included in a successful sync request.
func (p *Pending) ClearSent(sent Request) {
	if sent.ApplyFailure != nil {
		p.ApplyFailure = nil
	}
	if sent.Drift != nil {
		p.Drift = nil
	}
	if sent.AgentUpgradeStatus != nil {
		p.AgentUpgradeStatus = nil
	}
}

// SetAgentUpgradeStatus queues upgrade telemetry for the next sync.
func (p *Pending) SetAgentUpgradeStatus(desired, phase, message string) {
	if desired == "" && phase == "" && message == "" {
		p.AgentUpgradeStatus = nil
		return
	}
	p.AgentUpgradeStatus = &AgentUpgradeStatusPayload{
		Desired: desired,
		Phase:   phase,
		Message: message,
	}
}

// SetFromPipeline updates pending telemetry from a pipeline result.
func (p *Pending) SetFromPipeline(labels map[string]string, drift engine.DriftReport, failed *engine.ApplyFailure, digest string) {
	p.Labels = labels
	if len(drift.Items) > 0 {
		p.Drift = driftPayload(drift, digest)
	} else {
		p.Drift = nil
	}
	if failed != nil {
		p.ApplyFailure = &ApplyFailurePayload{
			ResourceAddress: failed.Address,
			Message:         failed.Err.Error(),
		}
	} else {
		p.ApplyFailure = nil
	}
}

type driftReportJSON struct {
	InCompliance bool              `json:"inCompliance"`
	Items        []driftItemJSON   `json:"items"`
}

type driftItemJSON struct {
	Address     string `json:"address"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func driftPayload(drift engine.DriftReport, digest string) *DriftPayload {
	items := make([]driftItemJSON, len(drift.Items))
	for i, item := range drift.Items {
		items[i] = driftItemJSON{
			Address:     item.Address,
			Name:        item.Name,
			Description: item.Description,
		}
	}
	raw, err := json.Marshal(driftReportJSON{
		InCompliance: drift.InCompliance,
		Items:        items,
	})
	if err != nil {
		return nil
	}
	return &DriftPayload{Digest: digest, Report: raw}
}
