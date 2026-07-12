package sync

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

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
	if payload.SchemaVersion != 2 {
		t.Fatalf("schemaVersion = %d, want 2", payload.SchemaVersion)
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
