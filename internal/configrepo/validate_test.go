package configrepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRepository_validFleet(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "engineering")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `configurations:
  - name: base-packages
    packages:
      - name: nmap
        present: true
        packageManager: pacman
`
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
	if len(res.OK) != 1 || res.OK[0] != filepath.Join("fleets", "engineering", "desired.yaml") {
		t.Fatalf("ok = %+v", res.OK)
	}
}

func TestValidateRepository_invalidEndpointID(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "engineering")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte("configurations:\n  - name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	epDir := filepath.Join(dir, "endpoints", "bad_id_with_underscore")
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epDir, "desired.yaml"), []byte("configurations:\n  - name: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_duplicateConfiguration(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `configurations:
  - name: dup
  - name: dup
`
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}
