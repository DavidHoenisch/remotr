//go:build vmsafety

package browserpolicy_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/browserpolicy"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

type vmBrowserPolicyCase struct {
	name        string
	value       models.BrowserPolicyValue
	recommended bool
}

// TestBrowserPolicyProviderVM qualifies Chromium, Google Chrome, and Firefox
// through their registered providers and native Ubuntu system paths. It covers
// the exact policy allowlist, every allowlisted native type, mandatory and
// supported recommended levels, LifecycleAbsent, unrelated policy preservation,
// ActivationApplicationRestart, idempotence, and a second Check.
func TestBrowserPolicyProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("browser-policy VM provider test must run as root")
	}
	vmAssertBrowserUbuntu2404(t)
	ctx := context.Background()
	stateDir := filepath.Join(t.TempDir(), "rollback-state")

	chromiumCases := []vmBrowserPolicyCase{
		{name: "HomepageLocation", value: vmBrowserValue(models.BrowserValueString, "https://example.test"), recommended: true},
		{name: "HomepageIsNewTabPage", value: vmBrowserValue(models.BrowserValueBoolean, false), recommended: true},
		{name: "RestoreOnStartup", value: vmBrowserValue(models.BrowserValueInteger, int64(4)), recommended: true},
		{name: "RestoreOnStartupURLs", value: vmBrowserValue(models.BrowserValueStringList, []string{"https://example.test/start"}), recommended: true},
		{name: "URLBlocklist", value: vmBrowserValue(models.BrowserValueStringList, []string{"https://blocked.example/*"}), recommended: true},
		{name: "URLAllowlist", value: vmBrowserValue(models.BrowserValueStringList, []string{"https://allowed.example/*"}), recommended: true},
		{name: "BrowserSignin", value: vmBrowserValue(models.BrowserValueInteger, int64(0))},
		{name: "PasswordManagerEnabled", value: vmBrowserValue(models.BrowserValueBoolean, false), recommended: true},
		{name: "ProxyMode", value: vmBrowserValue(models.BrowserValueString, "fixed_servers"), recommended: true},
		{name: "ProxyServer", value: vmBrowserValue(models.BrowserValueString, "proxy.example.test:8080"), recommended: true},
		{name: "ProxyPacUrl", value: vmBrowserValue(models.BrowserValueString, "https://example.test/proxy.pac"), recommended: true},
		{name: "ProxyBypassList", value: vmBrowserValue(models.BrowserValueStringList, []string{"localhost"}), recommended: true},
		{name: "AutoSelectCertificateForUrls", value: vmBrowserValue(models.BrowserValueStringList, []string{`{"pattern":"https://example.test","filter":{}}`})},
	}
	for _, browser := range []models.BrowserPolicyBrowser{models.BrowserChromium, models.BrowserGoogleChrome} {
		for _, policy := range chromiumCases {
			levels := []models.BrowserPolicyLevel{models.BrowserPolicyLevelMandatory}
			if policy.recommended {
				levels = append(levels, models.BrowserPolicyLevelRecommended)
			}
			for _, level := range levels {
				vmExerciseBrowserPolicy(t, ctx, stateDir, browser, level, policy)
			}
		}
	}

	for _, policy := range []vmBrowserPolicyCase{
		{name: "Homepage", value: vmBrowserValue(models.BrowserValueString, "https://example.test")},
		{name: "DisableTelemetry", value: vmBrowserValue(models.BrowserValueBoolean, true)},
		{name: "DisablePrivateBrowsing", value: vmBrowserValue(models.BrowserValueBoolean, true)},
		{name: "BlockAboutConfig", value: vmBrowserValue(models.BrowserValueBoolean, true)},
		{name: "WebsiteFilter", value: vmBrowserValue(models.BrowserValueObject, map[string]any{"Block": []any{"https://blocked.example/*"}})},
		{name: "Preferences", value: vmBrowserValue(models.BrowserValueObject, map[string]any{"browser.startup.page": map[string]any{"Value": int64(1), "Status": "locked"}})},
	} {
		vmExerciseBrowserPolicy(t, ctx, stateDir, models.BrowserFirefox, models.BrowserPolicyLevelMandatory, policy)
	}
}

