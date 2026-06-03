package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApp_endpointAgentUpgradeRequiresVersion(t *testing.T) {
	app := newApp()
	err := app.Run([]string{"remotr", "endpoint", "agent", "upgrade", "test-id"})
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
	err := app.Run([]string{
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
