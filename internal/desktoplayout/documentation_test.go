package desktoplayout_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopGuidesCoverLinuxSetupCredentialsGitParityAndRecovery(t *testing.T) {
	root := repositoryRoot(t)

	developer := readDocumentation(t, root, "docs/guides/develop-remotr-desktop.md")
	for _, fragment := range []string{
		"# Develop Remotr Desktop on Linux",
		"Go 1.26",
		"Node.js 24",
		"pnpm 11.7.0",
		"libgtk-3-dev",
		"libwebkit2gtk-4.1-dev",
		"xvfb",
		"Docker",
		"make desktop-test",
		"make desktop-dev",
		"make desktop-release-check",
		"unsigned development snapshot",
	} {
		if !strings.Contains(developer, fragment) {
			t.Errorf("developer guide does not cover %q", fragment)
		}
	}

	operator := readDocumentation(t, root, "docs/guides/use-remotr-desktop.md")
	for _, fragment := range []string{
		"# Use Remotr Desktop",
		"~/.config/remotr/config.yaml",
		"~/.config/remotr/desktop-profiles.json",
		"operator.crt",
		"operator.key",
		"ca.crt",
		"state.json",
		"absolute Operator state directory",
		"one-time bootstrap token",
		"never stores the token",
		"does not copy credential",
		"does not stage, commit, push, merge, or apply",
		"remotr doctor",
		"remotr endpoint list",
		"remotr git sync",
		"CLI fallback and recovery",
		"Troubleshooting",
	} {
		if !strings.Contains(operator, fragment) {
			t.Errorf("operator guide does not cover %q", fragment)
		}
	}

	reference := readDocumentation(t, root, "docs/reference/remotr-desktop.md")
	for _, fragment := range []string{
		"# Remotr Desktop support reference",
		"Linux only",
		"Linux/amd64",
		"DEB",
		"unsigned",
		"No signed release output is configured",
		"Admin CLI remains supported",
		"desktop-cli-parity.json",
		"parity_claim",
		"make desktop-release-check",
	} {
		if !strings.Contains(reference, fragment) {
			t.Errorf("desktop support reference does not cover %q", fragment)
		}
	}

	inventoryData, err := os.ReadFile(filepath.Join(root, "docs", "reference", "desktop-cli-parity.json"))
	if err != nil {
		t.Fatalf("read desktop parity inventory: %v", err)
	}
	var inventory struct {
		Publication struct {
			ParityClaim string `json:"parity_claim"`
		} `json:"publication"`
		Entries []struct {
			Status string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(inventoryData, &inventory); err != nil {
		t.Fatalf("parse desktop parity inventory: %v", err)
	}
	counts := map[string]int{}
	for _, entry := range inventory.Entries {
		counts[entry.Status]++
	}
	wantStatus := fmt.Sprintf(
		"Current inventory: `%d` implemented, `%d` planned, and `%d` reviewed not applicable; the published parity claim is `%s`.",
		counts["implemented"], counts["planned"], counts["not_applicable"], inventory.Publication.ParityClaim,
	)
	if !strings.Contains(reference, wantStatus) {
		t.Errorf("desktop support reference does not match current parity inventory; want %q", wantStatus)
	}

	mkdocs := readDocumentation(t, root, "mkdocs.yml")
	for _, path := range []string{
		"guides/develop-remotr-desktop.md",
		"guides/use-remotr-desktop.md",
		"reference/remotr-desktop.md",
	} {
		if !strings.Contains(mkdocs, path) {
			t.Errorf("published documentation navigation does not include %s", path)
		}
	}
}

func readDocumentation(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Errorf("read %s: %v", path, err)
		return ""
	}
	return string(data)
}
