package osquery_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/queryexec"
	"github.com/DavidHoenisch/remotr/internal/applicators/osquery"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-OSQ-003, OS-OSQ-005, OS-OSQ-007: end-to-end provider contract + real
// subprocess execution via queryexec against both a private 0700 managed
// installation and an Elastic Agent-style existing osqueryd binary.
func TestOsqueryProviderContractAndRealSubprocessExecution(t *testing.T) {
	t.Run("managed private 0700 install executes via queryexec", func(t *testing.T) {
		root := t.TempDir()
		installDir := filepath.Join(root, "private-osquery")
		stateFile := filepath.Join(root, "state", "osquery-runtime.json")
		script := []byte("#!/bin/sh\nif [ \"$1\" = \"--json\" ] && [ \"$2\" = \"--ephemeral\" ] && [ \"$3\" = \"--disable_database\" ] && [ \"$4\" = \"--disable_events\" ]; then\n  printf '[{\"host\":\"node1\",\"port\":\"443\"}]'\n  exit 0\nfi\nexit 2\n")
		sum := sha256.Sum256(script)

		app := osquery.New(models.OsqueryResource{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
			Name:         "priv-contract",
			Mode:         models.OsqueryModeManaged,
			Visibility:   models.OsqueryVisibilityPrivate,
			InstallDir:   installDir,
			Version:      "5.14.1",
			SourceURL:    "https://downloads.example.test/osqueryi",
			SHA256:       hex.EncodeToString(sum[:]),
		})
		app.RuntimeStatePath = stateFile
		app.Fetch = func(context.Context, string) ([]byte, error) { return script, nil }

		prov, err := contract.New(app)
		if err != nil {
			t.Fatal(err)
		}
		if chk := prov.Check(context.Background()); chk.Status != contract.Drifted {
			t.Fatalf("Check = %+v, want drifted", chk)
		}
		if ar := prov.Apply(context.Background()); ar.Status != contract.Changed || ar.Err != nil {
			t.Fatalf("Apply = %+v, want changed", ar)
		}
		if chk := prov.Check(context.Background()); chk.Status != contract.Compliant {
			t.Fatalf("second Check = %+v, want compliant", chk)
		}

		// Execute a real read-only query using the persisted private runtime state.
		exec := &queryexec.Executor{RuntimeStatePath: stateFile}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID: "real-run-1",
			Name:  "ports",
			SQL:   "SELECT host, port FROM listening_ports",
		})
		if res.Status != queryexec.StatusSuccess || len(res.Rows) != 1 || res.Rows[0]["port"] != "443" {
			t.Fatalf("real private subprocess result = %+v", res)
		}
	})

	t.Run("existing Elastic Agent osqueryd executes with -S and stays unmodified", func(t *testing.T) {
		root := t.TempDir()
		stateFile := filepath.Join(root, "state", "osquery-runtime.json")
		elasticBin := filepath.Join(root, "Elastic", "Agent", "data", "elastic-agent-8.16.0-xyz", "components", "osqueryd")
		if err := os.MkdirAll(filepath.Dir(elasticBin), 0o755); err != nil {
			t.Fatal(err)
		}
		script := []byte("#!/bin/sh\nif [ \"$1\" = \"-S\" ] && [ \"$2\" = \"--json\" ] && [ \"$3\" = \"--ephemeral\" ] && [ \"$4\" = \"--disable_database\" ] && [ \"$5\" = \"--disable_events\" ]; then\n  printf '[{\"agent\":\"elastic\"}]'\n  exit 0\nfi\nexit 3\n")
		if err := os.WriteFile(elasticBin, script, 0o755); err != nil {
			t.Fatal(err)
		}

		app := osquery.New(models.OsqueryResource{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
			Name:         "elastic-contract",
			Mode:         models.OsqueryModeExisting,
			AllowedOwner: currentUsername(t),
			BinaryPaths:  []string{filepath.Join(root, "Elastic", "Agent", "data", "elastic-agent-*", "components", "osqueryd")},
		})
		app.RuntimeStatePath = stateFile

		prov, err := contract.New(app)
		if err != nil {
			t.Fatal(err)
		}
		if ar := prov.Apply(context.Background()); ar.Status != contract.Changed || ar.Err != nil {
			t.Fatalf("Apply = %+v, want changed", ar)
		}
		if chk := prov.Check(context.Background()); chk.Status != contract.Compliant {
			t.Fatalf("second Check = %+v, want compliant", chk)
		}

		exec := &queryexec.Executor{RuntimeStatePath: stateFile}
		res := exec.Execute(context.Background(), queryexec.Request{
			RunID: "real-run-2",
			Name:  "elastic-check",
			SQL:   "SELECT 'elastic' AS agent",
		})
		if res.Status != queryexec.StatusSuccess || len(res.Rows) != 1 || res.Rows[0]["agent"] != "elastic" {
			t.Fatalf("real elastic osqueryd -S result = %+v", res)
		}
	})
}
