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
)

func TestConfigRepositoryParityConfinesLocalSharedPackageWorkflows(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "selected-config")
	writeDesktopConfigFixture(t, repoRoot)
	gitIndex := filepath.Join(repoRoot, ".git", "index")
	if err := os.MkdirAll(filepath.Dir(gitIndex), 0o750); err != nil {
		t.Fatal(err)
	}
	const gitCanary = "git-index-canary-must-remain-unchanged"
	if err := os.WriteFile(gitIndex, []byte(gitCanary), 0o600); err != nil {
		t.Fatal(err)
	}

	hubRoot := filepath.Join(t.TempDir(), "hub")
	writeDesktopHubFixture(t, hubRoot)
	newRepo := filepath.Join(t.TempDir(), "new-config")
	if err := os.Mkdir(newRepo, 0o750); err != nil {
		t.Fatal(err)
	}
	savedRender := filepath.Join(t.TempDir(), "production-desired.yaml")

	service := NewConfigRepositoryService(ConfigRepositoryServiceOptions{
		ChooseExisting:   func(context.Context) (string, error) { return repoRoot, nil },
		ChooseInitialize: func(context.Context) (string, error) { return newRepo, nil },
		ChooseRenderSave: func(context.Context, string) (string, error) { return savedRender, nil },
		HubRoot:          hubRoot,
	})
	app := NewApp("test", WithConfigRepositoryService(service))

	workingTree, err := app.ChooseConfigRepository()
	if err != nil {
		t.Fatalf("choose config repository: %v", err)
	}
	if workingTree.ID == "" || workingTree.DirectoryName != "selected-config" || strings.Contains(workingTree.ID, repoRoot) {
		t.Fatalf("working tree = %#v", workingTree)
	}

	validation, err := app.ValidateConfigRepository(workingTree.ID)
	if err != nil {
		t.Fatalf("validate config repository: %v", err)
	}
	if !validation.Valid || len(validation.Issues) != 0 || !slices.Contains(validation.OK, "fleets/production/manifest.yaml") {
		t.Fatalf("validation = %#v", validation)
	}

	discovery, err := app.DiscoverConfigFleet(ConfigFleetDiscoverRequest{WorkingTreeID: workingTree.ID, Fleet: "production"})
	if err != nil {
		t.Fatalf("discover config fleet: %v", err)
	}
	if discovery.Manifest != "fleets/production/manifest.yaml" || !slices.Equal(discovery.Modules, []string{"modules/base.yaml"}) || !slices.Contains(discovery.ResourceKinds, "package") {
		t.Fatalf("discovery = %#v", discovery)
	}

	rendered, err := app.RenderConfigRepository(ConfigRenderRequest{WorkingTreeID: workingTree.ID, Scope: "fleet", TargetID: "production"})
	if err != nil {
		t.Fatalf("render config repository: %v", err)
	}
	if len(rendered.Artifacts) != 1 || rendered.Artifacts[0].TargetType != "fleet" || rendered.Artifacts[0].TargetID != "production" || rendered.Artifacts[0].ArtifactType != "desired" || !strings.Contains(rendered.Artifacts[0].Content, "name: curl") || len(rendered.Artifacts[0].Digest) != 64 {
		t.Fatalf("rendered = %#v", rendered)
	}
	saved, err := app.SaveConfigRender(ConfigRenderSaveRequest{
		WorkingTreeID: workingTree.ID,
		TargetType:    rendered.Artifacts[0].TargetType,
		TargetID:      rendered.Artifacts[0].TargetID,
		ArtifactType:  rendered.Artifacts[0].ArtifactType,
		Digest:        rendered.Artifacts[0].Digest,
	})
	if err != nil {
		t.Fatalf("save config render: %v", err)
	}
	if saved.Status != "saved" || saved.FileName != filepath.Base(savedRender) {
		t.Fatalf("save result = %#v", saved)
	}
	savedBody, err := os.ReadFile(savedRender)
	if err != nil {
		t.Fatal(err)
	}
	if string(savedBody) != rendered.Artifacts[0].Content {
		t.Fatal("saved render differs from reviewed artifact")
	}

	hubEntries, err := app.ListConfigHubSnippets(workingTree.ID)
	if err != nil {
		t.Fatalf("list Hub snippets: %v", err)
	}
	if len(hubEntries) != 1 || hubEntries[0].ID != "ssh-hardening" || hubEntries[0].Title != "SSH hardening" {
		t.Fatalf("Hub entries = %#v", hubEntries)
	}
	imported, err := app.ImportConfigHubSnippet(ConfigHubImportRequest{WorkingTreeID: workingTree.ID, EntryID: "ssh-hardening", OutPath: "modules/ssh-hardening.yaml"})
	if err != nil {
		t.Fatalf("import Hub snippet: %v", err)
	}
	if imported.Status != "imported" || imported.OutPath != "modules/ssh-hardening.yaml" {
		t.Fatalf("import result = %#v", imported)
	}
	importedBody, err := os.ReadFile(filepath.Join(repoRoot, "modules", "ssh-hardening.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(importedBody, []byte("name: sshd")) {
		t.Fatalf("imported snippet = %s", importedBody)
	}

	escapePath := filepath.Join(filepath.Dir(repoRoot), "escaped.yaml")
	if _, err := app.ImportConfigHubSnippet(ConfigHubImportRequest{WorkingTreeID: workingTree.ID, EntryID: "ssh-hardening", OutPath: "../escaped.yaml"}); err == nil {
		t.Fatal("traversal import succeeded")
	}
	if _, err := os.Stat(escapePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("traversal output exists: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repoRoot, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ImportConfigHubSnippet(ConfigHubImportRequest{WorkingTreeID: workingTree.ID, EntryID: "ssh-hardening", OutPath: "linked/escaped.yaml"}); err == nil {
		t.Fatal("symlink escape import succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink escape output exists: %v", err)
	}

	initialized, err := app.InitializeConfigRepository(ConfigRepositoryInitRequest{Fleet: "lab", RemediationPolicy: "report"})
	if err != nil {
		t.Fatalf("initialize config repository: %v", err)
	}
	if initialized.Status != "initialized" || initialized.Fleet != "lab" || initialized.WorkingTree.DirectoryName != "new-config" {
		t.Fatalf("initialized = %#v", initialized)
	}
	for _, relative := range []string{"remotr.yaml", "modules/base-packages.yaml", "fleets/lab/manifest.yaml"} {
		if _, err := os.Stat(filepath.Join(newRepo, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("initialized %s: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(newRepo, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("initializer created Git state: %v", err)
	}

	gotGitIndex, err := os.ReadFile(gitIndex)
	if err != nil || string(gotGitIndex) != gitCanary {
		t.Fatalf("Git index changed: %q, %v", gotGitIndex, err)
	}
	encoded, err := json.Marshal([]any{workingTree, validation, discovery, rendered, saved, hubEntries, imported, initialized})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{repoRoot, hubRoot, outside} {
		if bytes.Contains(encoded, []byte(path)) {
			t.Fatalf("view models exposed native root %q: %s", path, encoded)
		}
	}
}

func writeDesktopConfigFixture(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"remotr.yaml": "kind: remotr-config-repo\nversion: 1\ndefaultFleet: production\n",
		"modules/base.yaml": `kind: module
schemaVersion: 1
configurations:
  - name: base-packages
    targetDistros: [Debian]
    resources:
      - kind: package
        name: curl
        lifecycle: present
`,
		"fleets/production/manifest.yaml": "kind: manifest\nmodules:\n  - modules/base.yaml\n",
	}
	for relative, body := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func writeDesktopHubFixture(t *testing.T, root string) {
	t.Helper()
	catalog := `{"categories":[{"id":"security","label":"Security","description":"Security modules"}],"entries":[{"id":"ssh-hardening","title":"SSH hardening","description":"Harden SSH defaults","category":"security","tags":["ssh"],"distros":["Debian"],"author":"Remotr","sourceUrl":"","sourceCommit":"","snippetPath":"snippets/ssh-hardening.yaml","featured":true}]}`
	files := map[string]string{
		"data/catalog.json": catalog,
		"snippets/ssh-hardening.yaml": `kind: module
schemaVersion: 1
configurations:
  - name: sshd
    targetDistros: [Debian]
    resources:
      - kind: package
        name: openssh-server
        lifecycle: present
`,
	}
	for relative, body := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}
