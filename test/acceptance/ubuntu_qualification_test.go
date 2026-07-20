package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"
)

// OS-AEC-095: the checked-in Ubuntu qualification repository is exercised
// only through the public operator configuration CLI.
func TestUbuntu2404QualificationRepository(t *testing.T) {
	feature, err := os.ReadFile("features/ubuntu_qualification.feature")
	if err != nil {
		t.Fatal(err)
	}
	state := &ubuntuQualificationState{}
	status := Run(t, []godog.Feature{{Name: "ubuntu_qualification.feature", Contents: feature}}, func(ctx *godog.ScenarioContext) {
		ctx.Before(func(context.Context, *godog.Scenario) (context.Context, error) {
			*state = ubuntuQualificationState{}
			return context.Background(), nil
		})
		ctx.Step(`^the checked-in Ubuntu 24\.04 M1-M5 qualification repository$`, state.useRepository)
		ctx.Step(`^the operator discovers validates and renders the Ubuntu qualification fleet$`, state.discoverValidateRender)
		ctx.Step(`^the Ubuntu qualification composition preserves every expected contract and safe policy$`, state.assertComposition)
		ctx.Step(`^repeated Ubuntu qualification render is deterministic and source-only$`, state.assertDeterministicSourceOnly)
	})
	if status != 0 {
		t.Fatalf("acceptance status = %d", status)
	}
}

type ubuntuQualificationState struct {
	repo           string
	discoverOutput string
	validateOutput string
	renderOutput   string
}

func (s *ubuntuQualificationState) useRepository() error {
	s.repo = filepath.Join(repositoryRoot(), "test", "config-repos", "ubuntu-2404-m1-m5")
	if info, err := os.Stat(s.repo); err != nil || !info.IsDir() {
		return fmt.Errorf("Ubuntu qualification repository is unavailable: %w", err)
	}
	return nil
}

func (s *ubuntuQualificationState) discoverValidateRender() error {
	var err error
	s.discoverOutput, err = runRemotr("config", "discover", "--fleet", "qualification-ubuntu", s.repo)
	if err != nil {
		return fmt.Errorf("discover Ubuntu qualification repository: %w: %s", err, s.discoverOutput)
	}
	s.validateOutput, err = runRemotr("config", "validate", s.repo)
	if err != nil {
		return fmt.Errorf("validate Ubuntu qualification repository: %w: %s", err, s.validateOutput)
	}
	s.renderOutput, err = runRemotr("config", "render", "--fleet", "qualification-ubuntu", s.repo)
	if err != nil {
		return fmt.Errorf("render Ubuntu qualification repository: %w: %s", err, s.renderOutput)
	}
	return nil
}

type ubuntuQualificationManifest struct {
	Rows []struct {
		CapabilityID    string  `yaml:"capability_id"`
		ComposedAddress *string `yaml:"composed_address"`
	} `yaml:"rows"`
}

type ubuntuQualificationDocument struct {
	SchemaVersion  int `yaml:"schemaVersion"`
	Configurations []struct {
		Name      string           `yaml:"name"`
		Resources []map[string]any `yaml:"resources"`
	} `yaml:"configurations"`
}

func (s *ubuntuQualificationState) assertComposition() error {
	expectedAddresses, expectedCapabilities, err := loadUbuntuQualificationManifest()
	if err != nil {
		return err
	}
	sourceResources, err := loadUbuntuQualificationSourceResources(s.repo)
	if err != nil {
		return err
	}
	renderedResources, err := decodeUbuntuQualificationResources([]byte(s.renderOutput), "rendered qualification artifact")
	if err != nil {
		return err
	}
	if err := requireExactAddressSet(expectedAddresses, sourceResources, renderedResources); err != nil {
		return err
	}
	for address, source := range sourceResources {
		canonicalizeQualificationResource(source)
		canonicalizeQualificationResource(renderedResources[address])
		if !reflect.DeepEqual(source, renderedResources[address]) {
			return fmt.Errorf("rendered resource %s does not preserve source semantics:\nsource: %#v\nrendered: %#v", address, source, renderedResources[address])
		}
	}
	for capability := range expectedCapabilities {
		if !strings.Contains(s.discoverOutput, "  - resource:"+capability+"\n") {
			return fmt.Errorf("discovery omitted capability requirement resource:%s", capability)
		}
	}
	for _, requirement := range ubuntuQualificationProviderRequirements {
		if !strings.Contains(s.discoverOutput, "  - "+requirement+"\n") {
			return fmt.Errorf("discovery omitted provider requirement %s", requirement)
		}
	}
	if !strings.Contains(s.discoverOutput, "  - schema:1\n") || !strings.Contains(s.validateOutput, "config validate: ok") {
		return fmt.Errorf("public CLI did not report schema-1 discovery and successful validation")
	}
	if err := assertUbuntuQualificationDependencies(renderedResources); err != nil {
		return err
	}
	if err := assertUbuntuQualificationGuardedPolicies(renderedResources); err != nil {
		return err
	}
	return assertUbuntuQualificationActivation(renderedResources)
}

