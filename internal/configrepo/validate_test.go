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

func TestValidateRepository_samePackageNameDifferentPackageManager(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "engineering")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `configurations:
  - name: base-packages
    targetDistros:
      - Arch
      - Debian
    packages:
      - name: nmap
        present: true
        packageManager: pacman
      - name: nmap
        present: true
        packageManager: apt
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
}

func TestValidateRepository_duplicatePackageSameManager(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `configurations:
  - name: base
    packages:
      - name: nmap
        present: true
        packageManager: apt
      - name: nmap
        present: true
        packageManager: apt
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

func TestValidateRepository_invalidDownloadDest(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `configurations:
  - name: base
    downloads:
      - name: bin
        url: https://example.com/x
        dest: relative/path
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

func TestValidateRepository_agentInstallRequiresFileSecret(t *testing.T) {
	dir := t.TempDir()
	fleetDir := filepath.Join(dir, "fleets", "demo")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `configurations:
  - name: elastic-agent
    agentInstall:
      - name: elastic-agent
        version: "1.0"
        artifactURL: https://example.com/a.tar.gz
        extractDir: dir
        fleetURL: https://fleet.example
        enrollmentTokenSecret: super-secret-token
        runningCheck:
          process: elastic-agent
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
