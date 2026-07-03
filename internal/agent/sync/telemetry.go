package sync

import (
	"encoding/json"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
)

// SystemInfoPayload is machine inventory telemetry reported on sync.
type SystemInfoPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}

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

type CronFailurePayload struct {
	ResourceAddress string `json:"resourceAddress"`
	Message         string `json:"message"`
}

type CronResultPayload struct {
	RunID       string               `json:"runId"`
	CronName    string               `json:"cronName"`
	Status      string               `json:"status"`
	StartedAt   time.Time            `json:"startedAt,omitempty"`
	CompletedAt time.Time            `json:"completedAt,omitempty"`
	Message     string               `json:"message,omitempty"`
	Failures    []CronFailurePayload `json:"failures,omitempty"`
}

type DueCronPayload struct {
	RunID        string `json:"runId"`
	CronName     string `json:"cronName"`
	ScheduledFor string `json:"scheduledFor"`
	SpecYAML     []byte `json:"specYaml"`
}

type DiagnosticCollectionPayload struct {
	RequestID  string    `json:"requestId"`
	Collectors []string  `json:"collectors"`
	Since      time.Time `json:"since"`
	Until      time.Time `json:"until"`
}

type DiagnosticResultPayload struct {
	RequestID string `json:"requestId"`
	Status    string `json:"status"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	Message   string `json:"message,omitempty"`
}

// FirewallAuditPayload is firewall audit log telemetry reported on sync.
type FirewallAuditPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
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
	CronResults        []CronResultPayload        `json:"cronResults,omitempty"`
	CronsDigest        string                     `json:"cronsDigest,omitempty"`
	SystemInfo         *SystemInfoPayload         `json:"systemInfo,omitempty"`
	Usernames          []string                   `json:"usernames,omitempty"`
	DiagnosticResult   *DiagnosticResultPayload   `json:"diagnosticResult,omitempty"`
	FirewallAudit      *FirewallAuditPayload      `json:"firewallAudit,omitempty"`
}

// Pending holds telemetry to send on the next sync after a pipeline run.
type Pending struct {
	Labels             map[string]string
	AgentUpgradeStatus *AgentUpgradeStatusPayload
	Drift              *DriftPayload
	ApplyFailure       *ApplyFailurePayload
	CronResults        []CronResultPayload
	CronsDigest        string
	SystemInfo         *SystemInfoPayload
	DiagnosticResult   *DiagnosticResultPayload
	FirewallAudit      *FirewallAuditPayload
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
		CronResults:        p.CronResults,
		CronsDigest:        p.CronsDigest,
		SystemInfo:         p.SystemInfo,
		DiagnosticResult:   p.DiagnosticResult,
		FirewallAudit:      p.FirewallAudit,
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
	if len(sent.CronResults) > 0 {
		p.CronResults = nil
	}
	if sent.SystemInfo != nil {
		p.SystemInfo = nil
	}
	if sent.DiagnosticResult != nil {
		p.DiagnosticResult = nil
	}
	if sent.FirewallAudit != nil {
		p.FirewallAudit = nil
	}
}

// AddCronResult queues cron execution telemetry for the next sync.
func (p *Pending) AddCronResult(result CronResultPayload) {
	p.CronResults = append(p.CronResults, result)
}

// SetDiagnosticResult queues diagnostic completion telemetry for the next sync.
func (p *Pending) SetDiagnosticResult(result DiagnosticResultPayload) {
	p.DiagnosticResult = &result
}

// SetFirewallAudit queues firewall audit log telemetry for the next sync.
func (p *Pending) SetFirewallAudit(digest string, report json.RawMessage) {
	if digest == "" && len(report) == 0 {
		p.FirewallAudit = nil
		return
	}
	p.FirewallAudit = &FirewallAuditPayload{
		Digest: digest,
		Report: report,
	}
}

// SetCronsDigest records the active crons artifact digest from the server.
func (p *Pending) SetCronsDigest(digest string) {
	p.CronsDigest = digest
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

// SetSystemInfo queues machine inventory telemetry for the next sync.
func (p *Pending) SetSystemInfo(digest string, report json.RawMessage) {
	if digest == "" && len(report) == 0 {
		p.SystemInfo = nil
		return
	}
	p.SystemInfo = &SystemInfoPayload{
		Digest: digest,
		Report: report,
	}
}

// SetFromPipeline updates pending telemetry from a pipeline result.
func (p *Pending) SetFromPipeline(labels map[string]string, drift engine.DriftReport, failed *engine.ApplyFailure, digest string) {
	p.Labels = labels
	p.Drift = driftPayload(drift, digest)
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
