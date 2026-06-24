package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackage_createAndBuild(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo-cli")
	app := newApp()
	if err := app.Run(context.Background(), []string{
		"remotr", "package", "create",
		"--path", dir,
		"--name", "demo/cli",
		"--version", "0.1.0",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "remotr-package.yaml")); err != nil {
		t.Fatal(err)
	}

	outZip := filepath.Join(t.TempDir(), "demo-cli.zip")
	if err := app.Run(context.Background(), []string{
		"remotr", "package", "build",
		"--path", dir,
		"--output", outZip,
	}); err != nil {
		t.Fatalf("build: %v", err)
	}
	data, err := os.ReadFile(outZip)
	if err != nil || len(data) == 0 {
		t.Fatalf("zip: %v", err)
	}
}

func TestPackage_createRequiresPath(t *testing.T) {
	app := newApp()
	err := app.Run(context.Background(), []string{"remotr", "package", "create"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "path") {
		t.Fatalf("err = %v", err)
	}
}
