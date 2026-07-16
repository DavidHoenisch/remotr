package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DavidHoenisch/remotr/internal/aisetup"
)

func TestAIIntegrationParityConfinesExplicitScopeVersionAndReplacement(t *testing.T) {
	userRoot := t.TempDir()
	t.Setenv("HOME", userRoot)
	projectRoot := filepath.Join(t.TempDir(), "selected-project")
	if err := os.Mkdir(projectRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(projectRoot, ".cursor")); err != nil {
		t.Fatal(err)
	}
	upgradeRoot := filepath.Join(t.TempDir(), "upgrade")
	writeDesktopAISkillFixture(t, upgradeRoot, "2.0.0", "# upgraded skill\n")

	fetchedVersions := []string{}
	service := NewAIIntegrationService(AIIntegrationServiceOptions{
		ChooseProjectRoot: func(context.Context) (string, error) { return projectRoot, nil },
		EmbeddedSource: fstest.MapFS{
			"remotr-agent/SKILL.md": &fstest.MapFile{Data: []byte("# embedded skill\n")},
			"remotr-agent/VERSION":  &fstest.MapFile{Data: []byte("1.0.0\n")},
		},
		EmbeddedRoot:    "remotr-agent",
		EmbeddedVersion: "desktop-test",
		FetchUpgrade: func(_ context.Context, version string) (aisetup.FetchedBundle, error) {
			fetchedVersions = append(fetchedVersions, version)
			return aisetup.FetchedBundle{Dir: upgradeRoot, Tag: version}, nil
		},
		RuntimeLookup: func(name string) (string, error) {
			if name == "cursor" {
				return "", os.ErrNotExist
			}
			return "/usr/bin/" + name, nil
		},
	})
	app := NewApp("test", WithAIIntegrationService(service))

	userIntegrations, err := app.ListAIIntegrations(AIIntegrationListRequest{Scope: "user"})
	if err != nil {
		t.Fatalf("list user AI integrations: %v", err)
	}
	if len(userIntegrations) != 3 || userIntegrations[0].Agent != "claude" || userIntegrations[1].Agent != "cursor" || userIntegrations[2].Agent != "pi" {
		t.Fatalf("user integrations = %#v", userIntegrations)
	}
	cursor := userIntegrations[1]
	if cursor.RuntimeAvailable || cursor.RuntimeStatus != "not_found" || cursor.Guidance == "" {
		t.Fatalf("missing Cursor runtime = %#v", cursor)
	}

	installed, err := app.SetupAIIntegration(AIIntegrationInstallRequest{Agent: "claude", Scope: "user"})
	if err != nil {
		t.Fatalf("setup Claude: %v", err)
	}
	if installed.Status != "installed" || installed.Integration.BundleVersion != "1.0.0" || !installed.Integration.RuntimeAvailable {
		t.Fatalf("installed Claude = %#v", installed)
	}
	claudeSkill := filepath.Join(userRoot, ".claude", "skills", "remotr-agent", "SKILL.md")
	if body, readErr := os.ReadFile(claudeSkill); readErr != nil || string(body) != "# embedded skill\n" {
		t.Fatalf("installed Claude skill = %q, %v", body, readErr)
	}
	if _, err := app.SetupAIIntegration(AIIntegrationInstallRequest{Agent: "claude", Scope: "user"}); err == nil {
		t.Fatal("setup replaced an existing integration without explicit replacement")
	}
	if err := os.WriteFile(claudeSkill, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.SetupAIIntegration(AIIntegrationInstallRequest{Agent: "claude", Scope: "user", Replace: true}); err != nil {
		t.Fatalf("replace Claude: %v", err)
	}
	if body, readErr := os.ReadFile(claudeSkill); readErr != nil || string(body) != "# embedded skill\n" {
		t.Fatalf("replaced Claude skill = %q, %v", body, readErr)
	}

	project, err := app.ChooseAIProjectRoot()
	if err != nil {
		t.Fatalf("choose AI project root: %v", err)
	}
	if project.ID == "" || project.DirectoryName != "selected-project" || strings.Contains(project.ID, projectRoot) {
		t.Fatalf("project root = %#v", project)
	}
	if _, err := app.SetupAIIntegration(AIIntegrationInstallRequest{Agent: "pi", Scope: "project", ProjectRootID: project.ID}); err != nil {
		t.Fatalf("setup project Pi: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".pi", "skills", "remotr-agent", "SKILL.md")); err != nil {
		t.Fatalf("project Pi skill: %v", err)
	}
	if _, err := app.SetupAIIntegration(AIIntegrationInstallRequest{Agent: "cursor", Scope: "project", ProjectRootID: project.ID}); err == nil {
		t.Fatal("project setup followed a symlink outside the selected root")
	}
	if _, err := os.Stat(filepath.Join(outside, "skills", "remotr-agent", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink escape wrote outside project: %v", err)
	}

	missingRuntimeInstall, err := app.SetupAIIntegration(AIIntegrationInstallRequest{Agent: "cursor", Scope: "user"})
	if err != nil {
		t.Fatalf("setup without external runtime: %v", err)
	}
	if missingRuntimeInstall.Integration.RuntimeAvailable || missingRuntimeInstall.Integration.Guidance == "" {
		t.Fatalf("missing-runtime recovery result = %#v", missingRuntimeInstall)
	}

	if _, err := app.UpgradeAIIntegration(AIIntegrationUpgradeRequest{Agent: "claude", Scope: "user", Version: "v2.0.0"}); err == nil {
		t.Fatal("upgrade replaced an existing integration without explicit replacement")
	}
	upgraded, err := app.UpgradeAIIntegration(AIIntegrationUpgradeRequest{Agent: "claude", Scope: "user", Version: "v2.0.0", Replace: true})
	if err != nil {
		t.Fatalf("upgrade Claude: %v", err)
	}
	if upgraded.Status != "upgraded" || upgraded.Integration.BundleVersion != "2.0.0" || !slices.Equal(fetchedVersions, []string{"v2.0.0"}) {
		t.Fatalf("upgraded Claude = %#v; fetched = %v", upgraded, fetchedVersions)
	}
	if body, readErr := os.ReadFile(claudeSkill); readErr != nil || string(body) != "# upgraded skill\n" {
		t.Fatalf("upgraded Claude skill = %q, %v", body, readErr)
	}
	if _, err := app.UpgradeAIIntegration(AIIntegrationUpgradeRequest{Agent: "pi", Scope: "user", Version: "../escape", Replace: true}); err == nil {
		t.Fatal("unsafe upgrade version was accepted")
	}
	if !slices.Equal(fetchedVersions, []string{"v2.0.0"}) {
		t.Fatalf("unsafe version reached fetch boundary: %v", fetchedVersions)
	}

	encoded, err := json.Marshal([]any{project, userIntegrations, installed, missingRuntimeInstall, upgraded})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{userRoot, projectRoot, outside, upgradeRoot} {
		if bytes.Contains(encoded, []byte(path)) {
			t.Fatalf("AI view models exposed native path %q: %s", path, encoded)
		}
	}
}

func writeDesktopAISkillFixture(t *testing.T, root, version, skill string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"SKILL.md": skill, "VERSION": version + "\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}
