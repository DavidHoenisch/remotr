package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// OS-SRM-007: an outstanding reboot requirement is first-class state report
// telemetry even when the current compliance run has no Apply items.
func TestPendingReportsPersistedRebootRequirementWithoutCurrentApply(t *testing.T) {
	var p Pending
	p.SetRebootRequired(rebootstate.Status{Required: true, Sources: []rebootstate.Source{{
		Address: "base/packages/kernel", Name: "kernel", Provider: "apt",
	}}})
	p.SetFromPipeline(nil, engine.DriftReport{InCompliance: true}, engine.ApplyResult{}, nil, "digest")

	var payload struct {
		SchemaVersion  int `json:"schemaVersion"`
		RebootRequired struct {
			Required bool `json:"required"`
			Sources  []struct {
				Address  string `json:"address"`
				Provider string `json:"provider"`
			} `json:"sources"`
		} `json:"rebootRequired"`
		Apply []applyItemJSON `json:"apply"`
	}
	if err := json.Unmarshal(p.Drift.Report, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 7 || !payload.RebootRequired.Required || len(payload.RebootRequired.Sources) != 1 || payload.RebootRequired.Sources[0].Address != "base/packages/kernel" || payload.RebootRequired.Sources[0].Provider != "apt" {
		t.Fatalf("reboot-required telemetry = %+v", payload)
	}
	if len(payload.Apply) != 0 {
		t.Fatalf("current apply results = %+v, want none", payload.Apply)
	}
}

// OS-SRM-009: a coordinated attempt that returns with the same boot identity
// remains visible as operational state, including its stable timeout reason.
func TestPendingReportsSameBootTimeoutReasonWithoutCurrentApply(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	var p Pending
	p.SetRebootRequired(rebootstate.Status{
		Required: true,
		Sources:  []rebootstate.Source{{Address: "base/packages/kernel", Provider: "apt"}},
		Intent: &rebootstate.Intent{
			Generation: "kernel-6.12.1", Phase: rebootstate.PhaseTimedOut,
			PriorBootID: "boot-1", CurrentBootID: "boot-1",
			AttemptGeneration: 3, AttemptedAt: now.Add(-15 * time.Minute),
			AttemptDeadline: now, Reason: "reboot_timeout_same_boot_id",
		},
		AttemptGeneration: 3,
	})
	p.SetFromPipeline(nil, engine.DriftReport{InCompliance: true}, engine.ApplyResult{}, nil, "digest")

	var payload struct {
		SchemaVersion  int `json:"schemaVersion"`
		RebootRequired struct {
			AttemptGeneration uint64 `json:"attemptGeneration"`
			Intent            struct {
				Generation        string `json:"generation"`
				Phase             string `json:"phase"`
				PriorBootID       string `json:"priorBootId"`
				CurrentBootID     string `json:"currentBootId"`
				AttemptGeneration uint64 `json:"attemptGeneration"`
				Reason            string `json:"reason"`
			} `json:"intent"`
		} `json:"rebootRequired"`
	}
	if err := json.Unmarshal(p.Drift.Report, &payload); err != nil {
		t.Fatal(err)
	}
	intent := payload.RebootRequired.Intent
	if payload.SchemaVersion != 7 || payload.RebootRequired.AttemptGeneration != 3 || intent.Generation != "kernel-6.12.1" || intent.Phase != "timed-out" || intent.PriorBootID != "boot-1" || intent.CurrentBootID != "boot-1" || intent.AttemptGeneration != 3 || intent.Reason != "reboot_timeout_same_boot_id" {
		t.Fatalf("coordinated reboot telemetry = %+v", payload.RebootRequired)
	}
}

// OS-SRM-008: prepared reboot intent is an explicit authenticated Sync payload
// and is cleared only with the successfully sent request.
func TestPendingCarriesPreRebootAcknowledgementIntent(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	var p Pending
	p.SetRebootIntent(&rebootstate.Intent{
		Generation: "kernel-6.12.1", Phase: rebootstate.PhaseAwaitingAcknowledgement,
		PriorBootID: "boot-1", PreparedAt: now, NotBefore: now, Timeout: 15 * time.Minute,
	})
	req := p.Request("digest", "release", "dev")
	if req.RebootIntent == nil || req.RebootIntent.Generation != "kernel-6.12.1" || req.RebootIntent.Phase != "awaiting-acknowledgement" || req.RebootIntent.PriorBootID != "boot-1" {
		t.Fatalf("reboot intent payload = %+v", req.RebootIntent)
	}
	p.ClearSent(req)
	if p.RebootIntent != nil {
		t.Fatalf("sent reboot intent was not cleared: %+v", p.RebootIntent)
	}
}

func TestPendingCarriesArmedNetworkAcknowledgementIntent(t *testing.T) {
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)
	var p Pending
	p.SetNetworkIntent(&networkstate.Intent{
		ID: "network-1", Phase: networkstate.PhaseAwaitingAcknowledgement,
		Deadline: now.Add(2 * time.Minute), PlanHash: "sha256:" + strings.Repeat("a", 64), WatchdogArmed: true,
	})
	req := p.Request("digest", "release", "dev")
	if req.NetworkIntent == nil || req.NetworkIntent.ID != "network-1" || !req.NetworkIntent.WatchdogArmed || req.NetworkIntent.PlanHash == "" {
		t.Fatalf("network intent payload = %+v", req.NetworkIntent)
	}
	p.ClearSent(req)
	if p.NetworkIntent != nil {
		t.Fatalf("sent network intent was not cleared: %+v", p.NetworkIntent)
	}
}

