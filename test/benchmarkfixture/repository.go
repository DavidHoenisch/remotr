package benchmarkfixture

import (
	"os"
	"path/filepath"
	"testing"
)

// WriteCompositionRepository creates a deterministic configuration repository
// with one fleet and one endpoint extension over the requested package fixture.
func WriteCompositionRepository(t testing.TB, resourceCount ResourceCount) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "modules", "base.yaml"), "kind: module\n"+string(Artifact(resourceCount)))
	writeFile(t, filepath.Join(root, "fleets", "benchmark", "manifest.yaml"), `kind: manifest
modules:
  - modules/base.yaml
`)
	writeFile(t, filepath.Join(root, "endpoints", "benchmark-endpoint", "manifest.yaml"), `kind: manifest
extends: fleets/benchmark/manifest.yaml
modules: []
`)
	return root
}

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
