package desktoplayout_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopDevelopmentPublicationIsAdditiveLinuxOnlyAndReversible(t *testing.T) {
	root := repositoryRoot(t)

	releaseWorkflow := readReleaseContract(t, root, ".github/workflows/release.yml")
	for _, fragment := range []string{
		"name: Release",
		"goreleaser:",
		"args: release --clean",
		"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
	} {
		if !strings.Contains(releaseWorkflow, fragment) {
			t.Errorf("tagged CLI/agent release workflow lost independent contract %q", fragment)
		}
	}
	if strings.Contains(strings.ToLower(releaseWorkflow), "desktop") {
		t.Error("unsigned desktop development publication was coupled to the tagged CLI/agent release workflow")
	}

	desktopWorkflow := readReleaseContract(t, root, ".github/workflows/desktop.yml")
	for _, fragment := range []string{
		"permissions:\n  contents: read",
		"publish_development_snapshot:",
		"type: boolean",
		"default: true",
		"if: github.event_name == 'pull_request' || inputs.publish_development_snapshot",
		"run: make desktop-release-check",
		"desktop/build/package/release-manifest.json",
		"retention-days: 7",
	} {
		if !strings.Contains(desktopWorkflow, fragment) {
			t.Errorf("desktop development publication workflow does not contain %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"gh release upload",
		"softprops/action-gh-release",
		"secrets.",
		"runs-on: macos",
		"runs-on: windows",
	} {
		if strings.Contains(strings.ToLower(desktopWorkflow), strings.ToLower(forbidden)) {
			t.Errorf("desktop development publication workflow contains forbidden release coupling %q", forbidden)
		}
	}

	targetData, err := os.ReadFile(filepath.Join(root, "desktop", "build", "linux", "package-targets.json"))
	if err != nil {
		t.Fatalf("read desktop package target policy: %v", err)
	}
	var targets struct {
		SignedReleaseOutput struct {
			Configured bool   `json:"configured"`
			Policy     string `json:"policy"`
		} `json:"signedReleaseOutput"`
		Artifacts []struct {
			OS              string `json:"os"`
			Publication     string `json:"publication"`
			SigningStatus   string `json:"signingStatus"`
			ReleaseEligible bool   `json:"releaseEligible"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(targetData, &targets); err != nil {
		t.Fatalf("parse desktop package target policy: %v", err)
	}
	if targets.SignedReleaseOutput.Configured || targets.SignedReleaseOutput.Policy != "not-configured" {
		t.Errorf("signed release policy = %#v, want explicitly not configured", targets.SignedReleaseOutput)
	}
	if len(targets.Artifacts) != 1 {
		t.Fatalf("desktop publication targets = %d, want one evidenced Linux development target", len(targets.Artifacts))
	}
	target := targets.Artifacts[0]
	if target.OS != "linux" || target.Publication != "ci-development-artifact" || target.SigningStatus != "unsigned" || target.ReleaseEligible {
		t.Errorf("desktop publication target = %#v, want unsigned non-release Linux CI artifact", target)
	}

	maintainerGuide := readReleaseContract(t, root, "docs/guides/installing-cli.md")
	for _, fragment := range []string{
		"Tagged CLI and agent release",
		"Desktop development publication",
		"not attached to the GitHub Release",
		"publish_development_snapshot",
		"no redistribution license",
		"No signed Remotr Desktop release output is configured",
		"does not require a server migration",
		"does not rotate or move Operator credentials",
	} {
		if !strings.Contains(maintainerGuide, fragment) {
			t.Errorf("maintainer release guide does not cover %q", fragment)
		}
	}

	desktopReference := readReleaseContract(t, root, "docs/reference/remotr-desktop.md")
	for _, fragment := range []string{
		"Publication and distribution rollback",
		"independent of the tagged CLI and agent release",
		"Disable desktop artifact upload",
		"leave the server, Admin API, database, and Operator credential directories unchanged",
	} {
		if !strings.Contains(desktopReference, fragment) {
			t.Errorf("desktop release reference does not cover %q", fragment)
		}
	}
}

func readReleaseContract(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read release contract %s: %v", path, err)
	}
	return string(data)
}
