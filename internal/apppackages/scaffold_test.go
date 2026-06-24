package apppackages

import (
	"os"
	"path/filepath"
	"strings"
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
	if len(m.Install.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(m.Install.Files))
	}

	for _, rel := range []string{
		ManifestName,
		"bin/mycli-linux-amd64",
		"bin/mycli-linux-arm64",
		"lib/mycli-helper",
		"share/mycli.conf.example",
		"install.sh",
		"uninstall.sh",
		"requirements.txt",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"mode: binary",
		"arch: x86",
		"arch: ARM",
		"# script:",
		"# build:",
		"# uninstall:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("manifest missing %q", want)
		}
	}
}

func TestCreateScaffold_script(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tool")
	m, err := CreateScaffold(ScaffoldOptions{
		Dir:  dir,
		Name: "demo/tool",
		Mode: "script",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Install.Mode != "script" {
		t.Fatalf("mode = %q", m.Install.Mode)
	}
	if len(m.Install.Script) != 1 || m.Install.Script[0] != "./install.sh" {
		t.Fatalf("script = %#v", m.Install.Script)
	}
}

func TestCreateScaffold_build(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "app")
	m, err := CreateScaffold(ScaffoldOptions{
		Dir:  dir,
		Name: "demo/app",
		Mode: "build",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Install.Mode != "build" {
		t.Fatalf("mode = %q", m.Install.Mode)
	}
	if len(m.Install.Build) != 2 {
		t.Fatalf("build steps = %d", len(m.Install.Build))
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
