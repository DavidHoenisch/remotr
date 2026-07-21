package traceability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryDiscoversActiveAndArchivedScenarios(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "changes/current/specs/capability/spec.md", "### Requirement: Current requirement\n\n<!-- verification-id: OS-AEC-001 -->\n#### Scenario: Current scenario\n")
	writeSpec(t, root, "changes/archive/2026-07-21-retired/specs/old-capability/spec.md", "### Requirement: Archived requirement\n\n#### Scenario: Archived scenario\n<!-- verification-id: OS-AEC-002 -->\n")

	scenarios, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("scenario count = %d, want 2", len(scenarios))
	}
	if got := scenarios[0]; got.Change != "retired" || got.Capability != "old-capability" || got.Requirement != "Archived requirement" || got.Title != "Archived scenario" || got.VerificationID != "OS-AEC-002" {
		t.Fatalf("archived scenario = %#v", got)
	}
	if got := scenarios[1]; got.Change != "current" || got.Capability != "capability" || got.Requirement != "Current requirement" || got.Title != "Current scenario" || got.VerificationID != "OS-AEC-001" {
		t.Fatalf("active scenario = %#v", got)
	}
}

func TestInventoryOnlyStripsValidArchiveDatePrefixes(t *testing.T) {
	for _, test := range []struct {
		name      string
		directory string
		want      string
	}{
		{name: "undated archive", directory: "retired", want: "retired"},
		{name: "invalid archive date", directory: "2026-02-30-retired", want: "2026-02-30-retired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeSpec(t, root, filepath.Join("changes", "archive", test.directory, "specs", "capability", "spec.md"), "### Requirement: Archived requirement\n\n#### Scenario: Archived scenario\n<!-- verification-id: OS-AEC-001 -->\n")

			scenarios, err := Inventory(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(scenarios) != 1 {
				t.Fatalf("scenario count = %d, want 1", len(scenarios))
			}
			if got := scenarios[0].Change; got != test.want {
				t.Fatalf("change = %q, want %q", got, test.want)
			}
		})
	}
}

func FuzzInventoryCanonicalizesDatedArchiveChange(f *testing.F) {
	for _, change := range []string{"a", "retired", "2026-07-21-retired"} {
		f.Add(change)
	}
	f.Fuzz(func(t *testing.T, change string) {
		if change == "" || len(change) > 64 || change == "." || change == ".." || strings.ContainsAny(change, `/\\`) || strings.ContainsRune(change, 0) {
			t.Skip()
		}
		root := t.TempDir()
		writeSpec(t, root, filepath.Join("changes", "archive", "2026-07-21-"+change, "specs", "capability", "spec.md"), "### Requirement: Archived requirement\n\n#### Scenario: Archived scenario\n<!-- verification-id: OS-AEC-001 -->\n")

		scenarios, err := Inventory(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(scenarios) != 1 {
			t.Fatalf("scenario count = %d, want 1", len(scenarios))
		}
		if got := scenarios[0].Change; got != change {
			t.Fatalf("change = %q, want %q", got, change)
		}
	})
}

func TestInventoryRecordsDeltaOperationForScenarioLineage(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "changes/child/specs/capability/spec.md", "## MODIFIED Requirements\n\n### Requirement: Existing requirement\n\n#### Scenario: Refined scenario\n<!-- verification-id: OS-AEC-001 -->\n\n## ADDED Requirements\n\n### Requirement: New requirement\n\n<!-- verification-id: OS-AEC-080 -->\n#### Scenario: New scenario\n")

	scenarios, err := Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("scenario count = %d, want 2", len(scenarios))
	}
	if got := scenarios[0].Operation; got != "modified" {
		t.Fatalf("modified scenario operation = %q, want modified", got)
	}
	if got := scenarios[1].Operation; got != "added" {
		t.Fatalf("added scenario operation = %q, want added", got)
	}
}

func TestApplicatorUmbrellaManifestClassifiesEveryScenarioTruthfully(t *testing.T) {
	inventory, err := Inventory(filepath.Join("..", "..", "openspec"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(filepath.Join("..", "..", "test", "traceability.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	const change = "expand-linux-system-administration-applicators"
	count := 0
	for _, scenario := range inventory {
		if scenario.Change != change {
			continue
		}
		count++
		entry, ok := manifest.Scenarios[scenario.VerificationID]
		if !ok {
			t.Errorf("%s is missing from the traceability manifest", scenario.VerificationID)
			continue
		}
		if entry.Source.Change != change || entry.Source.Capability != scenario.Capability {
			t.Errorf("%s source = %s/%s, want %s/%s", scenario.VerificationID, entry.Source.Change, entry.Source.Capability, change, scenario.Capability)
		}
		if entry.Lifecycle != "planned" && entry.Lifecycle != "verified" && entry.Lifecycle != "deferred" {
			t.Errorf("%s lifecycle = %q", scenario.VerificationID, entry.Lifecycle)
		}
		if entry.Lifecycle != "verified" && strings.Contains(entry.DispositionReason, "blocked on this foundation change") {
			t.Errorf("%s has stale foundation-blocker disposition", scenario.VerificationID)
		}
	}
	if count != 232 {
		t.Fatalf("umbrella scenario count = %d, want 232", count)
	}
}

func writeSpec(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
