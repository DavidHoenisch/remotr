package configrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFleetArtifact_readsDesiredYAML(t *testing.T) {
	dir := t.TempDir()
	fleet := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleet, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("configurations: []\n")
	if err := os.WriteFile(filepath.Join(fleet, "desired.yaml"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, digest, err := FleetArtifact(dir, "demo")
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

func TestFleetArtifact_rejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, fleet := range []string{"", "..", "../x", `a/b`} {
		if _, _, err := FleetArtifact(dir, fleet); err == nil {
			t.Fatalf("fleet %q should fail", fleet)
		}
	}
}

func TestEndpointArtifact_readsOverrideYAML(t *testing.T) {
	dir := t.TempDir()
	id := "11111111-1111-1111-1111-111111111111"
	epDir := filepath.Join(dir, "endpoints", id)
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("configurations:\n  - name: override\n")
	if err := os.WriteFile(filepath.Join(epDir, "desired.yaml"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	got, digest, err := EndpointArtifact(dir, id)
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

func TestResolveArtifact_prefersEndpointOverride(t *testing.T) {
	dir := t.TempDir()
	id := "11111111-1111-1111-1111-111111111111"

	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fleetYAML := []byte("configurations:\n  - name: fleet\n")
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), fleetYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	epDir := filepath.Join(dir, "endpoints", id)
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	overrideYAML := []byte("configurations:\n  - name: override\n")
	if err := os.WriteFile(filepath.Join(epDir, "desired.yaml"), overrideYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := ResolveArtifact(dir, "demo", id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(overrideYAML) {
		t.Fatalf("artifact = %q", got)
	}
}

func TestResolveArtifact_fallsBackToFleet(t *testing.T) {
	dir := t.TempDir()
	id := "11111111-1111-1111-1111-111111111111"

	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fleetYAML := []byte("configurations:\n  - name: fleet\n")
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), fleetYAML, 0o644); err != nil {
		t.Fatal(err)
	}

	got, _, err := ResolveArtifact(dir, "demo", id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(fleetYAML) {
		t.Fatalf("artifact = %q", got)
	}
}

func TestValidateEndpointID_rejectsInvalid(t *testing.T) {
	for _, id := range []string{"", "ab", "has space", "../x"} {
		if err := ValidateEndpointID(id); err == nil {
			t.Fatalf("id %q should fail", id)
		}
	}
}
