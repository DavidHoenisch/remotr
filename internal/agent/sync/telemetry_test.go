package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
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
	if payload.SchemaVersion != 4 || !payload.RebootRequired.Required || len(payload.RebootRequired.Sources) != 1 || payload.RebootRequired.Sources[0].Address != "base/packages/kernel" || payload.RebootRequired.Sources[0].Provider != "apt" {
		t.Fatalf("reboot-required telemetry = %+v", payload)
	}
	if len(payload.Apply) != 0 {
		t.Fatalf("current apply results = %+v, want none", payload.Apply)
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
		&engine.ApplyFailure{Address: "base-packages/true", Err: errors.New("exit status 1")},
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
	var p Pending
	p.SetFromPipeline(
		map[string]string{"distro": "Arch"},
		engine.DriftReport{
			Items: []engine.DriftItem{
				{
					Address:         "base/managed-file",
					Name:            "managed-file",
					Description:     "managed configuration",
					Provider:        "files",
					Status:          executor.Drifted,
					ReasonCode:      executor.ReasonStateDrift,
					DesiredSummary:  "sha256:desired",
					ObservedSummary: "sha256:observed",
				},
				{
					Address:     "base/unsupported",
					Name:        "unsupported",
					Description: "unavailable provider",
					Provider:    "nftables",
					Status:      executor.Unsupported,
					ReasonCode:  executor.ReasonProviderUnavailable,
				},
			},
		},
		engine.ApplyResult{Items: []engine.ApplyItem{{
			Address:         "base/managed-file",
			Name:            "managed-file",
			Provider:        "files",
			Status:          executor.Changed,
			Activation:      []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "example.service"}},
			RollbackClass:   executor.RollbackTransactional,
			Diagnostics:     []executor.RedactedSummary{"validated staged content"},
			DesiredSummary:  "sha256:desired",
			ObservedSummary: "sha256:observed",
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
			Status     string `json:"status"`
			ReasonCode string `json:"reasonCode"`
			Provider   string `json:"provider"`
		} `json:"items"`
		Apply []struct {
			Status        string `json:"status"`
			RollbackClass string `json:"rollbackClass"`
			Activation    []struct {
				Kind   string `json:"kind"`
				Target string `json:"target"`
			} `json:"activation"`
			Diagnostics []string `json:"diagnostics"`
		} `json:"apply"`
	}
	if err := json.Unmarshal(req.Drift.Report, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SchemaVersion != 4 {
		t.Fatalf("schemaVersion = %d, want 4", payload.SchemaVersion)
	}
	if len(payload.Items) != 2 || payload.Items[0].Status != "drifted" || payload.Items[0].ReasonCode != "state_drift" || payload.Items[0].Provider != "files" {
		t.Fatalf("items = %+v", payload.Items)
	}
	if payload.Items[1].Status != "unsupported" || payload.Items[1].ReasonCode != "provider_unavailable" {
		t.Fatalf("unsupported item = %+v", payload.Items[1])
	}
	if len(payload.Apply) != 1 || payload.Apply[0].Status != "changed" || payload.Apply[0].RollbackClass != "transactional" {
		t.Fatalf("apply = %+v", payload.Apply)
	}
	if len(payload.Apply[0].Activation) != 1 || payload.Apply[0].Activation[0].Kind != "restart" || payload.Apply[0].Activation[0].Target != "example.service" {
		t.Fatalf("activation = %+v", payload.Apply[0].Activation)
	}
	if len(payload.Apply[0].Diagnostics) != 1 || payload.Apply[0].Diagnostics[0] != "validated staged content" {
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
	if payload.SchemaVersion != 4 || !payload.InCompliance || len(payload.Items) != 1 || payload.Items[0].Status != "compliant" {
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
			DesiredSummary:  executor.RedactedSummary(strings.Repeat("desired ", 200)),
			ObservedSummary: executor.RedactedSummary(strings.Repeat("observed ", 200)),
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
			Message:         "install failed",
		},
	}
	req := p.Request("same-digest", "ref1", "v0.1.12")
	if req.ApplyFailure == nil {
		t.Fatal("expected apply failure in request for unchanged artifact sync")
	}
}
