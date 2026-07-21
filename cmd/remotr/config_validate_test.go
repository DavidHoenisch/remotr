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

func TestApp_configValidateAcceptsCompleteUbuntuProCatalogAndLandscape(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "modules", "ubuntu-pro.yaml"), `kind: module
schemaVersion: 1
configurations:
  - name: ubuntu-pro-catalog
    targetDistros: [Ubuntu]
    targetArch: [X86]
    resources:
      - kind: ubuntuPro
        name: complete-catalog
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/production@7
        services:
          - {name: esm-infra, state: enabled, enableMode: full}
          - {name: esm-apps, state: enabled, enableMode: access-only}
          - {name: livepatch, state: enabled}
          - {name: usg, state: enabled}
          - {name: fips, state: disabled, disableMode: retain-packages}
          - {name: fips-updates, state: disabled, disableMode: purge}
          - {name: realtime-kernel, state: disabled, variant: intel-iotg}
          - {name: ros, state: enabled}
          - {name: ros-updates, state: enabled}
          - {name: anbox-cloud, state: enabled}
        landscape:
          state: enrolled
          accountName: production
          computerTitle: host-from-endpoint
          serverURL: https://landscape.example.test/message-system
          pingURL: https://landscape.example.test/ping
          tags: [production, linux]
          accessGroup: servers
          registrationKeyRef: remotr:landscape/registration@active
          caRef: remotr:landscape/ca@3
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
}

func TestApp_configValidateRejectsUnsafeUbuntuProAuthoring(t *testing.T) {
	tests := map[string]struct {
		resource string
		want     string
	}{
		"attached token missing": {
			resource: "lifecycle: attached",
			want:     "tokenRef",
		},
		"detached carries token": {
			resource: "lifecycle: detached\n        tokenRef: remotr:ubuntu-pro/production@active",
			want:     "detached ubuntuPro resource forbids",
		},
		"duplicate service": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        services: [{name: esm-infra, state: enabled}, {name: esm-infra, state: disabled}]",
			want:     "duplicate service",
		},
		"unknown beta service": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        services: [{name: beta-magic, state: enabled}]",
			want:     "stable service catalog",
		},
		"historical service": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        services: [{name: cis, state: enabled}]",
			want:     "historical service name",
		},
		"raw arguments": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        args: [enable, esm-infra]",
			want:     "field args not found",
		},
		"generic provider options": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        providerOptions: {pro: {args: [enable]}}",
			want:     "does not accept provider options",
		},
		"client setting": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        proxy: https://proxy.example.test",
			want:     "field proxy not found",
		},
		"maintenance event": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        fix: CVE-2099-0001",
			want:     "field fix not found",
		},
		"enable option on disabled service": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        services: [{name: esm-infra, state: disabled, enableMode: access-only}]",
			want:     "enableMode requires state enabled",
		},
		"disable option on enabled service": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        services: [{name: esm-infra, state: enabled, disableMode: purge}]",
			want:     "disableMode requires state disabled",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			output, err := validateUbuntuProResource(t, test.resource)
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("config validate error = %v, output = %q, want %q", err, output, test.want)
			}
		})
	}
}

func validateUbuntuProResource(t *testing.T, fields string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	fields = strings.ReplaceAll(fields, "\n        ", "\n")
	writeConfigTestFile(t, filepath.Join(dir, "modules", "ubuntu-pro.yaml"), "kind: module\nschemaVersion: 1\nconfigurations:\n  - name: ubuntu-pro\n    resources:\n      - kind: ubuntuPro\n        name: subscription\n        "+strings.ReplaceAll(fields, "\n", "\n        ")+"\n")
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "workstations", "manifest.yaml"), "kind: manifest\nmodules: [modules/ubuntu-pro.yaml]\n")
	var runErr error
	output := captureStdout(t, func() {
		runErr = newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir})
	})
	return output, runErr
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
