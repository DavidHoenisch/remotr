package configcompose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
)

func TestRender_fleetModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base-packages.yaml"), kindModule(`configurations:
  - name: base-packages
    packages:
      - name: curl
        present: true
        packageManager: apt
`))
	writeFile(t, filepath.Join(dir, "modules", "sshd-hardening.yaml"), kindModule(`configurations:
  - name: ssh-hardening
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), kindManifest(`modules:
  - modules/base-packages.yaml
  - modules/sshd-hardening.yaml
`))

	body := renderFleetBody(t, dir, "engineering")
	if !strings.Contains(body, "base-packages") || !strings.Contains(body, "ssh-hardening") {
		t.Fatalf("missing configurations:\n%s", body)
	}
}

func TestRender_endpointExtendsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base-packages.yaml"), kindModule(`configurations:
  - name: base-packages
    targetDistros: [Debian]
    packages:
      - name: curl
        present: true
        packageManager: apt
`))
	writeFile(t, filepath.Join(dir, "modules", "designer-extra.yaml"), kindModule(`configurations:
  - name: designer-extra
    packages:
      - name: vim
        present: true
        packageManager: apt
`))
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), kindManifest(`modules:
  - modules/base-packages.yaml
`))
	writeFile(t, filepath.Join(dir, "endpoints", "workstation-42", "manifest.yaml"), kindManifest(`extends: fleets/engineering/manifest.yaml
modules:
  - modules/designer-extra.yaml
overrides:
  - name: base-packages
    packages:
      - name: curl
        present: true
        packageManager: apt
      - name: git
        present: true
        packageManager: apt
`))

	body := renderEndpointBody(t, dir, "workstation-42")
	if !strings.Contains(body, "designer-extra") {
		t.Fatalf("missing designer-extra:\n%s", body)
	}
	if !strings.Contains(body, "git") {
		t.Fatalf("override packages missing git:\n%s", body)
	}
}

func TestValidateComposition_duplicateConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "a.yaml"), kindModule(`configurations:
  - name: dup
    commands:
      - name: one
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "modules", "b.yaml"), kindModule(`configurations:
  - name: dup
    commands:
      - name: two
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/a.yaml
  - modules/b.yaml
`))

	res := validateComposition(t, dir)
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %#v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Message, "duplicate configuration") {
		t.Fatalf("issue = %#v", res.Issues[0])
	}
}

func TestValidateComposition_extendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fleets", "a", "manifest.yaml"), kindManifest(`extends: fleets/b/manifest.yaml
modules: []
`))
	writeFile(t, filepath.Join(dir, "fleets", "b", "manifest.yaml"), kindManifest(`extends: fleets/a/manifest.yaml
modules: []
`))

	res := validateComposition(t, dir)
	if len(res.Issues) < 2 {
		t.Fatalf("issues = %#v", res.Issues)
	}
}

func TestRenderStdout_desired(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
`))

	res, err := configcompose.RenderStdout(dir, "", "desired")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	if len(res.Rendered) != 1 {
		t.Fatalf("rendered = %#v", res.Rendered)
	}
	if !strings.Contains(res.Rendered[0].Content, "name: base") {
		t.Fatalf("content = %q", res.Rendered[0].Content)
	}
}

func TestRender_cronModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "crons", "modules", "weekly.yaml"), kindCrons(`crons:
  - use: builtin/system-upgrade-debian
    schedule: "0 0 * * 0"
`))
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
crons:
  - crons/modules/weekly.yaml
`))

	_, crons, _, _, err := configcompose.RenderFleet(dir, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if len(crons) == 0 || !strings.Contains(string(crons), "builtin/system-upgrade-debian") {
		t.Fatalf("crons = %s", crons)
	}
}

func TestHasManifests(t *testing.T) {
	dir := t.TempDir()
	ok, err := configcompose.HasManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no manifests")
	}
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
`))
	ok, err = configcompose.HasManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected manifests")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
