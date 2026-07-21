package browserpolicy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/browserpolicy"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// OS-IUP-006: an unsupported browser policy is reported before the provider
// writes an ignored managed-policy key.
func TestApplicator_reportsUnsupportedPolicyWithoutWriting(t *testing.T) {
	root := t.TempDir()
	provider := browserpolicy.New(models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "unknown-policy", Browser: models.BrowserFirefox,
		PolicyName: "NotARealPolicy", Scope: models.BrowserPolicyScopeSystem,
		Level: models.BrowserPolicyLevelMandatory,
		Value: &models.BrowserPolicyValue{Type: models.BrowserValueBoolean, Value: true},
	})
	provider.RootDir = root

	check := provider.Check(context.Background())
	if check.Status != executor.Unsupported || check.ReasonCode != "browser_policy_unsupported" {
		t.Fatalf("Check() = %+v", check)
	}
	path := filepath.Join(root, "etc", "firefox", "policies", "policies.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unsupported policy wrote %s: %v", path, err)
	}
}

func TestApplicator_convergesChromiumRecommendedTypedPolicy(t *testing.T) {
	root := t.TempDir()
	provider := browserpolicy.New(models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "blocked-sites", Browser: models.BrowserChromium,
		PolicyName: "URLBlocklist", Scope: models.BrowserPolicyScopeSystem,
		Level:        models.BrowserPolicyLevelRecommended,
		Value:        &models.BrowserPolicyValue{Type: models.BrowserValueStringList, Value: []string{"https://blocked.example/*"}},
		TrustAnchors: []string{"workstation/corporate-root"},
	})
	provider.RootDir = root
	rollbackRoot := filepath.Join(root, "state", "resource-transactions")
	store, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.ConfigureRollback(store, "base/blocked-sites", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}

	if check := provider.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v", check)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if len(result.Activation) != 1 || result.Activation[0] != (executor.ActivationSignal{Kind: executor.ActivationApplicationRestart, Target: "chromium"}) {
		t.Fatalf("ApplyResult() activation = %+v", result.Activation)
	}
	if check := provider.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}
	path := filepath.Join(root, "etc", "chromium", "policies", "recommended", "remotr-blocked-sites.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	if values, ok := document["URLBlocklist"].([]any); !ok || len(values) != 1 || values[0] != "https://blocked.example/*" {
		t.Fatalf("policy document = %#v", document)
	}
	if bytes.Contains(body, []byte("corporate-root")) || bytes.Contains(body, []byte("BEGIN CERTIFICATE")) || bytes.Contains(body, []byte("PRIVATE KEY")) {
		t.Fatalf("managed browser policy copied trust or private material: %s", body)
	}
	restartedStore, err := rollbackstore.New(rollbackstore.Options{Root: rollbackRoot})
	if err != nil {
		t.Fatal(err)
	}
	restarted := browserpolicy.New(provider.Resource)
	restarted.RootDir = root
	if err := restarted.ConfigureRollback(restartedStore, "base/blocked-sites", "sha256:artifact"); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Revert(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rollback did not remove newly created policy: %v", err)
	}
}

func TestApplicator_chromiumMandatoryUsesManagedLocation(t *testing.T) {
	root := t.TempDir()
	provider := browserpolicy.New(models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "homepage", Browser: models.BrowserGoogleChrome,
		PolicyName: "HomepageLocation", Scope: models.BrowserPolicyScopeSystem,
		Level: models.BrowserPolicyLevelMandatory,
		Value: &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"},
	})
	provider.RootDir = root
	if result := provider.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	want := filepath.Join(root, "etc", "opt", "chrome", "policies", "managed", "remotr-homepage.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("mandatory policy not written to managed location %s: %v", want, err)
	}
}

// OS-AEC-098: a Chromium-family resource owns one key in its native JSON
// document; convergence and lifecycle removal preserve unrelated keys.
func TestApplicator_chromiumLifecyclePreservesUnrelatedPolicy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc", "chromium", "policies", "managed", "remotr-homepage.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"Unrelated\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	present := browserpolicy.New(models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "homepage", Browser: models.BrowserChromium,
		PolicyName: "HomepageLocation", Scope: models.BrowserPolicyScopeSystem,
		Level: models.BrowserPolicyLevelMandatory,
		Value: &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"},
	})
	present.RootDir = root
	if result := present.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("present ApplyResult() = %+v", result)
	}

	absentResource := present.Resource
	absentResource.Lifecycle, absentResource.Value = models.LifecycleAbsent, nil
	absent := browserpolicy.New(absentResource)
	absent.RootDir = root
	if result := absent.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("absent ApplyResult() = %+v", result)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	if document["Unrelated"] != true || len(document) != 1 {
		t.Fatalf("document after lifecycle removal = %#v, want only unrelated policy", document)
	}
	if check := absent.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("absent second Check() = %+v", check)
	}
}

func TestApplicator_mergesAndRemovesFirefoxMandatoryPolicy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc", "firefox", "policies", "policies.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"policies\":{\"Unrelated\":true}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	present := browserpolicy.New(models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "telemetry", Browser: models.BrowserFirefox,
		PolicyName: "DisableTelemetry", Scope: models.BrowserPolicyScopeSystem,
		Level: models.BrowserPolicyLevelMandatory,
		Value: &models.BrowserPolicyValue{Type: models.BrowserValueBoolean, Value: true},
	})
	present.RootDir = root
	if result := present.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("present ApplyResult() = %+v", result)
	}

	absentResource := present.Resource
	absentResource.Lifecycle, absentResource.Value = models.LifecycleAbsent, nil
	absent := browserpolicy.New(absentResource)
	absent.RootDir = root
	if result := absent.ApplyResult(context.Background()); result.Status != executor.Changed {
		t.Fatalf("absent ApplyResult() = %+v", result)
	}
	if check := absent.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("absent second Check() = %+v", check)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Policies map[string]any `json:"policies"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	if document.Policies["Unrelated"] != true {
		t.Fatalf("unrelated Firefox policy was not preserved: %#v", document.Policies)
	}
	if _, exists := document.Policies["DisableTelemetry"]; exists {
		t.Fatalf("removed Firefox policy remains: %#v", document.Policies)
	}
}

func TestApplicator_reportsUnsupportedFirefoxRecommendedLevel(t *testing.T) {
	provider := browserpolicy.New(models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "telemetry", Browser: models.BrowserFirefox,
		PolicyName: "DisableTelemetry", Scope: models.BrowserPolicyScopeSystem,
		Level: models.BrowserPolicyLevelRecommended,
		Value: &models.BrowserPolicyValue{Type: models.BrowserValueBoolean, Value: true},
	})
	provider.RootDir = t.TempDir()
	if check := provider.Check(context.Background()); check.Status != executor.Unsupported {
		t.Fatalf("Check() = %+v", check)
	}
}
