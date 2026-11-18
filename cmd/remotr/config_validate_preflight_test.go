package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// OS-AEC-120/121: config validate warns, without failing, for a resource whose
// effective risk needs a preflight its provider cannot run, and stays quiet for
// normal-risk resources and preflight-capable providers.
func TestAppConfigValidateWarnsForUnsatisfiablePreflight(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "modules", "provisioning.yaml"), `kind: module
schemaVersion: 1
configurations:
  - name: provisioning
    targetDistros: [Ubuntu]
    targetArch: [x86]
    resources:
      - kind: bootstrap
        name: default-risk
        when:
          pathMissing: /opt/example
        steps:
          - exec: [/usr/bin/true]
      - kind: bootstrap
        name: routine
        risk: normal
        when:
          pathMissing: /opt/example
        steps:
          - exec: [/usr/bin/true]
      - kind: ubuntuPro
        name: primary
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/fleet@active
`)
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "servers", "manifest.yaml"), "kind: manifest\nmodules:\n  - modules/provisioning.yaml\n")

	var runErr error
	output := captureStdout(t, func() {
		runErr = newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir})
	})
	if runErr != nil {
		t.Fatalf("config validate must not fail on preflight warnings: %v\n%s", runErr, output)
	}
	want := "WARN modules/provisioning.yaml [unsatisfiable_preflight]: provisioning/default-risk"
	if !strings.Contains(output, want) || !strings.Contains(output, "risk boot") || !strings.Contains(output, "risk: normal") {
		t.Fatalf("missing preflight warning %q:\n%s", want, output)
	}
	for _, quiet := range []string{"provisioning/routine", "provisioning/primary"} {
		if strings.Contains(output, "[unsatisfiable_preflight]: "+quiet) {
			t.Fatalf("unexpected preflight warning for %s:\n%s", quiet, output)
		}
	}
}
