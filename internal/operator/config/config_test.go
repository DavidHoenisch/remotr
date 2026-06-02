package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_prefersFlagsOverFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`server_url: https://file.example
state_dir: /from/file
fleet: file-fleet
`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Resolve(cfgPath, "https://flag.example", "", "", "flag-fleet")
	if err != nil {
		t.Fatal(err)
	}
	if s.ServerURL != "https://flag.example" {
		t.Fatalf("server_url = %q", s.ServerURL)
	}
	if s.StateDir != "/from/file" {
		t.Fatalf("state_dir = %q", s.StateDir)
	}
	if s.Fleet != "flag-fleet" {
		t.Fatalf("fleet = %q", s.Fleet)
	}
}

func TestResolve_defaultsCAToStateDir(t *testing.T) {
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(ca, []byte("pem"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Resolve("", "", dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if s.CA != ca {
		t.Fatalf("ca = %q, want %q", s.CA, ca)
	}
}

func TestResolve_expandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "state_dir: ~/remotr-state\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Resolve(cfgPath, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "remotr-state")
	if s.StateDir != want {
		t.Fatalf("state_dir = %q, want %q", s.StateDir, want)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REMOTR_CONFIG", filepath.Join(dir, "config.yaml"))

	in := Settings{
		ServerURL: "https://example.fly.dev",
		StateDir:  dir,
		Fleet:     "default",
	}
	if err := Save(in); err != nil {
		t.Fatal(err)
	}

	got, err := Load(DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerURL != in.ServerURL || got.StateDir != in.StateDir || got.Fleet != in.Fleet {
		t.Fatalf("loaded=%+v want=%+v", got, in)
	}
}
