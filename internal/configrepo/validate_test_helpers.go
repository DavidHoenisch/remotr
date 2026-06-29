package configrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFleetModule(t *testing.T, dir, fleet, moduleYAML string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "modules", fleet+"-module.yaml"), "kind: module\n"+moduleYAML)
	writeFile(t, filepath.Join(dir, "fleets", fleet, "manifest.yaml"), "kind: manifest\nmodules:\n  - modules/"+fleet+"-module.yaml\n")
}
