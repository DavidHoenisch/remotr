package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

// OS-AEC-117: exact PopOS targeting through the public configuration CLI.
// Author-time acceptance must match already-qualified PopOS APT support.
func TestConfigValidateAPTPackageTargets(t *testing.T) {
	for _, tc := range []struct {
		distro string
		valid  bool
	}{{"PopOS", true}, {"Debian", true}, {"Ubuntu", true}, {"Arch", false}, {"unknown", false}} {
		t.Run(tc.distro, func(t *testing.T) {
			dir := t.TempDir()
			writeConfigTestFile(t, filepath.Join(dir, "modules", "clamav.yaml"), fmt.Sprintf(`kind: module
schemaVersion: 1
configurations:
  - name: antivirus
    targetDistros: [%s]
    targetArch: [x86]
    resources:
      - kind: package
        name: clamav
        lifecycle: present
        packageManager: apt
`, tc.distro))
			writeConfigTestFile(t, filepath.Join(dir, "fleets", "test", "manifest.yaml"), "kind: manifest\nmodules: [modules/clamav.yaml]\n")
			err := newApp().Run(context.Background(), []string{"remotr", "--config", filepath.Join(t.TempDir(), "operator.yaml"), "config", "validate", dir})
			if (err == nil) != tc.valid {
				t.Fatalf("APT config validation for %s: %v; want accepted=%v", tc.distro, err, tc.valid)
			}
		})
	}
}
