package sync

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
)

const (
	// MaxComplianceReportBytes bounds one structured compliance payload sent on
	// Sync. The bound keeps telemetry growth from defeating unchanged Syncs.
	MaxComplianceReportBytes = 64 << 10

	maxComplianceReportItems  = 128
	maxComplianceApplyItems   = 64
	maxScheduleRuntimeItems   = 64
	maxRebootRequiredSources  = 64
	maxComplianceDiagnostics  = 3
	maxComplianceSummaryBytes = 256
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
	LastDigest         string                          `json:"lastDigest"`
	LastReleaseRef     string                          `json:"lastReleaseRef,omitempty"`
	Labels             map[string]string               `json:"labels,omitempty"`
	AgentVersion       string                          `json:"agentVersion,omitempty"`
	AgentUpgradeStatus *AgentUpgradeStatusPayload      `json:"agentUpgradeStatus,omitempty"`
	Drift              *DriftPayload                   `json:"drift,omitempty"`
	ApplyFailure       *ApplyFailurePayload            `json:"applyFailure,omitempty"`
	CronResults        []CronResultPayload             `json:"cronResults,omitempty"`
	CronsDigest        string                          `json:"cronsDigest,omitempty"`
	SystemInfo         *SystemInfoPayload              `json:"systemInfo,omitempty"`
	Usernames          []string                        `json:"usernames,omitempty"`
	DiagnosticResult   *DiagnosticResultPayload        `json:"diagnosticResult,omitempty"`
	FirewallAudit      *FirewallAuditPayload           `json:"firewallAudit,omitempty"`
	ChangePreflights   []changecontrol.PreflightReport `json:"changePreflights,omitempty"`
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
	RebootRequired     rebootstate.Status
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

// SetRebootRequired retains endpoint-local reboot-required evidence for the
// next state report. It does not authorize or initiate reboot execution.
func (p *Pending) SetRebootRequired(status rebootstate.Status) {
	p.RebootRequired = rebootstate.Status{Required: status.Required, Sources: append([]rebootstate.Source(nil), status.Sources...)}
}

// SetFromPipeline updates pending telemetry from a pipeline result.
func (p *Pending) SetFromPipeline(labels map[string]string, drift engine.DriftReport, applied engine.ApplyResult, failed *engine.ApplyFailure, digest string) {
	p.Labels = labels
	p.Drift = driftPayload(drift, applied, p.RebootRequired, digest)
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
	SchemaVersion   int                   `json:"schemaVersion"`
	InCompliance    bool                  `json:"inCompliance"`
	Items           []driftItemJSON       `json:"items"`
	Apply           []applyItemJSON       `json:"apply,omitempty"`
	ScheduleRuntime []scheduleRuntimeJSON `json:"scheduleRuntime,omitempty"`
	RebootRequired  *rebootRequiredJSON   `json:"rebootRequired,omitempty"`
	Truncated       bool                  `json:"truncated,omitempty"`
}

type rebootRequiredJSON struct {
	Required bool                       `json:"required"`
	Sources  []rebootRequiredSourceJSON `json:"sources,omitempty"`
}

type rebootRequiredSourceJSON struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type driftItemJSON struct {
	Address         string `json:"address"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Provider        string `json:"provider,omitempty"`
	Status          string `json:"status,omitempty"`
	ReasonCode      string `json:"reasonCode,omitempty"`
	DesiredSummary  string `json:"desiredSummary,omitempty"`
	ObservedSummary string `json:"observedSummary,omitempty"`
}

type activationJSON struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
}

type applyItemJSON struct {
	Address         string           `json:"address"`
	Name            string           `json:"name"`
	Provider        string           `json:"provider,omitempty"`
	Status          string           `json:"status"`
	ReasonCode      string           `json:"reasonCode,omitempty"`
	DesiredSummary  string           `json:"desiredSummary,omitempty"`
	ObservedSummary string           `json:"observedSummary,omitempty"`
	Activation      []activationJSON `json:"activation,omitempty"`
	RebootRequired  string           `json:"rebootRequired,omitempty"`
	RollbackClass   string           `json:"rollbackClass,omitempty"`
	RollbackStatus  string           `json:"rollbackStatus,omitempty"`
	Diagnostics     []string         `json:"diagnostics,omitempty"`
}

type scheduleRuntimeJSON struct {
	Address           string `json:"address"`
	Name              string `json:"name"`
	Provider          string `json:"provider,omitempty"`
	Status            string `json:"status"`
	ExitCode          *int   `json:"exitCode,omitempty"`
	MissedRunBehavior string `json:"missedRunBehavior"`
}

func driftPayload(drift engine.DriftReport, applied engine.ApplyResult, rebootRequired rebootstate.Status, digest string) *DriftPayload {
	itemCount := min(len(drift.Items), maxComplianceReportItems)
	items := make([]driftItemJSON, itemCount)
	truncated := itemCount < len(drift.Items)
	for i, item := range drift.Items[:itemCount] {
		address, addressTruncated := truncateComplianceText(item.Address)
		name, nameTruncated := truncateComplianceText(item.Name)
		description, descriptionTruncated := truncateComplianceText(item.Description)
		provider, providerTruncated := truncateComplianceText(item.Provider)
		status, statusTruncated := truncateComplianceText(string(item.Status))
		reasonCode, reasonCodeTruncated := truncateComplianceText(string(item.ReasonCode))
		desired, desiredTruncated := truncateComplianceText(string(item.DesiredSummary))
		observed, observedTruncated := truncateComplianceText(string(item.ObservedSummary))
		truncated = truncated || addressTruncated || nameTruncated || descriptionTruncated || providerTruncated || statusTruncated || reasonCodeTruncated || desiredTruncated || observedTruncated
		items[i] = driftItemJSON{
			Address:         address,
			Name:            name,
			Description:     description,
			Provider:        provider,
			Status:          status,
			ReasonCode:      reasonCode,
			DesiredSummary:  desired,
			ObservedSummary: observed,
		}
	}
	applyCount := min(len(applied.Items), maxComplianceApplyItems)
	apply := make([]applyItemJSON, applyCount)
	truncated = truncated || applyCount < len(applied.Items)
	for i, item := range applied.Items[:applyCount] {
		activations := make([]activationJSON, len(item.Activation))
		for j, activation := range item.Activation {
			kind, kindTruncated := truncateComplianceText(string(activation.Kind))
			target, targetTruncated := truncateComplianceText(activation.Target)
			truncated = truncated || kindTruncated || targetTruncated
			activations[j] = activationJSON{Kind: kind, Target: target}
		}
		diagnosticCount := min(len(item.Diagnostics), maxComplianceDiagnostics)
		diagnostics := make([]string, diagnosticCount)
		truncated = truncated || diagnosticCount < len(item.Diagnostics)
		for j, diagnostic := range item.Diagnostics[:diagnosticCount] {
			boundedDiagnostic, diagnosticTruncated := truncateComplianceText(string(diagnostic))
			diagnostics[j] = boundedDiagnostic
			truncated = truncated || diagnosticTruncated
		}
		address, addressTruncated := truncateComplianceText(item.Address)
		name, nameTruncated := truncateComplianceText(item.Name)
		provider, providerTruncated := truncateComplianceText(item.Provider)
		status, statusTruncated := truncateComplianceText(string(item.Status))
		reasonCode, reasonCodeTruncated := truncateComplianceText(string(item.ReasonCode))
		desired, desiredTruncated := truncateComplianceText(string(item.DesiredSummary))
		observed, observedTruncated := truncateComplianceText(string(item.ObservedSummary))
		rebootRequired, rebootRequiredTruncated := truncateComplianceText(string(item.RebootRequired))
		rollbackClass, rollbackClassTruncated := truncateComplianceText(string(item.RollbackClass))
		rollbackStatus, rollbackStatusTruncated := truncateComplianceText(string(item.RollbackStatus))
		truncated = truncated || addressTruncated || nameTruncated || providerTruncated || statusTruncated || reasonCodeTruncated || desiredTruncated || observedTruncated || rebootRequiredTruncated || rollbackClassTruncated || rollbackStatusTruncated
		apply[i] = applyItemJSON{
			Address:         address,
			Name:            name,
			Provider:        provider,
			Status:          status,
			ReasonCode:      reasonCode,
			DesiredSummary:  desired,
			ObservedSummary: observed,
			Activation:      activations,
			RebootRequired:  rebootRequired,
			RollbackClass:   rollbackClass,
			RollbackStatus:  rollbackStatus,
			Diagnostics:     diagnostics,
		}
	}
	runtimeCount := min(len(drift.ScheduleRuntime), maxScheduleRuntimeItems)
	runtime := make([]scheduleRuntimeJSON, runtimeCount)
	truncated = truncated || runtimeCount < len(drift.ScheduleRuntime)
	for i, item := range drift.ScheduleRuntime[:runtimeCount] {
		address, addressTruncated := truncateComplianceText(item.Address)
		name, nameTruncated := truncateComplianceText(item.Name)
		provider, providerTruncated := truncateComplianceText(item.Provider)
		status, statusTruncated := truncateComplianceText(string(item.Status))
		missedRun, missedRunTruncated := truncateComplianceText(string(item.MissedRunBehavior))
		truncated = truncated || addressTruncated || nameTruncated || providerTruncated || statusTruncated || missedRunTruncated
		runtime[i] = scheduleRuntimeJSON{
			Address: address, Name: name, Provider: provider, Status: status,
			ExitCode: item.ExitCode, MissedRunBehavior: missedRun,
		}
	}
	var pendingReboot *rebootRequiredJSON
	if rebootRequired.Required {
		sourceCount := min(len(rebootRequired.Sources), maxRebootRequiredSources)
		sources := make([]rebootRequiredSourceJSON, sourceCount)
		truncated = truncated || sourceCount < len(rebootRequired.Sources)
		for i, source := range rebootRequired.Sources[:sourceCount] {
			address, addressTruncated := truncateComplianceText(source.Address)
			name, nameTruncated := truncateComplianceText(source.Name)
			provider, providerTruncated := truncateComplianceText(source.Provider)
			truncated = truncated || addressTruncated || nameTruncated || providerTruncated
			sources[i] = rebootRequiredSourceJSON{Address: address, Name: name, Provider: provider}
		}
		pendingReboot = &rebootRequiredJSON{Required: true, Sources: sources}
	}
	payload := driftReportJSON{
		SchemaVersion:   4,
		InCompliance:    drift.InCompliance,
		Items:           items,
		Apply:           apply,
		ScheduleRuntime: runtime,
		RebootRequired:  pendingReboot,
		Truncated:       truncated,
	}
	raw, err := marshalBoundedCompliancePayload(payload)
	if err != nil {
		return nil
	}
	return &DriftPayload{Digest: digest, Report: raw}
}

func marshalBoundedCompliancePayload(payload driftReportJSON) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) <= MaxComplianceReportBytes {
		return raw, err
	}
	payload.Truncated = true
	for len(payload.ScheduleRuntime) > 0 && len(raw) > MaxComplianceReportBytes {
		payload.ScheduleRuntime = payload.ScheduleRuntime[:len(payload.ScheduleRuntime)-1]
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	for len(payload.Apply) > 0 && len(raw) > MaxComplianceReportBytes {
		payload.Apply = payload.Apply[:len(payload.Apply)-1]
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	for len(payload.Items) > 1 && len(raw) > MaxComplianceReportBytes {
		payload.Items = payload.Items[:len(payload.Items)-1]
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	for payload.RebootRequired != nil && len(payload.RebootRequired.Sources) > 1 && len(raw) > MaxComplianceReportBytes {
		payload.RebootRequired.Sources = payload.RebootRequired.Sources[:len(payload.RebootRequired.Sources)-1]
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	return raw, nil
}

func truncateComplianceText(value string) (string, bool) {
	if len(value) <= maxComplianceSummaryBytes {
		return value, false
	}
	end := maxComplianceSummaryBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], true
}
