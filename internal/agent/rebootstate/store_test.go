package rebootstate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/rebootstate"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

// OS-SRM-007: reboot-required survives a later compliant cycle and an agent
// restart; observing the requirement never executes a reboot.
func TestStoreRetainsRebootRequirementAcrossCompliantRuns(t *testing.T) {
	dir := t.TempDir()
	store := rebootstate.New(dir)
	first, err := store.Record(engine.ApplyResult{Items: []engine.ApplyItem{{
		Address:        "base/packages/kernel",
		Name:           "kernel",
		Provider:       "apt",
		Status:         executor.Changed,
		RebootRequired: executor.RebootRequired,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Required || len(first.Sources) != 1 || first.Sources[0].Address != "base/packages/kernel" {
		t.Fatalf("first reboot requirement = %+v", first)
	}

	restarted := rebootstate.New(dir)
	later, err := restarted.Record(engine.ApplyResult{})
	if err != nil {
		t.Fatal(err)
	}
	if !later.Required || len(later.Sources) != 1 || later.Sources[0].Provider != "apt" {
		t.Fatalf("persisted reboot requirement = %+v", later)
	}
	if got := restarted.Path(); got != filepath.Join(dir, "reboot-state.json") {
		t.Fatalf("state path = %q", got)
	}
	info, err := os.Stat(restarted.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".reboot-state-") {
			t.Fatalf("temporary state file was not cleaned up: %s", entry.Name())
		}
	}
}

func TestStoreRejectsCorruptPersistentState(t *testing.T) {
	store := rebootstate.New(t.TempDir())
	if err := os.WriteFile(store.Path(), []byte(`{"schemaVersion":1,"required":true,"sources":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(engine.ApplyResult{}); err == nil || !strings.Contains(err.Error(), "required flag and sources disagree") {
		t.Fatalf("Record() error = %v", err)
	}
}
