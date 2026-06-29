package configcompose_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
)

func TestRender_applicationNestedOrganization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "pwa", "microsoft", "teams.yaml"), kindApplication(`name: teams
present: true
packageManager: pwa
pwaURL: https://teams.microsoft.com
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - pwa/microsoft/teams
`))

	body := renderFleetBody(t, dir, "lab")
	if !strings.Contains(body, "name: teams") {
		t.Fatalf("expected teams package:\n%s", body)
	}
}

func TestRender_applicationCrawlBasename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "pwa", "microsoft", "teams.yaml"), kindApplication(`name: teams
present: true
packageManager: pwa
pwaURL: https://teams.microsoft.com
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - teams
`))

	res := validateComposition(t, dir)
	if len(res.Issues) > 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
}

func TestValidateComposition_applicationAmbiguousBasename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "vendor-a", "tool.yaml"), kindApplication(`name: tool
present: true
packageManager: pwa
pwaURL: https://a.example
`))
	writeFile(t, filepath.Join(dir, "applications", "vendor-b", "tool.yaml"), kindApplication(`name: tool-b
present: true
packageManager: pwa
pwaURL: https://b.example
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - tool
`))

	res := validateComposition(t, dir)
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %#v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Message, "ambiguous") {
		t.Fatalf("issue = %#v", res.Issues[0])
	}
}

func TestRender_applicationDedicatedSlice(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "modules", "calc.yaml"), kindApplication(`targetDistros: [Debian]
name: org.gnome.Calculator
present: true
packageManager: flatpak
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - applications/modules/calc.yaml
`))

	body := renderFleetBody(t, dir, "lab")
	if !strings.Contains(body, "name: applications/org.gnome.Calculator") {
		t.Fatalf("expected dedicated slice:\n%s", body)
	}
}

func TestRender_applicationsInlineList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "modules", "slack.yaml"), kindApplication(`name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - applications/modules/slack.yaml
`))

	body := renderFleetBody(t, dir, "lab")
	if !strings.Contains(body, "name: applications") {
		t.Fatalf("expected applications slice:\n%s", body)
	}
}

func TestRender_applicationsInheritFromFleet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "modules", "slack.yaml"), kindApplication(`name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`))
	writeFile(t, filepath.Join(dir, "fleets", "eng", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - applications/modules/slack.yaml
`))
	writeFile(t, filepath.Join(dir, "endpoints", "11111111-1111-1111-1111-111111111111", "manifest.yaml"), kindManifest(`extends: fleets/eng/manifest.yaml
`))

	body := renderEndpointBody(t, dir, "11111111-1111-1111-1111-111111111111")
	if !strings.Contains(body, "name: slack") {
		t.Fatalf("endpoint should inherit fleet applications:\n%s", body)
	}
}

func TestValidateComposition_duplicateApplicationPackage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "modules", "a.yaml"), kindApplication(`name: slack
present: true
packageManager: pwa
pwaURL: https://a.example
`))
	writeFile(t, filepath.Join(dir, "applications", "modules", "b.yaml"), kindApplication(`name: slack
present: true
packageManager: pwa
pwaURL: https://b.example
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - applications/modules/a.yaml
  - applications/modules/b.yaml
`))

	res := validateComposition(t, dir)
	if len(res.Issues) != 1 {
		t.Fatalf("issues = %#v", res.Issues)
	}
	if !strings.Contains(res.Issues[0].Message, "duplicate application package") {
		t.Fatalf("issue = %#v", res.Issues[0])
	}
}

func TestDiscoverFleet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "modules", "base.yaml"), kindModule(`configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`))
	writeFile(t, filepath.Join(dir, "applications", "slack.yaml"), kindApplication(`name: slack
present: true
packageManager: pwa
pwaURL: https://example.com
`))
	writeFile(t, filepath.Join(dir, "fleets", "lab", "manifest.yaml"), kindManifest(`modules:
  - modules/base.yaml
applications:
  - applications/slack.yaml
`))

	summary, err := configcompose.DiscoverFleet(dir, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if summary.Manifest == "" || len(summary.Modules) != 1 || len(summary.Applications) != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
