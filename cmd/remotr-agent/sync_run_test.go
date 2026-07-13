package main

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// OS-SRM-007: the composed agent carries durable reboot-required state into a
// later compliant Sync report without coupling it to reboot execution.
func TestSyncRunStateCarriesPersistedRebootRequirementIntoLaterReport(t *testing.T) {
	dir := t.TempDir()
	state := newSyncRunState(dir, "https://remotr.example", nil, nil)
	var pending sync.Pending
	if err := state.recordRebootRequirement(&pending, engine.ApplyResult{Items: []engine.ApplyItem{{
		Address: "base/packages/kernel", Name: "kernel", Provider: "apt",
		Status: executor.Changed, RebootRequired: executor.RebootRequired,
	}}}); err != nil {
		t.Fatal(err)
	}

	restarted := newSyncRunState(dir, "https://remotr.example", nil, nil)
	var afterRestart sync.Pending
	if err := restarted.recordRebootRequirement(&afterRestart, engine.ApplyResult{}); err != nil {
		t.Fatal(err)
	}
	afterRestart.SetFromPipeline(nil, engine.DriftReport{InCompliance: true}, engine.ApplyResult{}, nil, "digest")
	if !afterRestart.RebootRequired.Required || len(afterRestart.RebootRequired.Sources) != 1 || afterRestart.RebootRequired.Sources[0].Address != "base/packages/kernel" {
		t.Fatalf("pending reboot requirement = %+v", afterRestart.RebootRequired)
	}
}
