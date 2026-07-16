package desktoplayout_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAffectedPullRequestsRunCompleteLinuxDesktopGate(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "desktop.yml"))
	if err != nil {
		t.Fatalf("read desktop pull-request workflow: %v", err)
	}
	workflow := string(data)

	required := []string{
		"name: Remotr Desktop Linux",
		"pull_request:",
		"workflow_dispatch:",
		"runs-on: ubuntu-latest",
		"desktop/**",
		"internal/**",
		"cmd/remotr/**",
		"docs/reference/desktop-cli-parity.json",
		"version: 11.7.0",
		"libgtk-3-dev",
		"libwebkit2gtk-4.1-dev",
		"desktop-file-utils",
		"pnpm install --frozen-lockfile",
		"cd desktop && go test ./...",
		"pnpm typecheck",
		"pnpm lint",
		"pnpm test",
		"TestWailsBindingAllowlist|TestBridgeViewModelsExcludeCredentialCanaries|TestBridgeSecurityRejectsRemoteNavigationAndContent",
		"pnpm exec playwright install --with-deps chromium",
		"pnpm test:browser",
		"pnpm build",
		"TestDesktopCLIParityInventoryMatchesCommandTree",
		"run: make test",
	}
	for _, fragment := range required {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("desktop workflow does not contain required gate fragment %q", fragment)
		}
	}

	for _, unsupported := range []string{"macos-", "windows-", "runs-on: [self-hosted"} {
		if strings.Contains(strings.ToLower(workflow), unsupported) {
			t.Errorf("desktop pull-request workflow contains unsupported runner marker %q", unsupported)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(root, "desktop", "frontend", "package.json"))
	if err != nil {
		t.Fatalf("read desktop frontend package manifest: %v", err)
	}
	for _, script := range []string{`"typecheck": "tsc --noEmit"`, `"lint": "biome lint src visual-tests"`} {
		if !strings.Contains(string(manifest), script) {
			t.Errorf("desktop frontend manifest does not contain CI script %q", script)
		}
	}
	playwrightConfig, err := os.ReadFile(filepath.Join(root, "desktop", "frontend", "playwright.config.ts"))
	if err != nil {
		t.Fatalf("read desktop Playwright configuration: %v", err)
	}
	if strings.Contains(string(playwrightConfig), "/usr/bin/google-chrome") || strings.Contains(string(playwrightConfig), "executablePath") {
		t.Error("desktop visual gate selects a floating host browser instead of Playwright's pinned Chromium")
	}
	if !strings.Contains(string(playwrightConfig), `channel: "chromium"`) {
		t.Error("desktop visual gate does not use Playwright's pinned full Chromium renderer")
	}
	packageManagerConfig, err := os.ReadFile(filepath.Join(root, "desktop", "frontend", "pnpm-workspace.yaml"))
	if err != nil {
		t.Fatalf("read desktop frontend package-manager configuration: %v", err)
	}
	if !strings.Contains(string(packageManagerConfig), "storeDir: .pnpm-store") {
		t.Error("desktop frontend does not keep the pnpm store inside its ignored workspace cache")
	}

	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("read root Makefile: %v", err)
	}
	if !strings.Contains(string(makefile), "test: test-fuzz-seeds") {
		t.Error("root make test gate is no longer retained")
	}
}

func TestDesktopWorkflowsDoNotUseSetupNodeCacheWithRepositoryLocalPnpmStore(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{
		".github/workflows/desktop.yml",
		".github/workflows/release.yml",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read desktop workflow %s: %v", path, err)
		}
		if strings.Contains(string(data), "cache: pnpm") {
			t.Errorf("%s uses setup-node pnpm caching, which probes outside the repository-local pnpm store", path)
		}
	}
}

func TestRootQualityGateInstallsDesktopPackageValidation(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "quality-gate.yml"))
	if err != nil {
		t.Fatalf("read root quality workflow: %v", err)
	}
	if !strings.Contains(string(data), "sudo apt-get install --yes desktop-file-utils") {
		t.Error("root quality workflow does not install desktop-file-validate before running the DEB lifecycle evidence")
	}
}
