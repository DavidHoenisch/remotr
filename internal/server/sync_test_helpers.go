package server

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFleetDesired(t *testing.T, repoDir, fleet, configurationsYAML string) {
	t.Helper()
	modulePath := filepath.Join(repoDir, "modules", fleet+"-module.yaml")
	if err := os.MkdirAll(filepath.Dir(modulePath), 0o750); err != nil {
		t.Fatal(err)
	}
	module := "kind: module\n" + configurationsYAML
	if err := os.WriteFile(modulePath, []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	fleetDir := filepath.Join(repoDir, "fleets", fleet)
	if err := os.MkdirAll(fleetDir, 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := "kind: manifest\nmodules:\n  - modules/" + fleet + "-module.yaml\n"
	if err := os.WriteFile(filepath.Join(fleetDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestFleetWithCrons(t *testing.T, repoDir, fleet, configurationsYAML, cronsYAML string) {
	t.Helper()
	writeTestFleetDesired(t, repoDir, fleet, configurationsYAML)
	cronsPath := filepath.Join(repoDir, "crons", fleet+"-crons.yaml")
	if err := os.MkdirAll(filepath.Dir(cronsPath), 0o750); err != nil {
		t.Fatal(err)
	}
	cronsFile := "kind: crons\n" + cronsYAML
	if err := os.WriteFile(cronsPath, []byte(cronsFile), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repoDir, "fleets", fleet, "manifest.yaml")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(manifestBytes) + "crons:\n  - crons/" + fleet + "-crons.yaml\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestEndpointOverride(t *testing.T, repoDir, endpointID, configurationsYAML string) {
	t.Helper()
	epDir := filepath.Join(repoDir, "endpoints", endpointID)
	if err := os.MkdirAll(epDir, 0o750); err != nil {
		t.Fatal(err)
	}
	modulePath := filepath.Join(repoDir, "modules", endpointID+"-override.yaml")
	module := "kind: module\n" + configurationsYAML
	if err := os.WriteFile(modulePath, []byte(module), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "kind: manifest\nmodules:\n  - modules/" + endpointID + "-override.yaml\n"
	if err := os.WriteFile(filepath.Join(epDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
