package configrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFleetCronArtifact_readsCronsYAML(t *testing.T) {
	dir := t.TempDir()
	fleet := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleet, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("crons: []\n")
	if err := os.WriteFile(filepath.Join(fleet, "crons.yaml"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, digest, err := FleetCronArtifact(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("artifact = %q", got)
	}
	if digest == "" {
		t.Fatal("expected digest")
	}
}

func TestResolveCronArtifact_prefersEndpointOverride(t *testing.T) {
	dir := t.TempDir()
	id := "11111111-1111-1111-1111-111111111111"

	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fleetYAML := []byte("crons:\n  - name: fleet\n    schedule: \"0 0 * * 0\"\n    commands:\n      - name: run\n        apply: [true]\n")
	if err := os.WriteFile(filepath.Join(fleetDir, "crons.yaml"), fleetYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	epDir := filepath.Join(dir, "endpoints", id)
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	epYAML := []byte("crons:\n  - name: override\n    schedule: \"0 1 * * 0\"\n    commands:\n      - name: run\n        apply: [true]\n")
	if err := os.WriteFile(filepath.Join(epDir, "crons.yaml"), epYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, ok, err := ResolveCronArtifact(dir, "demo", id)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected crons artifact")
	}
	if string(got) != string(epYAML) {
		t.Fatalf("artifact = %q", got)
	}
}

func TestResolveCronArtifact_missingFiles(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations:\n  - name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, ok, err := ResolveCronArtifact(dir, "demo", "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no crons artifact")
	}
}
