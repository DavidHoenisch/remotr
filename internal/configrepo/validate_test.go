package configrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRepository_validFleet(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "engineering", `configurations:
  - name: base-packages
    packages:
      - name: nmap
        present: true
        packageManager: pacman
`)

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
	if len(res.OK) < 2 {
		t.Fatalf("ok = %+v", res.OK)
	}
}

func TestValidateRepository_invalidEndpointID(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "engineering", `configurations:
  - name: x
`)
	epDir := filepath.Join(dir, "endpoints", "bad_id_with_underscore")
	if err := os.MkdirAll(epDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(epDir, "manifest.yaml"), []byte("kind: manifest\nmodules: []\n"), 0o644); err != nil {
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
	writeFleetModule(t, dir, "engineering", `configurations:
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
`)

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
	writeFleetModule(t, dir, "demo", `configurations:
  - name: base
    packages:
      - name: nmap
        present: true
        packageManager: apt
      - name: nmap
        present: true
        packageManager: apt
`)

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
	writeFleetModule(t, dir, "demo", `configurations:
  - name: base
    downloads:
      - name: bin
        url: https://example.com/x
        dest: relative/path
`)
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
	writeFleetModule(t, dir, "demo", `configurations:
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
`)
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
	writeFleetModule(t, dir, "demo", `configurations:
  - name: dup
  - name: dup
`)

	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_flatpakCustomRemoteRequiresURL(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: apps
    packages:
      - name: com.example.App
        present: true
        packageManager: flatpak
        flatpakRemote: company
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_flatpakFlathubWithoutURL(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: apps
    packages:
      - name: org.gnome.Calculator
        present: true
        packageManager: flatpak
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_remotrPackageRequiresVersion(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: apps
    packages:
      - name: internal/mycli
        present: true
        packageManager: remotr
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 || !strings.Contains(res.Issues[0].Message, "requires version") {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_remotrPackageValid(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: apps
    packages:
      - name: internal/mycli
        version: "1.4.0"
        present: true
        packageManager: remotr
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_versionOnlyAllowedForRemotr(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: base
    packages:
      - name: curl
        version: "1.0.0"
        present: true
        packageManager: apt
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 || !strings.Contains(res.Issues[0].Message, "only allowed with packageManager remotr") {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_pwaRequiresURL(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: apps
    packages:
      - name: slack
        present: true
        packageManager: pwa
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 || !strings.Contains(res.Issues[0].Message, "requires pwaURL") {
		t.Fatalf("issues = %+v", res.Issues)
	}
}

func TestValidateRepository_pwaValid(t *testing.T) {
	dir := t.TempDir()
	writeFleetModule(t, dir, "demo", `configurations:
  - name: apps
    packages:
      - name: slack
        present: true
        packageManager: pwa
        pwaURL: https://app.slack.com/client
        pwaTitle: Slack
        pwaIcon: https://example.com/icon.png
        pwaBrowser: chromium
`)
	res, err := ValidateRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues = %+v", res.Issues)
	}
}
