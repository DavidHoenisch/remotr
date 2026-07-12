package traceability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLintGodogFeaturesRequiresKnownActiveTags(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Scenarios: map[string]ManifestEntry{"OS-AEC-001": {Lifecycle: "verified"}, "OS-AEC-002": {Lifecycle: "deferred"}}}
	content := "@os_OS-AEC-001\nScenario: good\n  Given x\n\nScenario: missing\n  Given y\n\n@os_OS-AEC-002\nScenario: deferred\n  Given z\n"
	if err := os.WriteFile(filepath.Join(dir, "feature.feature"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	issues, err := LintGodogFeatures(dir, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %v", issues)
	}
}