func (s *ubuntuQualificationState) assertDeterministicSourceOnly() error {
	second, err := runRemotr("config", "render", "--fleet", "qualification-ubuntu", s.repo)
	if err != nil {
		return fmt.Errorf("repeat Ubuntu qualification render: %w: %s", err, second)
	}
	if second != s.renderOutput {
		return fmt.Errorf("repeated Ubuntu qualification render differs from the first render")
	}
	var generated []string
	err = filepath.WalkDir(s.repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (entry.Name() == "desired.yaml" || entry.Name() == "crons.yaml") {
			generated = append(generated, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect Ubuntu qualification source repository: %w", err)
	}
	if len(generated) != 0 {
		return fmt.Errorf("Ubuntu qualification source repository contains generated artifacts: %v", generated)
	}
	return nil
}

var ubuntuQualificationProviderRequirements = []string{
	"provider:authentication/pam-auth-update",
	"provider:browser/chromium",
	"provider:browser/firefox",
	"provider:browser/google-chrome",
	"provider:desktop/dconf",
	"provider:desktop/gsettings",
	"provider:firewall/firewalld",
	"provider:firewall/nftables",
	"provider:host/hostnamectl",
	"provider:host/localectl",
	"provider:init/systemd",
	"provider:kernel/modules",
	"provider:kernel/sysctl",
	"provider:logging/logrotate",
	"provider:network/network-manager",
	"provider:schedule/cron",
	"provider:schedule/systemd-timer",
	"provider:security/apparmor",
	"provider:storage/mount",
	"provider:storage/swap",
	"provider:time-sync/systemd-timesyncd",
}

func loadUbuntuQualificationManifest() (map[string]string, map[string]struct{}, error) {
	path := filepath.Join(repositoryRoot(), "test", "qualification", "ubuntu-2404-applicators.yaml")
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read Ubuntu qualification manifest: %w", err)
	}
	var manifest ubuntuQualificationManifest
	if err := yaml.Unmarshal(contents, &manifest); err != nil {
		return nil, nil, fmt.Errorf("decode Ubuntu qualification manifest: %w", err)
	}
	addresses := make(map[string]string)
	capabilities := make(map[string]struct{})
	for _, row := range manifest.Rows {
		if row.ComposedAddress == nil {
			continue
		}
		if prior, exists := addresses[*row.ComposedAddress]; exists {
			return nil, nil, fmt.Errorf("qualification address %s is shared by %s and %s", *row.ComposedAddress, prior, row.CapabilityID)
		}
		addresses[*row.ComposedAddress] = row.CapabilityID
		capabilities[row.CapabilityID] = struct{}{}
	}
	return addresses, capabilities, nil
}

func loadUbuntuQualificationSourceResources(repo string) (map[string]map[string]any, error) {
	modulePaths, err := filepath.Glob(filepath.Join(repo, "modules", "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("enumerate Ubuntu qualification modules: %w", err)
	}
	resources := make(map[string]map[string]any)
	for _, path := range modulePaths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read qualification module %s: %w", path, err)
		}
		decoded, err := decodeUbuntuQualificationResources(contents, path)
		if err != nil {
			return nil, err
		}
		for address, resource := range decoded {
			if _, exists := resources[address]; exists {
				return nil, fmt.Errorf("duplicate source resource address %s", address)
			}
			resources[address] = resource
		}
	}
	return resources, nil
}

func decodeUbuntuQualificationResources(contents []byte, source string) (map[string]map[string]any, error) {
	var document ubuntuQualificationDocument
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", source, err)
	}
	if document.SchemaVersion != 1 {
		return nil, fmt.Errorf("%s schemaVersion = %d, want 1", source, document.SchemaVersion)
	}
	resources := make(map[string]map[string]any)
	for _, configuration := range document.Configurations {
		for _, resource := range configuration.Resources {
			name, nameOK := resource["name"].(string)
			_, kindOK := resource["kind"].(string)
			if !nameOK || !kindOK {
				return nil, fmt.Errorf("%s contains resource without string kind and name", source)
			}
			address := configuration.Name + "/" + name
			if _, exists := resources[address]; exists {
				return nil, fmt.Errorf("%s contains duplicate resource address %s", source, address)
			}
			resources[address] = resource
		}
	}
	return resources, nil
}

