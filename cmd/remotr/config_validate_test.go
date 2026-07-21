package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApp_configValidateApplicationRendersAgentParsableArtifact(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "applications", "cherrypick.yaml"), `kind: application
name: io.github.ellie_commons.cherrypick
present: true
packageManager: flatpak
`)
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "workstations", "manifest.yaml"), `kind: manifest
applications:
  - applications/cherrypick.yaml
`)

	if err := newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir}); err != nil {
		t.Fatalf("config validate: %v", err)
	}

	rendered := captureStdout(t, func() {
		if err := newApp().Run(context.Background(), []string{"remotr", "config", "render", "--fleet", "workstations", dir}); err != nil {
			t.Fatalf("config render: %v", err)
		}
	})
	state, err := models.ParseState(bytes.NewBufferString(rendered))
	if err != nil {
		t.Fatalf("agent parse of artifact approved by config validate: %v\n%s", err, rendered)
	}
	if got := state.Configurations[0].Packages[0].Lifecycle; got != models.LifecyclePresent {
		t.Fatalf("package lifecycle = %q, want %q", got, models.LifecyclePresent)
	}
}

func TestApp_configValidateRejectsArtifactRejectedByAgentParser(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "applications", "cherrypick.yaml"), `kind: application
name: io.github.ellie_commons.cherrypick
lifecycle: disabled
packageManager: flatpak
`)
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "workstations", "manifest.yaml"), `kind: manifest
applications:
  - applications/cherrypick.yaml
`)

	err := newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir})
	if err == nil || !strings.Contains(err.Error(), "config validate: 1 issue(s)") {
		t.Fatalf("config validate error = %v, want rendered artifact rejection", err)
	}
}

func TestApp_configValidateAcceptsTypedUbuntuProResource(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "modules", "ubuntu-pro.yaml"), `kind: module
schemaVersion: 1
configurations:
  - name: ubuntu-pro
    targetDistros: [Ubuntu]
    targetArch: [X86]
    resources:
      - kind: ubuntuPro
        name: primary-subscription
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/production@active
        services:
          - {name: esm-infra, state: enabled}
          - {name: livepatch, state: disabled, disableMode: retain-packages}
        policy: report
        authorizationGroup: ubuntu-pro-production
        enforce: false
`)
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "workstations", "manifest.yaml"), `kind: manifest
modules: [modules/ubuntu-pro.yaml]
`)

	if err := newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir}); err != nil {
		t.Fatalf("config validate: %v", err)
	}
	rendered := captureStdout(t, func() {
		if err := newApp().Run(context.Background(), []string{"remotr", "config", "render", "--fleet", "workstations", dir}); err != nil {
			t.Fatalf("config render: %v", err)
		}
	})
	state, err := models.ParseState(bytes.NewBufferString(rendered))
	if err != nil {
		t.Fatalf("ParseState(rendered) error = %v\n%s", err, rendered)
	}
	resource := state.Configurations[0].UbuntuPro[0]
	if resource.TokenRef != "remotr:ubuntu-pro/production@active" || resource.Lifecycle != models.UbuntuProAttached || len(resource.Services) != 2 {
		t.Fatalf("rendered Ubuntu Pro resource = %#v", resource)
	}
}

func writeConfigTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
