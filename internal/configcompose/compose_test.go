package configcompose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompose_fleetModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base-packages.yaml"), `configurations:
  - name: base-packages
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	writeFile(t, filepath.Join(dir, "modules", "sshd-hardening.yaml"), `configurations:
  - name: ssh-hardening
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), `modules:
  - modules/base-packages.yaml
  - modules/sshd-hardening.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	if len(res.Written) != 1 || res.Written[0] != "fleets/engineering/desired.yaml" {
		t.Fatalf("written = %#v", res.Written)
	}

	got, err := os.ReadFile(filepath.Join(dir, "fleets", "engineering", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, "base-packages") || !strings.Contains(body, "ssh-hardening") {
		t.Fatalf("desired.yaml missing configurations:\n%s", body)
	}
}

func TestCompose_endpointExtendsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base-packages.yaml"), `configurations:
  - name: base-packages
    targetDistros: [Debian]
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	writeFile(t, filepath.Join(dir, "modules", "designer-extra.yaml"), `configurations:
  - name: designer-extra
    packages:
      - name: vim
        present: true
        packageManager: apt
`)
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), `modules:
  - modules/base-packages.yaml
`)
	writeFile(t, filepath.Join(dir, "endpoints", "workstation-42", "manifest.yaml"), `extends: fleets/engineering/manifest.yaml
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
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	if len(res.Written) != 2 {
		t.Fatalf("written = %#v", res.Written)
	}

	got, err := os.ReadFile(filepath.Join(dir, "endpoints", "workstation-42", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, "designer-extra") {
		t.Fatalf("missing designer-extra:\n%s", body)
	}
	if !strings.Contains(body, "git") {
		t.Fatalf("override packages missing git:\n%s", body)
	}
	if strings.Contains(body, "targetDistros") {
		// override replaced packages only; targetDistros from base should remain
	}
}

func TestCompose_duplicateConfiguration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "a.yaml"), `configurations:
  - name: dup
    commands:
      - name: one
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "modules", "b.yaml"), `configurations:
  - name: dup
    commands:
      - name: two
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/a.yaml
  - modules/b.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %#v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Message, "duplicate configuration") {
		t.Fatalf("issue = %#v", res.Issues[0])
	}
}

func TestCompose_extendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fleets", "a", "manifest.yaml"), `extends: fleets/b/manifest.yaml
modules: []
`)
	writeFile(t, filepath.Join(dir, "fleets", "b", "manifest.yaml"), `extends: fleets/a/manifest.yaml
modules: []
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 2 {
		t.Fatalf("issues = %#v", res.Issues)
	}
}

func TestCompose_fleetFilter(t *testing.T) {
	dir := t.TempDir()
	writeModule := func(name string) {
		writeFile(t, filepath.Join(dir, "modules", name+".yaml"), `configurations:
  - name: `+name+`
    commands:
      - name: noop
        apply: [true]
`)
	}
	writeModule("base")
	writeModule("other")
	writeFile(t, filepath.Join(dir, "fleets", "eng", "manifest.yaml"), `modules:
  - modules/base.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "ops", "manifest.yaml"), `modules:
  - modules/other.yaml
`)
	writeFile(t, filepath.Join(dir, "endpoints", "ws", "manifest.yaml"), `extends: fleets/eng/manifest.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir, Fleet: "eng"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	if len(res.Written) != 2 {
		t.Fatalf("written = %#v", res.Written)
	}
}

func TestCompose_checkStale(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "desired.yaml"), `configurations:
  - name: stale
    commands:
      - name: noop
        apply: [true]
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir, Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stale) != 1 || res.Stale[0] != "fleets/lab/desired.yaml" {
		t.Fatalf("stale = %#v", res.Stale)
	}
}

func TestCompose_dryRunDiff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "desired.yaml"), `configurations:
  - name: stale
    commands:
      - name: noop
        apply: [true]
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stale) != 1 || len(res.Diffs) != 1 {
		t.Fatalf("stale=%#v diffs=%#v", res.Stale, res.Diffs)
	}
	if res.Diffs[0].Text == "" {
		t.Fatal("expected diff text")
	}
}

func TestCompose_cronModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "crons", "modules", "weekly.yaml"), `crons:
  - use: builtin/system-upgrade-debian
    schedule: "0 0 * * 0"
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "crons.manifest.yaml"), `modules:
  - crons/modules/weekly.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	if len(res.Written) != 1 || res.Written[0] != "fleets/lab/crons.yaml" {
		t.Fatalf("written = %#v", res.Written)
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
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
`)
	ok, err = configcompose.HasManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected manifests")
	}
}
