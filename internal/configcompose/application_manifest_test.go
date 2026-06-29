package configcompose_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
)

func TestCompose_applicationNestedOrganization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "pwa", "microsoft", "teams.yaml"), `name: teams
present: true
packageManager: pwa
pwaURL: https://teams.microsoft.com
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - pwa/microsoft/teams
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	got, err := os.ReadFile(filepath.Join(dir, "fleets", "lab", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "name: teams") {
		t.Fatalf("expected teams package:\n%s", got)
	}
}

func TestCompose_applicationCrawlBasename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "pwa", "microsoft", "teams.yaml"), `name: teams
present: true
packageManager: pwa
pwaURL: https://teams.microsoft.com
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - teams
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
}

func TestCompose_applicationAmbiguousBasename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "vendor-a", "tool.yaml"), `name: tool
present: true
packageManager: pwa
pwaURL: https://a.example
`)
	writeFile(t, filepath.Join(dir, "applications", "vendor-b", "tool.yaml"), `name: tool-b
present: true
packageManager: pwa
pwaURL: https://b.example
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - tool
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %#v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Message, "ambiguous") {
		t.Fatalf("issue = %#v", res.Issues[0])
	}
}

func TestCompose_applicationSamePackageNameDifferentTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "clamav", "debian.yaml"), `configuration:
  name: apps/clamav-debian
  targetDistros: [Debian]
packages:
  - name: clamav
    present: true
    packageManager: apt
`)
	writeFile(t, filepath.Join(dir, "applications", "clamav", "arch.yaml"), `configuration:
  name: apps/clamav-arch
  targetDistros: [Arch]
name: clamav
present: true
packageManager: pacman
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - clamav/debian
  - clamav/arch
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
}

func TestCompose_applicationShortName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "slack.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - slack
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	got, err := os.ReadFile(filepath.Join(dir, "fleets", "lab", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "name: slack") {
		t.Fatalf("expected slack in desired.yaml:\n%s", got)
	}
}

func TestCompose_sharedApplicationsManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "slack.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`)
	writeFile(t, filepath.Join(dir, "applications", "github.yaml"), `name: github
present: true
packageManager: pwa
pwaURL: https://github.com
`)
	writeFile(t, filepath.Join(dir, "applications", "manifest.yaml"), `modules:
  - slack
`)
	writeFile(t, filepath.Join(dir, "fleets", "eng", "applications.manifest.yaml"), `extends: applications/manifest.yaml
modules:
  - github
`)
	writeFile(t, filepath.Join(dir, "fleets", "eng", "manifest.yaml"), `modules:
  - modules/base.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "ops", "applications.manifest.yaml"), `extends: applications/manifest.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "ops", "manifest.yaml"), `modules:
  - modules/base.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}

	eng, err := os.ReadFile(filepath.Join(dir, "fleets", "eng", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	engBody := string(eng)
	if !strings.Contains(engBody, "name: slack") || !strings.Contains(engBody, "name: github") {
		t.Fatalf("eng fleet should include shared + fleet apps:\n%s", engBody)
	}

	ops, err := os.ReadFile(filepath.Join(dir, "fleets", "ops", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	opsBody := string(ops)
	if !strings.Contains(opsBody, "name: slack") {
		t.Fatalf("ops fleet should inherit shared catalog:\n%s", opsBody)
	}
	if strings.Contains(opsBody, "name: github") {
		t.Fatalf("ops fleet should not include eng-only app:\n%s", opsBody)
	}
}

func TestCompose_applicationsManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base-packages.yaml"), `configurations:
  - name: base-packages
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "slack.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://app.slack.com/client
`)
	writeFile(t, filepath.Join(dir, "applications", "bundles", "tools.yaml"), `packages:
  - name: e2e/test-cli
    present: true
    packageManager: remotr
    version: "1.0.0"
`)
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "applications.manifest.yaml"), `modules:
  - applications/modules/slack.yaml
  - applications/bundles/tools.yaml
overrides:
  - name: e2e/test-cli
    version: "1.1.0"
`)
	writeFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), `modules:
  - modules/base-packages.yaml
applications: fleets/engineering/applications.manifest.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}

	got, err := os.ReadFile(filepath.Join(dir, "fleets", "engineering", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, "name: applications") {
		t.Fatalf("missing applications slice:\n%s", body)
	}
	if !strings.Contains(body, "name: slack") {
		t.Fatalf("missing slack package:\n%s", body)
	}
	if !strings.Contains(body, "version: 1.1.0") {
		t.Fatalf("override version missing:\n%s", body)
	}
}

func TestCompose_applicationsInlineList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "slack.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - applications/modules/slack.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	got, err := os.ReadFile(filepath.Join(dir, "fleets", "lab", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "name: applications") {
		t.Fatalf("expected applications slice:\n%s", got)
	}
}

func TestCompose_applicationsSiblingManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "slack.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "applications.manifest.yaml"), `modules:
  - applications/modules/slack.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
}

func TestCompose_applicationsInheritFromFleet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "slack.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`)
	writeFile(t, filepath.Join(dir, "fleets", "eng", "applications.manifest.yaml"), `modules:
  - applications/modules/slack.yaml
`)
	writeFile(t, filepath.Join(dir, "fleets", "eng", "manifest.yaml"), `modules:
  - modules/base.yaml
`)
	writeFile(t, filepath.Join(dir, "endpoints", "ws", "manifest.yaml"), `extends: fleets/eng/manifest.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	got, err := os.ReadFile(filepath.Join(dir, "endpoints", "ws", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "name: slack") {
		t.Fatalf("endpoint should inherit fleet applications:\n%s", got)
	}
}

func TestCompose_applicationsDedicatedSlice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "calc.yaml"), `targetDistros: [Debian]
name: org.gnome.Calculator
present: true
packageManager: flatpak
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - applications/modules/calc.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	got, err := os.ReadFile(filepath.Join(dir, "fleets", "lab", "desired.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	if !strings.Contains(body, "name: applications/org.gnome.Calculator") {
		t.Fatalf("expected dedicated slice:\n%s", body)
	}
	if strings.Contains(body, "name: applications\n") {
		t.Fatalf("unexpected shared applications slice:\n%s", body)
	}
}

func TestCompose_duplicateApplicationPackage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "a.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://a.example
`)
	writeFile(t, filepath.Join(dir, "applications", "modules", "b.yaml"), `name: slack
present: true
packageManager: pwa
pwaURL: https://b.example
`)
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), `modules:
  - modules/base.yaml
applications:
  - applications/modules/a.yaml
  - applications/modules/b.yaml
`)

	res, err := configcompose.Compose(configcompose.Options{RepoRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %#v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Message, "duplicate application package") {
		t.Fatalf("issue = %#v", res.Issues[0])
	}
}