func vmExerciseBrowserPolicy(t *testing.T, ctx context.Context, stateDir string, browser models.BrowserPolicyBrowser, level models.BrowserPolicyLevel, policy vmBrowserPolicyCase) {
	t.Helper()
	resource := models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         vmBrowserResourceName(browser, policy.name, level), Browser: browser,
		PolicyName: policy.name, Scope: models.BrowserPolicyScopeSystem, Level: level,
		Value: &policy.value,
	}
	path := vmBrowserPolicyPath(resource)
	vmWriteUnrelatedBrowserPolicy(t, path, browser)
	t.Cleanup(func() { _ = os.Remove(path) })
	provider := vmRegisteredBrowserProvider(t, resource, stateDir, "m5-desktop/"+resource.Name)
	if check := provider.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("%s/%s/%s initial Check = %+v, want drifted", browser, policy.name, level, check)
	}
	wantActivation := []contract.ActivationSignal{{Kind: contract.ActivationApplicationRestart, Target: string(browser)}}
	if result := provider.Apply(ctx); result.Status != contract.Changed || result.RollbackClass != contract.RollbackTransactional || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("%s/%s/%s Apply = %+v", browser, policy.name, level, result)
	}
	if check := provider.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("%s/%s/%s second Check = %+v", browser, policy.name, level, check)
	}
	if result := provider.Apply(ctx); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("%s/%s/%s second Apply = %+v", browser, policy.name, level, result)
	}
	vmAssertBrowserDocument(t, path, browser, policy.name, true)
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("%s policy mode = %v, %v", path, info, err)
	}

	absentResource := resource
	absentResource.Lifecycle, absentResource.Value = models.LifecycleAbsent, nil
	absent := vmRegisteredBrowserProvider(t, absentResource, stateDir, "m5-desktop/"+resource.Name+"-absent")
	if result := absent.Apply(ctx); result.Status != contract.Changed || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("%s/%s/%s absent Apply = %+v", browser, policy.name, level, result)
	}
	if check := absent.Check(ctx); check.Status != contract.Compliant {
		t.Fatalf("%s/%s/%s absent second Check = %+v", browser, policy.name, level, check)
	}
	vmAssertBrowserDocument(t, path, browser, policy.name, false)
}

func vmRegisteredBrowserProvider(t *testing.T, resource models.BrowserPolicyResource, stateDir, address string) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{BrowserPolicies: []models.BrowserPolicyResource{resource}})
	if err != nil || len(resources) != 1 || resources[0].Kind() != models.ResourceKindBrowserPolicy {
		t.Fatalf("browser-policy registry resources = %+v, %v", resources, err)
	}
	handler, err := resources[0].NewProvider(resourceregistry.FactoryContext{
		StateDir: stateDir, ResourceAddress: address, ArtifactDigest: "sha256:browser-policy-vm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := handler.(*browserpolicy.Applicator); !ok {
		t.Fatalf("browser-policy registry provider = %T", handler)
	}
	provider, err := contract.New(handler)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmWriteUnrelatedBrowserPolicy(t *testing.T, path string, browser models.BrowserPolicyBrowser) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	document := map[string]any{"Unrelated": true}
	if browser == models.BrowserFirefox {
		document = map[string]any{"policies": map[string]any{"Unrelated": true}}
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func vmAssertBrowserDocument(t *testing.T, path string, browser models.BrowserPolicyBrowser, policyName string, present bool) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	policies := document
	if browser == models.BrowserFirefox {
		var ok bool
		policies, ok = document["policies"].(map[string]any)
		if !ok {
			t.Fatalf("Firefox policies document = %#v", document)
		}
	}
	if policies["Unrelated"] != true {
		t.Fatalf("%s unrelated policy was not preserved: %#v", browser, policies)
	}
	_, exists := policies[policyName]
	if exists != present {
		t.Fatalf("%s policy %s present = %t, want %t: %#v", browser, policyName, exists, present, policies)
	}
}

func vmBrowserPolicyPath(resource models.BrowserPolicyResource) string {
	level := "recommended"
	if resource.Level == models.BrowserPolicyLevelMandatory {
		level = "managed"
	}
	switch resource.Browser {
	case models.BrowserChromium:
		return filepath.Join("/etc/chromium/policies", level, "remotr-"+resource.Name+".json")
	case models.BrowserGoogleChrome:
		return filepath.Join("/etc/opt/chrome/policies", level, "remotr-"+resource.Name+".json")
	default:
		return "/etc/firefox/policies/policies.json"
	}
}

func vmBrowserResourceName(browser models.BrowserPolicyBrowser, policy string, level models.BrowserPolicyLevel) string {
	return strings.ToLower(fmt.Sprintf("vm-%s-%s-%s", browser, policy, level))
}

func vmBrowserValue(valueType models.BrowserPolicyValueType, value any) models.BrowserPolicyValue {
	return models.BrowserPolicyValue{Type: valueType, Value: value}
}

func vmAssertBrowserUbuntu2404(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ID=ubuntu") || !strings.Contains(string(body), `VERSION_ID="24.04"`) {
		t.Fatalf("unexpected browser VM os-release: %s", body)
	}
}
