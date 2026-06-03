package sync

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
)

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