func requireExactAddressSet(expected map[string]string, source, rendered map[string]map[string]any) error {
	for address, capability := range expected {
		for label, resources := range map[string]map[string]map[string]any{"source": source, "rendered": rendered} {
			resource, exists := resources[address]
			if !exists {
				return fmt.Errorf("%s composition omitted expected address %s", label, address)
			}
			if resource["kind"] != capability {
				return fmt.Errorf("%s address %s kind = %v, want %s", label, address, resource["kind"], capability)
			}
		}
	}
	for label, resources := range map[string]map[string]map[string]any{"source": source, "rendered": rendered} {
		if len(resources) != len(expected) {
			var unexpected []string
			for address := range resources {
				if _, exists := expected[address]; !exists {
					unexpected = append(unexpected, address)
				}
			}
			sort.Strings(unexpected)
			return fmt.Errorf("%s composition has %d resources, want %d; unexpected: %v", label, len(resources), len(expected), unexpected)
		}
	}
	return nil
}

// Canonical rendering omits false/zero values for schema fields whose model
// uses a value rather than an optional pointer. Their decoded semantics remain
// unchanged; explicit optional false values are intentionally retained.
func canonicalizeQualificationResource(resource map[string]any) {
	for _, field := range []string{"allowTypeReplacement", "recursive", "allowGIDReassignment", "allowUIDReassignment", "replaceExisting", "runtime", "allowRemove"} {
		if resource[field] == false {
			delete(resource, field)
		}
	}
	for _, field := range []string{"dump", "pass"} {
		if resource[field] == 0 {
			delete(resource, field)
		}
	}
}

func assertUbuntuQualificationDependencies(resources map[string]map[string]any) error {
	want := map[string][]string{
		"m2-filesystem/managed-link":       {"m2-filesystem/managed-directory"},
		"m2-access/managed-user":           {"m2-access/managed-group"},
		"m2-access/managed-authorized-key": {"m2-access/managed-user"},
		"m2-access/managed-sudo":           {"m2-access/managed-group"},
		"m2-access/managed-user-file":      {"m2-access/managed-user"},
	}
	for address, expected := range want {
		actual, ok := stringSlice(resources[address]["dependsOn"])
		if !ok || !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("resource %s dependsOn = %v, want %v", address, actual, expected)
		}
	}
	return nil
}

func assertUbuntuQualificationGuardedPolicies(resources map[string]map[string]any) error {
	guardedRisks := map[string]struct{}{"access": {}, "boot": {}, "connectivity": {}, "destructive": {}, "sensitive": {}}
	for address, resource := range resources {
		risk, _ := resource["risk"].(string)
		if _, guarded := guardedRisks[risk]; !guarded {
			continue
		}
		if resource["policy"] != "report" || resource["enforce"] != false {
			return fmt.Errorf("guarded resource %s must preserve policy=report and enforce=false, got policy=%v enforce=%v", address, resource["policy"], resource["enforce"])
		}
		if group, _ := resource["authorizationGroup"].(string); group == "" {
			return fmt.Errorf("guarded resource %s omits its qualification authorization group", address)
		}
	}
	return nil
}

func assertUbuntuQualificationActivation(resources map[string]map[string]any) error {
	want := map[string]map[string]any{
		"m3-host/kernel-sysctl":               {"activation": "next-boot"},
		"m4-services/systemd-timer":           {"persistent": true},
		"m4-services/coordinated-reboot":      {"generation": "ubuntu-qualification-generation-1", "onlyIfRequired": true},
		"m4-network/nftables-enforcement":     {"audit": false, "rollbackTimeout": "2m"},
		"m5-security/apparmor-profile":        {"mode": "complain"},
		"m5-desktop/dconf-session-policy":     {"lockEnabled": true},
		"m5-desktop/gsettings-session-policy": {"lockEnabled": true},
	}
	for address, fields := range want {
		for field, expected := range fields {
			if actual := resources[address][field]; !reflect.DeepEqual(actual, expected) {
				return fmt.Errorf("resource %s field %s = %v, want %v", address, field, actual, expected)
			}
		}
	}
	return nil
}

func stringSlice(value any) ([]string, bool) {
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(values))
	for i, value := range values {
		var itemOK bool
		out[i], itemOK = value.(string)
		if !itemOK {
			return nil, false
		}
	}
	return out, true
}
