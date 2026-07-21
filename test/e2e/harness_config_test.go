//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestE2EHarnessOperatorConfigIsIsolatedFromDefaultProductionConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("REMOTR_CONFIG", "")
	t.Setenv("REMOTR_OPERATOR_STATE_DIR", "")

	productionConfig := filepath.Join(home, ".config", "remotr", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(productionConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	wantProduction := []byte("server_url: https://prod.remotr.example\nstate_dir: /var/lib/remotr/operator\n")
	if err := os.WriteFile(productionConfig, wantProduction, 0o600); err != nil {
		t.Fatal(err)
	}

	operatorState := filepath.Join(t.TempDir(), "operator")
	command := remotrCommand(
		t,
		"https://e2e.remotr.invalid",
		filepath.Join(t.TempDir(), "ca.crt"),
		operatorState,
		"config", "init",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize isolated E2E operator config: %v\n%s", err, output)
	}

	gotProduction, err := os.ReadFile(productionConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotProduction, wantProduction) {
		t.Fatalf("default production config was overwritten:\ngot:\n%s\nwant:\n%s", gotProduction, wantProduction)
	}
	if _, err := os.Stat(filepath.Join(operatorState, "config.yaml")); err != nil {
		t.Fatalf("isolated E2E config was not written: %v", err)
	}
}
