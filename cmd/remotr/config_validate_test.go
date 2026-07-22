package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
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

// OS-FOM-014. Public seam: remotr config validate. Legacy reloadExec input
// must be representable by the shared activation queue before release.
func TestApp_configValidateRejectsUnrepresentableDownloadReloadExec(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "modules", "audit.yaml"), `kind: module
schemaVersion: 1
configurations:
  - name: auditd-rules
    resources:
      - kind: download
        name: audit-rules
        url: https://example.test/audit.rules
        dest: /etc/audit/rules.d/audit.rules
        reloadExec: [augenrules, --load]
`)
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), `kind: manifest
modules: [modules/audit.yaml]
`)

	output := captureStdout(t, func() {
		err := newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir})
		if err == nil || !strings.Contains(err.Error(), "config validate:") {
			t.Fatalf("config validate error = %v, want reloadExec rejection", err)
		}
	})
	if !strings.Contains(output, "reloadExec supports only systemctl daemon-reload, reload, try-restart, or restart") {
		t.Fatalf("config validate output = %q, want actionable reloadExec diagnostic", output)
	}
}

// OS-AEC-104 and OS-PRM-030. Public seam: remotr config validate. The same
// provider-release ambiguity rejected during server composition must fail
// locally before an operator attempts Git sync.
func TestApp_configValidateRejectsProviderTargetWithoutArchitecture(t *testing.T) {
	dir := t.TempDir()
	writeConfigTestFile(t, filepath.Join(dir, "modules", "apt.yaml"), `kind: module
schemaVersion: 1
configurations:
  - name: apt-applications
    targetDistros: [Debian]
    resources:
      - kind: package
        name: curl
        lifecycle: present
        packageManager: apt
`)
	writeConfigTestFile(t, filepath.Join(dir, "fleets", "engineering", "manifest.yaml"), `kind: manifest
modules: [modules/apt.yaml]
`)

	output := captureStdout(t, func() {
		err := newApp().Run(context.Background(), []string{"remotr", "config", "validate", dir})
		if err == nil || !strings.Contains(err.Error(), "config validate: 1 issue(s)") {
			t.Fatalf("config validate error = %v, want provider release rejection", err)
		}
	})
	if !strings.Contains(output, "provider_release_target_arch") || !strings.Contains(output, "apt-applications") {
		t.Fatalf("config validate output = %q, want stable diagnostic identity and configuration", output)
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

func TestApp_configValidateAcceptsCompleteUbuntuProCatalog(t *testing.T) {
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

// OS-UPM-001, OS-UPM-034 through OS-UPM-036, and OS-UPM-043 through
// OS-UPM-045: the checked-in Ubuntu Pro repository remains source-only and is
// consumed through the public operator configuration workflow.
func TestApp_configUbuntuProRepositoryWorkflow(t *testing.T) {
	repo := filepath.Join("..", "..", "test", "config-repos", "ubuntu-pro-management")
	if info, err := os.Stat(repo); err != nil || !info.IsDir() {
		t.Fatalf("checked-in Ubuntu Pro repository is unavailable: %v", err)
	}

	discovered := captureStdout(t, func() {
		if err := newApp().Run(context.Background(), []string{"remotr", "config", "discover", "--fleet", "ubuntu-pro", repo}); err != nil {
			t.Fatalf("config discover: %v", err)
		}
	})
	validated := captureStdout(t, func() {
		if err := newApp().Run(context.Background(), []string{"remotr", "config", "validate", repo}); err != nil {
			t.Fatalf("config validate: %v", err)
		}
	})
	render := func() string {
		return captureStdout(t, func() {
			if err := newApp().Run(context.Background(), []string{"remotr", "config", "render", "--fleet", "ubuntu-pro", repo}); err != nil {
				t.Fatalf("config render: %v", err)
			}
		})
	}
	first, second := render(), render()
	if first != second {
		t.Fatal("repeated Ubuntu Pro render is not deterministic")
	}
	if !strings.Contains(validated, "config validate: ok") {
		t.Fatalf("validation output = %q", validated)
	}

	wantRequirements := []string{
		"provider:ubuntu-pro-option/esm-apps/full",
		"provider:ubuntu-pro-service/esm-apps",
		"resource:ubuntu-pro",
		"schema:1",
	}
	requirementSection := strings.Split(discovered, "Capability requirements:\n")
	if len(requirementSection) != 2 {
		t.Fatalf("config discover omitted capability requirement section:\n%s", discovered)
	}
	var gotRequirements []string
	for _, line := range strings.Split(strings.TrimSpace(requirementSection[1]), "\n") {
		gotRequirements = append(gotRequirements, strings.TrimPrefix(strings.TrimSpace(line), "- "))
	}
	if !slices.Equal(gotRequirements, wantRequirements) {
		t.Errorf("config discover requirements = %#v, want %#v", gotRequirements, wantRequirements)
	}

	state, err := models.ParseState(bytes.NewBufferString(first))
	if err != nil {
		t.Fatalf("ParseState(rendered) error = %v\n%s", err, first)
	}
	address := models.ResourceAddress("ubuntu-pro", "primary-subscription")
	if len(state.Configurations) != 1 || len(state.Configurations[0].UbuntuPro) != 1 {
		t.Fatalf("rendered state does not preserve one Ubuntu Pro resource: %#v", state.Configurations)
	}
	if _, ok := state.ResourceSources[address]; !ok {
		t.Fatalf("rendered state omitted stable resource address %q: %#v", address, state.ResourceSources)
	}

	if err := filepath.WalkDir(repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (entry.Name() == "desired.yaml" || entry.Name() == "crons.yaml") {
			t.Errorf("source-only repository contains generated artifact %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
		"removed Landscape service": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        services: [{name: landscape, state: enabled}]",
			want:     "stable service catalog",
		},
		"removed Landscape block": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        landscape: {state: enrolled, accountName: production, computerTitle: host}",
			want:     "field landscape not found",
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
			want:     "field \"proxy\" is outside subscription and service lifecycle management; use a separate typed capability",
		},
		"maintenance event": {
			resource: "lifecycle: attached\n        tokenRef: remotr:ubuntu-pro/production@active\n        fix: CVE-2099-0001",
			want:     "field \"fix\" is outside subscription and service lifecycle management; use a separate typed capability",
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