func TestPending_SetFromPipeline_compliantAlwaysReportsDrift(t *testing.T) {
	var p Pending
	p.SetFromPipeline(
		map[string]string{"distro": "Arch"},
		engine.DriftReport{InCompliance: true},
		engine.ApplyResult{},
		nil,
		"digest123",
	)
	if p.Drift == nil {
		t.Fatal("expected drift payload for compliant check")
	}
	req := p.Request("last", "ref1", "dev")
	if req.Drift == nil || len(req.Drift.Report) == 0 {
		t.Fatalf("drift = %+v", req.Drift)
	}
}

func TestPending_SetFromPipeline_applyFailure(t *testing.T) {
	var p Pending
	p.SetFromPipeline(
		map[string]string{"distro": "Arch", "arch": "x86"},
		engine.DriftReport{
			Items: []engine.DriftItem{{
				Address:     "base-packages/true",
				Name:        "true",
				Description: "pacman package true",
			}},
		},
		engine.ApplyResult{},
		&engine.ApplyFailure{Address: "base-packages/true", Err: executor.NewSafeError("apply_failed", "provider_apply", errors.New("exit status 1"))},
		"digest123",
	)

	req := p.Request("last", "ref1", "dev")
	if req.ApplyFailure == nil || req.ApplyFailure.ResourceAddress != "base-packages/true" {
		t.Fatalf("applyFailure = %+v", req.ApplyFailure)
	}
	if req.Drift == nil || req.Drift.Digest != "digest123" {
		t.Fatalf("drift = %+v", req.Drift)
	}
	if req.Labels["distro"] != "Arch" {
		t.Fatalf("labels = %+v", req.Labels)
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(body) {
		t.Fatalf("invalid json: %s", body)
	}

	p.ClearSent(req)
	if p.ApplyFailure != nil || p.Drift != nil {
		t.Fatalf("expected cleared telemetry, apply=%v drift=%v", p.ApplyFailure, p.Drift)
	}
	if p.Labels["distro"] != "Arch" {
		t.Fatalf("labels should remain: %+v", p.Labels)
	}
}

func TestPending_SetFromPipeline_versionsStructuredCheckAndApplyTelemetry(t *testing.T) {
	const managedHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const unsupportedHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	var p Pending
	p.SetFromPipeline(
		map[string]string{"distro": "Arch"},
		engine.DriftReport{
			Items: []engine.DriftItem{
				{
					Address:          "base/managed-file",
					Name:             "managed-file",
					Description:      "managed configuration",
					Provider:         "files",
					ProviderRevision: "file-v1",
					EffectiveHash:    managedHash,
					Status:           executor.Drifted,
					ReasonCode:       executor.ReasonStateDrift,
					PreflightStatus:  engine.PreflightReady,
					PreflightReason:  executor.ReasonPreflightReady,
					DesiredSummary:   safeTestSummary("sha256:desired"),
					ObservedSummary:  safeTestSummary("sha256:observed"),
					Subresults: []engine.CheckSubresult{
						{Target: "alice", Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, ObservedSummary: safeTestSummary("owned user file matches")},
						{Target: "bob", Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, ObservedSummary: safeTestSummary("owned user file differs")},
					},
				},
				{
					Address:          "base/unsupported",
					Name:             "unsupported",
					Description:      "unavailable provider",
					Provider:         "nftables",
					ProviderRevision: "firewall-v1",
					EffectiveHash:    unsupportedHash,
					Status:           executor.Unsupported,
					ReasonCode:       executor.ReasonProviderUnavailable,
					PreflightStatus:  engine.PreflightBlocked,
					PreflightReason:  executor.ReasonProviderUnavailable,
				},
			},
		},
		engine.ApplyResult{Items: []engine.ApplyItem{{
			Address:          "base/managed-file",
			Name:             "managed-file",
			Provider:         "files",
			ProviderRevision: "file-v1",
			EffectiveHash:    managedHash,
			Status:           executor.Changed,
			Activation:       []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "example.service"}},
			RollbackClass:    executor.RollbackTransactional,
			Diagnostics:      []executor.SafeSummary{safeTestSummary("validated staged content")},
			DesiredSummary:   safeTestSummary("sha256:desired"),
			ObservedSummary:  safeTestSummary("sha256:observed"),
		}}},
		nil,
		"digest123",
	)

	req := p.Request("last", "ref1", "dev")
	if req.Drift == nil {
		t.Fatal("expected structured telemetry")
	}
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		Items         []struct {
			Status           string `json:"status"`
			ReasonCode       string `json:"reasonCode"`
			PreflightStatus  string `json:"preflightStatus"`
			PreflightReason  string `json:"preflightReason"`
			Provider         string `json:"provider"`
			ProviderRevision string `json:"providerRevision"`
			EffectiveHash    string `json:"effectiveHash"`
			Subresults       []struct {
				Target string `json:"target"`
				Status string `json:"status"`
			} `json:"subresults"`
		} `json:"items"`
		Apply []struct {
			Status           string `json:"status"`
			RollbackClass    string `json:"rollbackClass"`
			ProviderRevision string `json:"providerRevision"`
			EffectiveHash    string `json:"effectiveHash"`
			Activation       []struct {
				Kind   string `json:"kind"`
				Target string `json:"target"`
			} `json:"activation"`
			Diagnostics []executor.SafeSummary `json:"diagnostics"`
		} `json:"apply"`
	}
	if err := json.Unmarshal(req.Drift.Report, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 9 {
		t.Fatalf("schemaVersion = %d, want 9", payload.SchemaVersion)
	}
	if len(payload.Items) != 2 || payload.Items[0].Status != "drifted" || payload.Items[0].ReasonCode != "state_drift" || payload.Items[0].PreflightStatus != "ready" || payload.Items[0].PreflightReason != "preflight_ready" || payload.Items[0].Provider != "files" || payload.Items[0].ProviderRevision != "file-v1" || payload.Items[0].EffectiveHash != managedHash {
		t.Fatalf("items = %+v", payload.Items)
	}
	if payload.Items[1].Status != "unsupported" || payload.Items[1].ReasonCode != "provider_unavailable" {
		t.Fatalf("unsupported item = %+v", payload.Items[1])
	}
	if len(payload.Items[0].Subresults) != 2 || payload.Items[0].Subresults[0].Target != "alice" || payload.Items[0].Subresults[1].Status != "drifted" {
		t.Fatalf("subresults = %+v", payload.Items[0].Subresults)
	}
	if len(payload.Apply) != 1 || payload.Apply[0].Status != "changed" || payload.Apply[0].RollbackClass != "transactional" || payload.Apply[0].ProviderRevision != "file-v1" || payload.Apply[0].EffectiveHash != managedHash {
		t.Fatalf("apply = %+v", payload.Apply)
	}
	if len(payload.Apply[0].Activation) != 1 || payload.Apply[0].Activation[0].Kind != "restart" || payload.Apply[0].Activation[0].Target != "example.service" {
		t.Fatalf("activation = %+v", payload.Apply[0].Activation)
	}
	if len(payload.Apply[0].Diagnostics) != 1 || !strings.Contains(payload.Apply[0].Diagnostics[0].String(), "validated staged content") {
		t.Fatalf("diagnostics = %+v", payload.Apply[0].Diagnostics)
	}
	if string(req.Drift.Report) == "" || string(req.Drift.Report) == "secret-value" {
		t.Fatalf("report = %s", req.Drift.Report)
	}
}

