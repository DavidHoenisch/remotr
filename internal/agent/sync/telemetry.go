package sync

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/executor"
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
	ResourceAddress string             `json:"resourceAddress"`
	Failure         executor.SafeError `json:"failure"`
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
	RequestID string              `json:"requestId"`
	Status    string              `json:"status"`
	SHA256    string              `json:"sha256,omitempty"`
	SizeBytes int64               `json:"sizeBytes,omitempty"`
	Failure   *executor.SafeError `json:"failure,omitempty"`
}

// FirewallAuditPayload is firewall audit log telemetry reported on sync.
type FirewallAuditPayload struct {
	Digest string          `json:"digest,omitempty"`
	Report json.RawMessage `json:"report,omitempty"`
}

// RebootIntentPayload requests authenticated acknowledgement of a prepared
// local reboot intent. It does not itself authorize the server to reboot.
type RebootIntentPayload struct {
	Generation  string    `json:"generation"`
	Phase       string    `json:"phase"`
	PriorBootID string    `json:"priorBootId"`
	NotBefore   time.Time `json:"notBefore"`
	Deadline    time.Time `json:"deadline,omitempty"`
}

// NetworkIntentPayload requests authenticated acknowledgement after an armed
// connectivity transaction has re-established the Sync path.
type NetworkIntentPayload struct {
	ID            string    `json:"id"`
	Phase         string    `json:"phase"`
	Deadline      time.Time `json:"deadline"`
	PlanHash      string    `json:"planHash"`
	WatchdogArmed bool      `json:"watchdogArmed"`
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
	RebootIntent       *RebootIntentPayload            `json:"rebootIntent,omitempty"`
	NetworkIntent      *NetworkIntentPayload           `json:"networkIntent,omitempty"`
	CapabilityDocument *capabilitydoc.Document         `json:"capabilityDocument,omitempty"`
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
	RebootIntent       *RebootIntentPayload
	NetworkIntent      *NetworkIntentPayload
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
		RebootIntent:       p.RebootIntent,
		NetworkIntent:      p.NetworkIntent,
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
	if sent.RebootIntent != nil {
		p.RebootIntent = nil
	}
	if sent.NetworkIntent != nil {
		p.NetworkIntent = nil
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
	p.RebootRequired = rebootstate.Status{
		Required:          status.Required,
		Sources:           append([]rebootstate.Source(nil), status.Sources...),
		AttemptGeneration: status.AttemptGeneration,
	}
	if status.Intent != nil {
		intent := *status.Intent
		p.RebootRequired.Intent = &intent
	}
	if status.Completion != nil {
		completion := *status.Completion
		p.RebootRequired.Completion = &completion
	}
}

// SetRebootIntent queues or clears a prepared pre-reboot acknowledgement.
func (p *Pending) SetRebootIntent(intent *rebootstate.Intent) {
	if intent == nil {
		p.RebootIntent = nil
		return
	}
	p.RebootIntent = &RebootIntentPayload{
		Generation: intent.Generation, Phase: string(intent.Phase), PriorBootID: intent.PriorBootID,
		NotBefore: intent.NotBefore, Deadline: intent.Deadline,
	}
}

// SetNetworkIntent queues or clears an armed connectivity transaction.
func (p *Pending) SetNetworkIntent(intent *networkstate.Intent) {
	if intent == nil {
		p.NetworkIntent = nil
		return
	}
	p.NetworkIntent = &NetworkIntentPayload{
		ID: intent.ID, Phase: string(intent.Phase), Deadline: intent.Deadline,
		PlanHash: intent.PlanHash, WatchdogArmed: intent.WatchdogArmed,
	}
}

// SetFromPipeline updates pending telemetry from a pipeline result.
func (p *Pending) SetFromPipeline(labels map[string]string, drift engine.DriftReport, applied engine.ApplyResult, failed *engine.ApplyFailure, digest string) {
	p.Labels = labels
	p.Drift = driftPayload(drift, applied, p.RebootRequired, digest)
	if failed != nil {
		p.ApplyFailure = &ApplyFailurePayload{
			ResourceAddress: failed.Address,
			Failure:         failed.Err,
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
	Required          bool                       `json:"required"`
	Sources           []rebootRequiredSourceJSON `json:"sources,omitempty"`
	Intent            *rebootIntentJSON          `json:"intent,omitempty"`
	Completion        *rebootCompletionJSON      `json:"completion,omitempty"`
	AttemptGeneration uint64                     `json:"attemptGeneration,omitempty"`
}

type rebootRequiredSourceJSON struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

type rebootIntentJSON struct {
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

type rebootCompletionJSON struct {
	Generation        string    `json:"generation"`
	BootID            string    `json:"bootId"`
	AttemptGeneration uint64    `json:"attemptGeneration"`
	CompletedAt       time.Time `json:"completedAt"`
}

type driftItemJSON struct {
	Address             string               `json:"address"`
	Name                string               `json:"name"`
	Description         string               `json:"description"`
	Provider            string               `json:"provider,omitempty"`
	ProviderRevision    string               `json:"providerRevision,omitempty"`
	EffectiveHash       string               `json:"effectiveHash,omitempty"`
	EffectiveHashStatus string               `json:"effectiveHashStatus,omitempty"`
	Status              string               `json:"status,omitempty"`
	ReasonCode          string               `json:"reasonCode,omitempty"`
	PreflightStatus     string               `json:"preflightStatus,omitempty"`
	PreflightReason     string               `json:"preflightReason,omitempty"`
	DesiredSummary      executor.SafeSummary `json:"desiredSummary,omitempty"`
	ObservedSummary     executor.SafeSummary `json:"observedSummary,omitempty"`
	Subresults          []checkSubresultJSON `json:"subresults,omitempty"`
	SubresultsTruncated bool                 `json:"subresultsTruncated,omitempty"`
}

type checkSubresultJSON struct {
	Target          string               `json:"target"`
	Status          string               `json:"status"`
	ReasonCode      string               `json:"reasonCode"`
	DesiredSummary  executor.SafeSummary `json:"desiredSummary,omitempty"`
	ObservedSummary executor.SafeSummary `json:"observedSummary,omitempty"`
}

type activationJSON struct {
	Kind   string `json:"kind"`
	Target string `json:"target,omitempty"`
}

type applyItemJSON struct {
	Address          string                 `json:"address"`
	Name             string                 `json:"name"`
	Provider         string                 `json:"provider,omitempty"`
	ProviderRevision string                 `json:"providerRevision,omitempty"`
	EffectiveHash    string                 `json:"effectiveHash,omitempty"`
	Status           string                 `json:"status"`
	ReasonCode       string                 `json:"reasonCode,omitempty"`
	DesiredSummary   executor.SafeSummary   `json:"desiredSummary,omitempty"`
	ObservedSummary  executor.SafeSummary   `json:"observedSummary,omitempty"`
	Activation       []activationJSON       `json:"activation,omitempty"`
	RebootRequired   string                 `json:"rebootRequired,omitempty"`
	RollbackClass    string                 `json:"rollbackClass,omitempty"`
	RollbackStatus   string                 `json:"rollbackStatus,omitempty"`
	Diagnostics      []executor.SafeSummary `json:"diagnostics,omitempty"`
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
	activationBootstrap := hasActivationBootstrapEvidence(drift, applied)
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
		preflightStatus, preflightStatusTruncated := truncateComplianceText(string(item.PreflightStatus))
		preflightReason, preflightReasonTruncated := truncateComplianceText(string(item.PreflightReason))
		effectiveHashStatus := ""
		if activationBootstrap && item.EffectiveHashAuthorizationRequired {
			effectiveHashStatus = "authorization_required"
		}
		desired := item.DesiredSummary.Clone()
		observed := item.ObservedSummary.Clone()
		truncated = truncated || addressTruncated || nameTruncated || descriptionTruncated || providerTruncated || statusTruncated || reasonCodeTruncated || preflightStatusTruncated || preflightReasonTruncated
		subresultCount := min(len(item.Subresults), executor.MaxCheckSubresults)
		subresults := make([]checkSubresultJSON, subresultCount)
		truncated = truncated || item.SubresultsTruncated || subresultCount < len(item.Subresults)
		for j, subresult := range item.Subresults[:subresultCount] {
			target, targetTruncated := truncateComplianceText(subresult.Target)
			substatus, substatusTruncated := truncateComplianceText(string(subresult.Status))
			subreason, subreasonTruncated := truncateComplianceText(string(subresult.ReasonCode))
			subdesired := subresult.DesiredSummary.Clone()
			subobserved := subresult.ObservedSummary.Clone()
			truncated = truncated || targetTruncated || substatusTruncated || subreasonTruncated
			subresults[j] = checkSubresultJSON{Target: target, Status: substatus, ReasonCode: subreason, DesiredSummary: subdesired, ObservedSummary: subobserved}
		}
		items[i] = driftItemJSON{
			Address:             address,
			Name:                name,
			Description:         description,
			Provider:            provider,
			ProviderRevision:    item.ProviderRevision,
			EffectiveHash:       item.EffectiveHash,
			EffectiveHashStatus: effectiveHashStatus,
			Status:              status,
			ReasonCode:          reasonCode,
			PreflightStatus:     preflightStatus,
			PreflightReason:     preflightReason,
			DesiredSummary:      desired,
			ObservedSummary:     observed,
			Subresults:          subresults,
			SubresultsTruncated: item.SubresultsTruncated || subresultCount < len(item.Subresults),
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
		diagnostics := make([]executor.SafeSummary, diagnosticCount)
		truncated = truncated || diagnosticCount < len(item.Diagnostics)
		for j, diagnostic := range item.Diagnostics[:diagnosticCount] {
			diagnostics[j] = diagnostic.Clone()
		}
		address, addressTruncated := truncateComplianceText(item.Address)
		name, nameTruncated := truncateComplianceText(item.Name)
		provider, providerTruncated := truncateComplianceText(item.Provider)
		status, statusTruncated := truncateComplianceText(string(item.Status))
		reasonCode, reasonCodeTruncated := truncateComplianceText(string(item.ReasonCode))
		desired := item.DesiredSummary.Clone()
		observed := item.ObservedSummary.Clone()
		rebootRequired, rebootRequiredTruncated := truncateComplianceText(string(item.RebootRequired))
		rollbackClass, rollbackClassTruncated := truncateComplianceText(string(item.RollbackClass))
		rollbackStatus, rollbackStatusTruncated := truncateComplianceText(string(item.RollbackStatus))
		truncated = truncated || addressTruncated || nameTruncated || providerTruncated || statusTruncated || reasonCodeTruncated || rebootRequiredTruncated || rollbackClassTruncated || rollbackStatusTruncated
		apply[i] = applyItemJSON{
			Address:          address,
			Name:             name,
			Provider:         provider,
			ProviderRevision: item.ProviderRevision,
			EffectiveHash:    item.EffectiveHash,
			Status:           status,
			ReasonCode:       reasonCode,
			DesiredSummary:   desired,
			ObservedSummary:  observed,
			Activation:       activations,
			RebootRequired:   rebootRequired,
			RollbackClass:    rollbackClass,
			RollbackStatus:   rollbackStatus,
			Diagnostics:      diagnostics,
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
	if rebootRequired.Required || rebootRequired.Intent != nil || rebootRequired.Completion != nil {
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
		pendingReboot = &rebootRequiredJSON{
			Required: rebootRequired.Required, Sources: sources,
			AttemptGeneration: rebootRequired.AttemptGeneration,
		}
		if rebootRequired.Intent != nil {
			intent := rebootRequired.Intent
			generation, generationTruncated := truncateComplianceText(intent.Generation)
			phase, phaseTruncated := truncateComplianceText(string(intent.Phase))
			priorBootID, priorBootIDTruncated := truncateComplianceText(intent.PriorBootID)
			currentBootID, currentBootIDTruncated := truncateComplianceText(intent.CurrentBootID)
			reason, reasonTruncated := truncateComplianceText(intent.Reason)
			truncated = truncated || generationTruncated || phaseTruncated || priorBootIDTruncated || currentBootIDTruncated || reasonTruncated
			pendingReboot.Intent = &rebootIntentJSON{
				Generation: generation, Phase: phase, PriorBootID: priorBootID, CurrentBootID: currentBootID,
				PreparedAt: intent.PreparedAt, NotBefore: intent.NotBefore, Deadline: intent.Deadline,
				AttemptedAt: intent.AttemptedAt, AttemptDeadline: intent.AttemptDeadline,
				AttemptGeneration: intent.AttemptGeneration, Reason: reason,
			}
		}
		if rebootRequired.Completion != nil {
			completion := rebootRequired.Completion
			generation, generationTruncated := truncateComplianceText(completion.Generation)
			bootID, bootIDTruncated := truncateComplianceText(completion.BootID)
			truncated = truncated || generationTruncated || bootIDTruncated
			pendingReboot.Completion = &rebootCompletionJSON{
				Generation: generation, BootID: bootID, AttemptGeneration: completion.AttemptGeneration,
				CompletedAt: completion.CompletedAt,
			}
		}
	}
	schemaVersion := 7
	if activationBootstrap {
		schemaVersion = 10
	} else if hasCompleteCanonicalIdentities(drift, applied) {
		schemaVersion = 8
		if hasCompletePreflightEvidence(drift) {
			schemaVersion = 9
		}
	}
	payload := driftReportJSON{
		SchemaVersion:   schemaVersion,
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

func hasActivationBootstrapEvidence(drift engine.DriftReport, applied engine.ApplyResult) bool {
	if len(drift.Items) == 0 || !hasCompletePreflightEvidence(drift) {
		return false
	}
	missingHash := false
	for _, item := range drift.Items {
		if item.Provider == "" || item.ProviderRevision == "" {
			return false
		}
		if item.EffectiveHash == "" {
			if !item.EffectiveHashAuthorizationRequired {
				return false
			}
			missingHash = true
			continue
		}
		if item.EffectiveHashAuthorizationRequired {
			return false
		}
		if effectivehash.Validate(item.EffectiveHash) != nil {
			return false
		}
	}
	for _, item := range applied.Items {
		if item.Provider == "" || item.ProviderRevision == "" || effectivehash.Validate(item.EffectiveHash) != nil {
			return false
		}
	}
	return missingHash
}

func hasCompleteCanonicalIdentities(drift engine.DriftReport, applied engine.ApplyResult) bool {
	if len(drift.Items) == 0 && len(applied.Items) == 0 {
		return false
	}
	for _, item := range drift.Items {
		if item.EffectiveHash == "" || item.ProviderRevision == "" {
			return false
		}
	}
	for _, item := range applied.Items {
		if item.EffectiveHash == "" || item.ProviderRevision == "" {
			return false
		}
	}
	return true
}

func hasCompletePreflightEvidence(drift engine.DriftReport) bool {
	if len(drift.Items) == 0 {
		return false
	}
	for _, item := range drift.Items {
		switch item.PreflightStatus {
		case engine.PreflightNotRequired:
			if item.PreflightReason != "" {
				return false
			}
		case engine.PreflightReady, engine.PreflightBlocked:
			if item.PreflightReason == "" {
				return false
			}
		default:
			return false
		}
	}
	return true
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
