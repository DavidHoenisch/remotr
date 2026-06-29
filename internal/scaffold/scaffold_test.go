package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_writesLayout(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "repo")
	res, err := Init(context.Background(), Options{Dir: child, Fleet: "lab"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fleet != "lab" {
		t.Fatalf("fleet = %q", res.Fleet)
	}

	want := []string{
		".gitignore",
		"README.md",
		"remotr.yaml",
		"server.env.example",
		filepath.Join("modules", "base-packages.yaml"),
		filepath.Join("applications", ".gitkeep"),
		filepath.Join("fleets", "lab", "manifest.yaml"),
		filepath.Join("endpoints", ".gitkeep"),
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(child, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(child, "fleets", "lab", "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "kind: manifest") {
		t.Fatalf("manifest missing kind: %s", raw)
	}
}

func TestInit_rejectsNonemptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Init(context.Background(), Options{Dir: dir, Fleet: "lab"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInit_rejectsInvalidFleet(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Init(context.Background(), Options{Dir: dir, Fleet: "../bad"})
	if err == nil {
		t.Fatal("expected error")
	}
}
