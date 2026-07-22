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

func TestSecretListEnumeratesLogicalSecretsAndShowDisplaysVersionHistory(t *testing.T) {
	dir := t.TempDir()
	fixturesDir := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(fixturesDir, 0o700); err != nil {
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
	t.Setenv("REMOTR_DEMO", "1")
	t.Setenv("REMOTR_DEMO_FIXTURES", fixturesDir)
	t.Setenv("REMOTR_SERVER_URL", "https://demo.remotr.example")
	t.Setenv("REMOTR_OPERATOR_STATE_DIR", stateDir)
	fixturePath := filepath.Join(fixturesDir, "GET_v1_admin_secrets.json")
	writeFixture := func(body any) {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"status": 200, "body": body})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixturePath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeFixture(map[string]any{"items": []map[string]any{{
		"name": "ubuntu-pro/shared", "scope": "global", "activeVersion": "1", "versionCount": 2,
	}}, "nextCursor": ""})
	listed := captureStdout(t, func() {
		if err := newApp().Run(context.Background(), []string{"remotr", "secret", "list", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(listed, `"name": "ubuntu-pro/shared"`) || !strings.Contains(listed, `"scope": "global"`) {
		t.Fatalf("secret list output = %q", listed)
	}
	if err := newApp().Run(context.Background(), []string{"remotr", "secret", "list", "ubuntu-pro/shared"}); err == nil || !strings.Contains(err.Error(), "remotr secret show ubuntu-pro/shared") {
		t.Fatalf("legacy list migration error = %v", err)
	}

	writeFixture([]map[string]any{{
		"name": "ubuntu-pro/shared", "version": "1", "scope": "global", "fingerprint": "sha256:safe",
		"active": true, "createdAt": time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), "createdBy": "operator-1", "revoked": false, "resolutionBlocked": false,
	}})
	shown := captureStdout(t, func() {
		if err := newApp().Run(context.Background(), []string{"remotr", "secret", "show", "ubuntu-pro/shared", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(shown, `"version": "1"`) || strings.Contains(shown, "plaintext") {
		t.Fatalf("secret show output = %q", shown)
	}
}

func TestSecretShowWithoutIDFailsPromptlyForStructuredOrNonInteractiveUse(t *testing.T) {
	for _, args := range [][]string{
		{"remotr", "secret", "show", "--json"},
		{"remotr", "secret", "show"},
	} {
		err := newApp().Run(context.Background(), args)
		if err == nil || !strings.Contains(err.Error(), "requires a secret ID") || !strings.Contains(err.Error(), "remotr secret list") {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
}

func TestSecretUploadScopeFlagsAreExplicitAndMutuallyExclusive(t *testing.T) {
	for _, args := range [][]string{
		{"remotr", "secret", "upload", "shared/token", "--file", "-"},
		{"remotr", "secret", "upload", "shared/token", "--file", "-", "--global", "--fleet", "engineering"},
		{"remotr", "secret", "upload", "shared/token", "--file", "-", "--fleet", "engineering", "--endpoint", "endpoint-1"},
	} {
		err := newApp().Run(context.Background(), args)
		if err == nil || !strings.Contains(err.Error(), "exactly one of --global, --fleet, or --endpoint") {
			t.Fatalf("args=%v err=%v", args, err)
		}
	}
}
