package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestConfigurationAuthoringFeature(t *testing.T) {
	feature, err := os.ReadFile("features/config_authoring.feature")
	if err != nil {
		t.Fatal(err)
	}
	state := &configAuthoringState{}
	status := Run(t, []godog.Feature{{Name: "config_authoring.feature", Contents: feature}}, func(ctx *godog.ScenarioContext) {
		ctx.Before(func(context.Context, *godog.Scenario) (context.Context, error) {
			*state = configAuthoringState{}
			return context.Background(), nil
		})
		ctx.Step(`^an invalid configuration repository$`, state.invalidRepository)
		ctx.Step(`^the Compose configuration repository$`, func() error { state.repo = filepath.Join(repositoryRoot(), "compose", "config-repo"); return nil })
		ctx.Step(`^a canonical configuration with an unknown resource field$`, state.canonicalUnknownFieldRepository)
		ctx.Step(`^a canonical configuration with a cross-kind duplicate name$`, state.canonicalCrossKindDuplicateRepository)
		ctx.Step(`^a canonical configuration selecting deferred DNF$`, state.canonicalDNFRepository)
		ctx.Step(`^a legacy configuration repository$`, state.legacyRepository)
		ctx.Step(`^a canonical M1 applicator repository$`, state.canonicalM1Repository)
		ctx.Step(`^a canonical M3 host-baseline repository$`, state.canonicalM3Repository)
		ctx.Step(`^a cron endpoint schedule with a systemd-only field$`, state.invalidCronEndpointScheduleRepository)
		ctx.Step(`^a canonical cron endpoint schedule repository$`, state.canonicalCronEndpointScheduleRepository)
		ctx.Step(`^a canonical systemd timer schedule repository$`, state.canonicalSystemdTimerScheduleRepository)
		ctx.Step(`^a canonical provider-neutral systemd service repository$`, state.canonicalServiceRepository)
		ctx.Step(`^an OpenRC service requesting masked state$`, state.unsupportedOpenRCMaskRepository)
		ctx.Step(`^a canonical user resource with an invalid shell field$`, state.unsupportedUserRepository)
		ctx.Step(`^the operator validates the repository$`, state.validate)
		ctx.Step(`^validation is rejected$`, func() error {
			if state.err == nil {
				return os.ErrInvalid
			}
			return nil
		})
		ctx.Step(`^validation is accepted$`, func() error {
			if state.err != nil {
				return fmt.Errorf("validation failed: %w: %s", state.err, state.output)
			}
			return nil
		})
		ctx.Step(`^rendering preserves every advertised M1 field$`, state.renderPreservesM1Fields)
		ctx.Step(`^rendering preserves every advertised M3 field$`, state.renderPreservesM3Fields)
		ctx.Step(`^rendering preserves every advertised cron schedule field$`, state.renderPreservesCronScheduleFields)
		ctx.Step(`^rendering preserves every advertised systemd timer field$`, state.renderPreservesSystemdTimerFields)
		ctx.Step(`^rendering preserves every advertised service field$`, state.renderPreservesServiceFields)
		ctx.Step(`^the operator renders fleet "([^"]*)" twice$`, state.renderTwice)
		ctx.Step(`^both rendered artifacts are identical$`, func() error {
			if state.err != nil || state.first != state.second {
				return os.ErrInvalid
			}
			return nil
		})
		ctx.Step(`^validation identifies resource "([^"]*)" and field "([^"]*)"$`, state.validationIdentifies)
		ctx.Step(`^validation rejects ambiguous resource "([^"]*)"$`, state.validationRejectsAmbiguousResource)
		ctx.Step(`^validation reports the RPM-family roadmap for resource "([^"]*)"$`, state.validationReportsRPMRoadmap)
		ctx.Step(`^the operator discovers validates and renders fleet "([^"]*)"$`, state.discoverValidateRender)
		ctx.Step(`^tooling reports resource kind "([^"]*)" and capability "([^"]*)"$`, state.toolingReportsResourceCapability)
		ctx.Step(`^validation emits the schema zero deprecation diagnostic$`, state.validationEmitsLegacyDiagnostic)
		ctx.Step(`^no composed artifacts are written to the source repository$`, state.noComposedArtifactsWritten)
	})
	if status != 0 {
		t.Fatalf("acceptance status = %d", status)
	}
}

type configAuthoringState struct {
	repo, first, second string
	output              string
	discoverOutput      string
	validateOutput      string
	renderOutput        string
	err                 error
}

