package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApp_aiListJSON(t *testing.T) {
	app := newApp()
	out := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"remotr", "ai", "list", "--json", "--scope", "project"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, `"agent": "claude"`) || !strings.Contains(out, `"agent": "cursor"`) {
		t.Fatalf("output = %q", out)
	}
}

func TestApp_aiSetupEmbedded(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	app := newApp()
	if err := app.Run(context.Background(), []string{
		"remotr", "ai", "setup", "--agent", "cursor", "--scope", "project", "--force",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "skills", "remotr-agent", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestApp_aiSetupRequiresAgent(t *testing.T) {
	app := newApp()
	err := app.Run(context.Background(), []string{"remotr", "ai", "setup"})
	if err == nil {
		t.Fatal("expected error")
	}
}
