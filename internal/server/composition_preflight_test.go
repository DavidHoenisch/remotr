package server

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/providermatrix"
)

// OS-AEC-120: Git sync composition logs the same unsatisfiable-preflight
// warning as config validate, naming the Fleet and resource, without failing.
func TestCompositionLogsUnsatisfiablePreflightWarnings(t *testing.T) {
	repo := t.TempDir()
	for path, content := range map[string]string{
		"modules/provisioning.yaml": `kind: module
schemaVersion: 1
configurations:
  - name: provisioning
    resources:
      - kind: bootstrap
        name: default-risk
        when:
          pathMissing: /opt/example
        steps:
          - exec: [/usr/bin/true]
`,
		"fleets/servers/manifest.yaml": "kind: manifest\nmodules:\n  - modules/provisioning.yaml\n",
	} {
		full := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	matrix, err := providermatrix.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := (&CompositionService{RepoRoot: repo, Store: &capturingVariantStore{}, ProviderMatrix: &matrix}).ComposeAll(t.Context(), "rel-1"); err != nil {
		t.Fatalf("ComposeAll() = %v", err)
	}
	for _, want := range []string{"unsatisfiable preflight", "fleet=servers", "provisioning/default-risk"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("composition log missing %q:\n%s", want, logs.String())
		}
	}
}
