package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSecretUploadReadsProtectedInputFileAndNeverAcceptsArgvMaterial(t *testing.T) {
	dir := t.TempDir()
	fixturesDir := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(fixturesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := map[string]any{"status": 201, "body": map[string]any{
		"name": "repositories/private", "version": "1", "fingerprint": "sha256:safe", "fleet": "production",
		"active": false, "createdAt": time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), "createdBy": "operator-1", "revoked": false, "resolutionBlocked": false,
	}}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixturesDir, "POST_v1_admin_secrets_versions.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(dir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operator.crt", "operator.key", "ca.crt", "state.json"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	secretPath := filepath.Join(dir, "repository-token")
	if err := os.WriteFile(secretPath, []byte("cli-secret-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMOTR_DEMO", "1")
	t.Setenv("REMOTR_DEMO_FIXTURES", fixturesDir)
	t.Setenv("REMOTR_SERVER_URL", "https://demo.remotr.example")
	t.Setenv("REMOTR_OPERATOR_STATE_DIR", stateDir)

	stdout := captureStdout(t, func() {
		err := newApp().Run(context.Background(), []string{"remotr", "secret", "upload", "repositories/private", "--file", secretPath, "--fleet", "production", "--json"})
		if err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(stdout, "cli-secret-canary") || !strings.Contains(stdout, `"version": "1"`) {
		t.Fatalf("stdout = %q", stdout)
	}

	err = newApp().Run(context.Background(), []string{"remotr", "secret", "upload", "repositories/private", "plaintext-in-argv", "--file", secretPath, "--fleet", "production"})
	if err == nil || !strings.Contains(err.Error(), "exactly one secret name") {
		t.Fatalf("argv material err = %v", err)
	}
	if err := os.Chmod(secretPath, 0o644); err != nil {
		t.Fatal(err)
	}
	err = newApp().Run(context.Background(), []string{"remotr", "secret", "upload", "repositories/private", "--file", secretPath, "--fleet", "production"})
	if err == nil || !strings.Contains(err.Error(), "mode 0600") {
		t.Fatalf("permissive file err = %v", err)
	}
}
