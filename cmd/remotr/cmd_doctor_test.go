package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindConfigRepoRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "fleets"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := findConfigRepoRoot(dir)
	if got != dir {
		t.Fatalf("findConfigRepoRoot = %q want %q", got, dir)
	}
}

func TestActionRootGettingStarted(t *testing.T) {
	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "remotr bootstrap") || !strings.Contains(out, "remotr doctor") {
		t.Fatalf("getting started output = %q", out)
	}
}

func TestDoctorJSONMissingCredentials(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("server_url: https://example.invalid\nstate_dir: "+filepath.Join(dir, "state")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr", "--config", cfg, "doctor", "--json", "--skip-network"}); err == nil {
			t.Fatal("expected doctor to fail")
		}
	})
	if !strings.Contains(out, `"ok": false`) {
		t.Fatalf("doctor json = %q", out)
	}
}
