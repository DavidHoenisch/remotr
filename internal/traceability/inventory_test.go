package traceability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryDiscoversActiveAndArchivedScenarios(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "changes/current/specs/capability/spec.md", "### Requirement: Current requirement\n\n<!-- verification-id: OS-AEC-001 -->\n#### Scenario: Current scenario\n")
	writeSpec(t, root, "changes/archive/retired/specs/old-capability/spec.md", "### Requirement: Archived requirement\n\n#### Scenario: Archived scenario\n<!-- verification-id: OS-AEC-002 -->\n")

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