func (s *configAuthoringState) invalidRepository() error {
	dir, err := os.MkdirTemp("", "remotr-invalid-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	return os.WriteFile(filepath.Join(dir, "invalid.yaml"), []byte("kind: invalid\n"), 0o600)
}
func (s *configAuthoringState) validate() error {
	s.output, s.err = runRemotr("config", "validate", s.repo)
	return nil
}

func (s *configAuthoringState) canonicalUnknownFieldRepository() error {
	dir, err := os.MkdirTemp("", "remotr-canonical-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	module := `kind: module
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: curl
        lifecycle: present
        present: true
        presnt: false
`
	manifest := "kind: manifest\nmodules:\n  - modules/base.yaml\n"
	for path, content := range map[string]string{
		filepath.Join(dir, "modules", "base.yaml"):                  module,
		filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): manifest,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) validationIdentifies(address, field string) error {
	if s.err == nil || !strings.Contains(s.output, address) || !strings.Contains(s.output, field) {
		return fmt.Errorf("validation output %q, error %v; want address %q and field %q", s.output, s.err, address, field)
	}
	return nil
}

func (s *configAuthoringState) canonicalCrossKindDuplicateRepository() error {
	dir, err := os.MkdirTemp("", "remotr-duplicate-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	module := `kind: module
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: shared
        lifecycle: present
        present: true
      - kind: file
        name: shared
        path: /tmp/shared
        content: managed
`
	manifest := "kind: manifest\nmodules:\n  - modules/base.yaml\n"
	for path, content := range map[string]string{
		filepath.Join(dir, "modules", "base.yaml"):                  module,
		filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): manifest,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) validationRejectsAmbiguousResource(address string) error {
	if s.err == nil || !strings.Contains(s.output, address) || !strings.Contains(s.output, "duplicate") {
		return fmt.Errorf("validation output %q, error %v; want duplicate resource %q", s.output, s.err, address)
	}
	return nil
}

func (s *configAuthoringState) canonicalDNFRepository() error {
	dir, err := os.MkdirTemp("", "remotr-dnf-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	module := `kind: module
schemaVersion: 1
configurations:
  - name: base
    targetDistros: [Debian]
    resources:
      - kind: package
        name: curl
        lifecycle: present
        present: true
        packageManager: dnf
`
	manifest := "kind: manifest\nmodules:\n  - modules/base.yaml\n"
	for path, content := range map[string]string{
		filepath.Join(dir, "modules", "base.yaml"):                  module,
		filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): manifest,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) canonicalM1Repository() error {
	module := `kind: module
schemaVersion: 1
configurations:
- name: base
  targetDistros: [Debian]
  resources:
  - kind: package
    name: curl
    lifecycle: present
    packageManager: apt
    version: "1.0"
    allowUpgrade: true
    allowDowngrade: false
    hold: false
    refreshCache: true
    removeDependencies: false
    nonInteractive: true
  - kind: file
    name: motd
    lifecycle: present
    path: /tmp/remotr-motd
    content: managed
    mode: [420]
    owner: root
    group: root
  - kind: download
    name: helper
    lifecycle: present
    url: https://example.com/helper
    dest: /tmp/remotr-helper
    checksum: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    signature: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
    trustedSigner: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
    authenticationRef: secrets/helper-token
    redirectPolicy: same-origin
    timeout: 30s
    mode: [493]
    owner: root
    group: root
    notifications: [{type: restart, target: helper.service}]
  - kind: user
    name: alice
    username: alice
    present: true
    uid: 2000
    allowUIDReassignment: true
  - kind: firewall
    name: allow-web
    lifecycle: present
    audit: true
    action: allow
    protocol: tcp
    ports: [443]
    sources: [10.0.0.0/8]
    backend: nftables
    table: filter
    chain: input
    family: inet
    protectRemotr: true
`
	return s.writeRepository("remotr-m1-config-", module)
}

func (s *configAuthoringState) canonicalM3Repository() error {
	module := `kind: module
schemaVersion: 1
configurations:
- name: base
  targetDistros: [Debian, Ubuntu, Arch]
  resources:
  - kind: kernelModule
    name: loop
    module: loop
    loaded: true
    persistent: true
  - kind: hostname
    name: endpoint-name
    static: remotr-endpoint
  - kind: hostLocale
    name: locale
    timezone: UTC
  - kind: timeSync
    name: ntp
    provider: systemd-timesyncd
    enabled: true
  - kind: mount
    name: cache
    source: tmpfs
    target: /var/cache/remotr
    filesystemType: tmpfs
    options: [mode=0755]
    mounted: true
    persistent: true
  - kind: swap
    name: page
    path: /var/lib/remotr/swapfile
    type: file
    sizeBytes: 67108864
    priority: 5
    active: true
    persistent: true
`
	return s.writeRepository("remotr-m3-config-", module)
}

func (s *configAuthoringState) unsupportedUserRepository() error {
	return s.writeRepository("remotr-user-config-", "kind: module\nschemaVersion: 1\nconfigurations:\n- name: base\n  resources:\n  - kind: user\n    name: alice\n    username: alice\n    present: true\n    shell: bash\n")
}

func (s *configAuthoringState) invalidCronEndpointScheduleRepository() error {
	module := `kind: module
schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: endpointSchedule
    name: nightly-backup
    lifecycle: present
    backend: cron
    schedule: "0 3 * * *"
    user: root
    argv: [/usr/local/bin/backup, "daily archive"]
    persistent: true
`
	return s.writeRepository("remotr-endpoint-schedule-config-", module)
}

func (s *configAuthoringState) canonicalCronEndpointScheduleRepository() error {
	module := `kind: module
schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: endpointSchedule
    name: nightly-backup
    lifecycle: present
    backend: cron
    schedule: "0 3 * * *"
    user: root
    argv: [/usr/local/bin/backup, "daily archive"]
    workingDirectory: /var/lib/backup
    timeout: 30m
    overlap: forbid
`
	return s.writeRepository("remotr-cron-schedule-config-", module)
}

func (s *configAuthoringState) canonicalSystemdTimerScheduleRepository() error {
	module := `kind: module
schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: endpointSchedule
    name: inventory
    lifecycle: present
    backend: systemd-timer
    schedule: "*-*-* 03:00:00"
    user: root
    argv: [/usr/local/bin/inventory, --upload]
    persistent: true
    overlap: forbid
`
	return s.writeRepository("remotr-systemd-timer-config-", module)
}

func (s *configAuthoringState) canonicalServiceRepository() error {
	return s.writeRepository("remotr-service-config-", `kind: module
schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: service
    name: ssh
    provider: systemd
    scope: system
    service: ssh.service
    enabled: true
    active: true
    masked: false
  - kind: service
    name: desktop-agent
    provider: systemd
    scope: user
    service: desktop-agent.service
    users: interactive
    linger: true
    enabled: true
    active: true
`)
}

func (s *configAuthoringState) unsupportedOpenRCMaskRepository() error {
	return s.writeRepository("remotr-openrc-service-config-", `kind: module
schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: service
    name: ssh
    provider: openrc
    scope: system
    service: sshd
    masked: true
`)
}

func (s *configAuthoringState) writeRepository(prefix, module string) error {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return err
	}
	s.repo = dir
	for path, content := range map[string]string{filepath.Join(dir, "modules", "base.yaml"): module, filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): "kind: manifest\nmodules:\n- modules/base.yaml\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) renderPreservesM1Fields() error {
	rendered, err := runRemotr("config", "render", "--fleet", "test-fleet", s.repo)
	if err != nil {
		return fmt.Errorf("render M1: %w: %s", err, rendered)
	}
	for _, field := range []string{"allowDowngrade:", "owner: root", "trustedSigner:", "authenticationRef:", "allowUIDReassignment:", "protectRemotr:"} {
		if !strings.Contains(rendered, field) {
			return fmt.Errorf("rendered M1 artifact omitted %q: %s", field, rendered)
		}
	}
	return nil
}

func (s *configAuthoringState) renderPreservesM3Fields() error {
	rendered, err := runRemotr("config", "render", "--fleet", "test-fleet", s.repo)
	if err != nil {
		return fmt.Errorf("render M3: %w: %s", err, rendered)
	}
	for _, field := range []string{"kernelModule", "hostLocale", "timeSync", "filesystemType:", "sizeBytes:", "priority:"} {
		if !strings.Contains(rendered, field) {
			return fmt.Errorf("rendered M3 artifact omitted %q: %s", field, rendered)
		}
	}
	if strings.Contains(rendered, "kind: command") {
		return fmt.Errorf("M3 artifact unexpectedly uses generic command: %s", rendered)
	}
	return nil
}

func (s *configAuthoringState) renderPreservesCronScheduleFields() error {
	rendered, err := runRemotr("config", "render", "--fleet", "test-fleet", s.repo)
	if err != nil {
		return fmt.Errorf("render endpoint schedule: %w: %s", err, rendered)
	}
	for _, field := range []string{"kind: endpointSchedule", "backend: cron", "workingDirectory:", "timeout: 30m", "overlap: forbid", "daily archive"} {
		if !strings.Contains(rendered, field) {
			return fmt.Errorf("rendered endpoint schedule omitted %q: %s", field, rendered)
		}
	}
	return nil
}

func (s *configAuthoringState) renderPreservesSystemdTimerFields() error {
	rendered, err := runRemotr("config", "render", "--fleet", "test-fleet", s.repo)
	if err != nil {
		return fmt.Errorf("render systemd timer: %w: %s", err, rendered)
	}
	for _, field := range []string{"kind: endpointSchedule", "backend: systemd-timer", "persistent: true", "overlap: forbid", "*-*-* 03:00:00"} {
		if !strings.Contains(rendered, field) {
			return fmt.Errorf("rendered systemd timer omitted %q: %s", field, rendered)
		}
	}
	return nil
}

func (s *configAuthoringState) renderPreservesServiceFields() error {
	rendered, err := runRemotr("config", "render", "--fleet", "test-fleet", s.repo)
	if err != nil {
		return fmt.Errorf("render service: %w: %s", err, rendered)
	}
	for _, field := range []string{"kind: service", "provider: systemd", "scope: system", "scope: user", "service: ssh.service", "users: interactive", "linger: true", "masked: false"} {
		if !strings.Contains(rendered, field) {
			return fmt.Errorf("rendered service omitted %q: %s", field, rendered)
		}
	}
	return nil
}

func (s *configAuthoringState) validationReportsRPMRoadmap(address string) error {
	if s.err == nil || !strings.Contains(s.output, address) || !strings.Contains(s.output, "RPM-family roadmap") {
		return fmt.Errorf("validation output %q, error %v; want RPM roadmap for %q", s.output, s.err, address)
	}
	return nil
}

func (s *configAuthoringState) legacyRepository() error {
	dir, err := os.MkdirTemp("", "remotr-legacy-config-")
	if err != nil {
		return err
	}
	s.repo = dir
	module := `kind: module
configurations:
  - name: base
    targetDistros: [Debian]
    packages:
      - name: curl
        present: true
        packageManager: apt
`
	manifest := "kind: manifest\nmodules:\n  - modules/base.yaml\n"
	for path, content := range map[string]string{
		filepath.Join(dir, "modules", "base.yaml"):                  module,
		filepath.Join(dir, "fleets", "test-fleet", "manifest.yaml"): manifest,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *configAuthoringState) discoverValidateRender(fleet string) error {
	var err error
	s.discoverOutput, err = runRemotr("config", "discover", "--fleet", fleet, s.repo)
	if err != nil {
		return fmt.Errorf("discover: %w: %s", err, s.discoverOutput)
	}
	s.validateOutput, err = runRemotr("config", "validate", s.repo)
	if err != nil {
		return fmt.Errorf("validate: %w: %s", err, s.validateOutput)
	}
	s.renderOutput, err = runRemotr("config", "render", "--fleet", fleet, s.repo)
	if err != nil {
		return fmt.Errorf("render: %w: %s", err, s.renderOutput)
	}
	return nil
}

func (s *configAuthoringState) toolingReportsResourceCapability(kind, capability string) error {
	if !strings.Contains(s.discoverOutput, kind) || !strings.Contains(s.discoverOutput, capability) || !strings.Contains(s.renderOutput, "schemaVersion: 1") {
		return fmt.Errorf("discover %q and render %q do not expose kind %q, capability %q, and schema 1", s.discoverOutput, s.renderOutput, kind, capability)
	}
	return nil
}

func (s *configAuthoringState) validationEmitsLegacyDiagnostic() error {
	if !strings.Contains(s.validateOutput, "WARN") || !strings.Contains(s.validateOutput, "legacy_schema_0") {
		return fmt.Errorf("validation output %q lacks schema-0 deprecation warning", s.validateOutput)
	}
	return nil
}

func (s *configAuthoringState) noComposedArtifactsWritten() error {
	var found []string
	err := filepath.WalkDir(s.repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (entry.Name() == "desired.yaml" || entry.Name() == "crons.yaml") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(found) > 0 {
		return fmt.Errorf("tooling wrote composed artifacts: %v", found)
	}
	return nil
}
func (s *configAuthoringState) renderTwice(fleet string) error {
	s.first, s.err = runRemotr("config", "render", "--fleet", fleet, s.repo)
	if s.err != nil {
		return nil
	}
	s.second, s.err = runRemotr("config", "render", "--fleet", fleet, s.repo)
	return nil
}
func runRemotr(args ...string) (string, error) {
	command := exec.Command("go", append([]string{"run", "-mod=vendor", "./cmd/remotr"}, args...)...)
	command.Dir = repositoryRoot()
	out, err := command.CombinedOutput()
	return string(out), err
}
func repositoryRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