// OS-ESM-009: runtime failures remain a separate collection and do not alter
// the configuration compliance bit or item status.
func TestPending_SetFromPipelineSeparatesScheduleRuntimeTelemetry(t *testing.T) {
	exitCode := 17
	var p Pending
	p.SetFromPipeline(nil, engine.DriftReport{
		InCompliance: true,
		Items: []engine.DriftItem{{
			Address: "base/nightly", Name: "nightly", Provider: "endpoint-schedule/systemd-timer",
			Status: executor.Compliant, ReasonCode: executor.ReasonCompliant,
		}},
		ScheduleRuntime: []engine.ScheduleRuntimeItem{{
			Address: "base/nightly", Name: "nightly", Provider: "endpoint-schedule/systemd-timer",
			Status: executor.ScheduleRunFailed, ExitCode: &exitCode, MissedRunBehavior: executor.ScheduleMissedRunCatchUp,
		}},
	}, engine.ApplyResult{}, nil, "digest")

	var payload struct {
		SchemaVersion   int             `json:"schemaVersion"`
		InCompliance    bool            `json:"inCompliance"`
		Items           []driftItemJSON `json:"items"`
		ScheduleRuntime []struct {
			Address           string `json:"address"`
			Status            string `json:"status"`
			ExitCode          *int   `json:"exitCode"`
			MissedRunBehavior string `json:"missedRunBehavior"`
		} `json:"scheduleRuntime"`
	}
	if err := json.Unmarshal(p.Drift.Report, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 7 || !payload.InCompliance || len(payload.Items) != 1 || payload.Items[0].Status != "compliant" {
		t.Fatalf("configuration payload = %+v", payload)
	}
	if len(payload.ScheduleRuntime) != 1 || payload.ScheduleRuntime[0].Address != "base/nightly" || payload.ScheduleRuntime[0].Status != "failed" || payload.ScheduleRuntime[0].ExitCode == nil || *payload.ScheduleRuntime[0].ExitCode != exitCode || payload.ScheduleRuntime[0].MissedRunBehavior != "catch-up" {
		t.Fatalf("schedule runtime payload = %+v", payload.ScheduleRuntime)
	}
}

func TestPending_SetFromPipeline_boundsExpandedTelemetryWithoutChangingUnchangedDetection(t *testing.T) {
	items := make([]engine.DriftItem, 0, 512)
	for i := 0; i < 512; i++ {
		items = append(items, engine.DriftItem{
			Address:         fmt.Sprintf("base/resource-%03d", i),
			Name:            fmt.Sprintf("resource-%03d", i),
			Description:     strings.Repeat("description ", 200),
			Provider:        "provider",
			Status:          executor.Drifted,
			ReasonCode:      executor.ReasonStateDrift,
			DesiredSummary:  safeTestSummary(strings.Repeat("desired ", 200)),
			ObservedSummary: safeTestSummary(strings.Repeat("observed ", 200)),
		})
	}

	var p Pending
	p.SetFromPipeline(nil, engine.DriftReport{Items: items}, engine.ApplyResult{}, nil, "digest")
	if p.Drift == nil {
		t.Fatal("expected telemetry")
	}
	if len(p.Drift.Report) > MaxComplianceReportBytes {
		t.Fatalf("report size = %d, limit = %d", len(p.Drift.Report), MaxComplianceReportBytes)
	}
	var payload struct {
		Truncated bool `json:"truncated"`
		Items     []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(p.Drift.Report, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Truncated || len(payload.Items) == 0 || payload.Items[0].Status != "drifted" {
		t.Fatalf("bounded payload = %+v", payload)
	}
	if !Unchanged("digest", "digest", "release", "release") {
		t.Fatal("expanded telemetry must not change digest/release unchanged detection")
	}
}

func TestPending_SetSystemInfo_clearSent(t *testing.T) {
	var p Pending
	raw := json.RawMessage(`{"cpu":{"modelName":"Test CPU"}}`)
	p.SetSystemInfo("digest1", raw)
	req := p.Request("last", "ref1", "dev")
	if req.SystemInfo == nil || req.SystemInfo.Digest != "digest1" {
		t.Fatalf("systemInfo = %+v", req.SystemInfo)
	}
	p.ClearSent(req)
	if p.SystemInfo != nil {
		t.Fatal("expected system info cleared after sync")
	}
}

func TestPending_unchangedSyncStillSendsFailure(t *testing.T) {
	p := Pending{
		ApplyFailure: &ApplyFailurePayload{
			ResourceAddress: "cfg/pkg",
			Failure:         executor.NewSafeError("apply_failed", "provider_apply", errors.New("install failed")),
		},
	}
	req := p.Request("same-digest", "ref1", "v0.1.12")
	if req.ApplyFailure == nil {
		t.Fatal("expected apply failure in request for unchanged artifact sync")
	}
}

func safeTestSummary(text string) executor.SafeSummary {
	summary, err := executor.NewSafeSummary([]executor.SafeField{{
		Path: "test", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: text,
	}})
	if err != nil {
		panic(err)
	}
	return summary
}
