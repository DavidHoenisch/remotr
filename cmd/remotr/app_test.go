package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
