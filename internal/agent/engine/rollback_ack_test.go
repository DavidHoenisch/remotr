package engine_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/agent/resolve"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-AEC-122: after a file change applies, the engine's next Check proves it
// compliant and acknowledges the armed rollback record, so the resource is not
// reported as rollback_reservation_failed and is free for later changes.
func TestEngineAcknowledgesConvergedFileRollbackOnCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	if err := os.WriteFile(path, []byte("PASS_MAX_DAYS 99999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "state")
	state, err := models.ParseState(bytes.NewBufferString(`schemaVersion: 1
configurations:
  - name: password-policy
    resources:
      - kind: file
        name: pass-max-days
        path: ` + path + `
        content: "PASS_MAX_DAYS 90\n"
`))
	if err != nil {
		t.Fatal(err)
	}
	endpointFacts := facts.Facts{Distro: types.Debian, Arch: types.X86}
	eng, err := engine.New(resolve.Resolve(state, endpointFacts), endpointFacts, nil, nil,
		engine.WithArtifactDigest("sha256:artifact"), engine.WithStateDir(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	if result := eng.ApplyAll(t.Context(), engine.PolicyAuto); result.Failed != nil {
		t.Fatalf("ApplyAll() failed: %+v", result.Failed)
	}
	report := eng.CheckAll(t.Context())
	if len(report.Items) != 1 {
		t.Fatalf("report items = %+v", report.Items)
	}
	item := report.Items[0]
	if item.Status != executor.Compliant || item.PreflightStatus != engine.PreflightReady {
		t.Fatalf("file after converged Check = %+v, want compliant and preflight ready", item)
	}
	store, err := rollbackstore.New(rollbackstore.Options{Root: filepath.Join(stateDir, "resource-transactions")})
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.Records(t.Context(), "password-policy/pass-max-days")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Armed {
			t.Fatalf("rollback record still armed after compliant Check: %+v", record)
		}
	}
}
