package apppackages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateScaffold_binary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mycli")
	m, err := CreateScaffold(ScaffoldOptions{
		Dir:     dir,
		Name:    "internal/mycli",
		Version: "1.0.0",
		Mode:    "binary",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Install.Mode != "binary" {
		t.Fatalf("mode = %q", m.Install.Mode)
	}
	if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bin", "mycli")); err != nil {
		t.Fatal(err)
	}
}

func TestCreateScaffold_refusesExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exists"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := CreateScaffold(ScaffoldOptions{Dir: dir, Name: "demo/tool"})
	if err == nil {
		t.Fatal("expected error for existing path")
	}
}

func TestBuildZipFromDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pkg")
	if _, err := CreateScaffold(ScaffoldOptions{Dir: dir, Name: "demo/tool", Version: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	data, sum, err := BuildZipFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || sum.SHA256 == "" {
		t.Fatal("expected zip output")
	}
	if sum.Manifest.Name != "demo/tool" {
		t.Fatalf("name = %q", sum.Manifest.Name)
	}
}
