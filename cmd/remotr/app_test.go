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

func TestApp_endpointAgentUpgradeRequiresVersion(t *testing.T) {
	app := newApp()
	err := app.Run(context.Background(), []string{"remotr", "endpoint", "agent", "upgrade", "test-id"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("err = %v", err)
	}
}

func TestApp_endpointShowAcceptsIDBeforeFlags(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("server_url: https://example.invalid\nstate_dir: "+stateDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := newApp()
	err := app.Run(context.Background(), []string{
		"remotr", "--config", cfgPath,
		"endpoint", "show", "phalanx-acae925c",
	})
	if err == nil {
		t.Fatal("expected error (no credentials)")
	}
	if !strings.Contains(err.Error(), "credentials missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestApp_globalFlagsAfterSubcommand(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("state_dir: "+stateDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := newApp()
	err := app.Run(context.Background(), []string{
		"remotr", "endpoint", "list",
		"--server-url", "https://example.invalid",
		"--config", cfgPath,
	})
	if err == nil {
		t.Fatal("expected error (no credentials)")
	}
	if !strings.Contains(err.Error(), "credentials missing") {
		t.Fatalf("expected credentials error after trailing global flags, got: %v", err)
	}
}

func TestApp_endpointShowGlobalFlagAfterID(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("state_dir: "+stateDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := newApp()
	err := app.Run(context.Background(), []string{
		"remotr", "endpoint", "show", "phalanx-acae925c",
		"--server-url", "https://example.invalid",
		"--config", cfgPath,
	})
	if err == nil {
		t.Fatal("expected error (no credentials)")
	}
	if !strings.Contains(err.Error(), "credentials missing") {
		t.Fatalf("expected credentials error, got: %v", err)
	}
}

func TestApp_appListFormatJSON(t *testing.T) {
	dir := t.TempDir()
	fixturesDir := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(fixturesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := struct {
		Status int              `json:"status"`
		Body   []map[string]any `json:"body"`
	}{
		Status: 200,
		Body: []map[string]any{
			{
				"id":         "pkg-1",
				"name":       "demo/cli",
				"version":    "0.1.0",
				"s3_key":     "app-packages/demo/cli/0.1.0/pkg.zip",
				"sha256":     "abc123",
				"manifest":   map[string]any{"name": "demo/cli", "version": "0.1.0", "install": map[string]any{"mode": "binary"}},
				"created_at": time.Date(2026, 6, 24, 4, 36, 35, 0, time.UTC).Format(time.RFC3339),
			},
		},
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixturesDir, "GET_v1_admin_app-packages.json"), raw, 0o644); err != nil {
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

	stdout := captureStdout(t, func() {
		app := newApp()
		if err := app.Run(context.Background(), []string{"remotr", "app", "list", "--format", "json"}); err != nil {
			t.Fatalf("app list: %v", err)
		}
	})

	var items []map[string]any
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("stdout is not JSON: %v\nstdout = %q", err, stdout)
	}
	if len(items) != 1 || items[0]["name"] != "demo/cli" {
		t.Fatalf("stdout = %q", stdout)
	}
}
