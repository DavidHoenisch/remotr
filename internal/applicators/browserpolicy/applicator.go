// Package browserpolicy manages supported Chromium-family and Firefox policy.
package browserpolicy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource models.BrowserPolicyResource
	RootDir  string

	previous       []byte
	previousExists bool
	armed          bool
}

func New(resource models.BrowserPolicyResource) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	return &Applicator{Resource: resource, RootDir: string(os.PathSeparator)}
}

func (a *Applicator) Name() string { return string(a.Resource.Browser) + "-policy:" + a.Resource.Name }

func (a *Applicator) Description() string { return "managed browser policy " + a.Resource.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("typed managed browser policy " + a.Resource.PolicyName)
	if err := a.Resource.Validate(); err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	if !supported(a.Resource) {
		return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "browser_policy_unsupported", DesiredSummary: desired, ObservedSummary: "browser provider does not support the requested policy, type, or level"}
	}
	path, err := a.path()
	if err != nil {
		return failed(desired, err)
	}
	document, exists, err := readDocument(path)
	if err != nil {
		return failed(desired, err)
	}
	observed, present, err := a.observed(document, exists)
	if err != nil {
		return failed(desired, err)
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if !present {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "managed browser policy is absent"}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "managed browser policy exists"}
	}
	desiredValue, err := a.Resource.Value.JSONValue()
	if err != nil {
		return failed(desired, err)
	}
	if present && sameJSON(observed, desiredValue) {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "managed browser policy matches"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "managed browser policy differs"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	switch check.Status {
	case executor.Compliant:
		return appErr.ErrStateAlreadyMet
	case executor.Drifted:
	case executor.Unsupported:
		return fmt.Errorf("browser policy is unsupported")
	default:
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("browser policy is not eligible for apply: %s", check.Status)
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	previous, exists, err := readRaw(path)
	if err != nil {
		return err
	}
	if exists {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("managed browser policy path must be a regular file")
		}
	}
	if err := a.write(path, previous, exists); err != nil {
		return err
	}
	a.previous, a.previousExists, a.armed = previous, exists, true
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	}
}

func (a *Applicator) Revert(context.Context) error {
	if !a.armed {
		return appErr.ErrNoOp
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if !a.previousExists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return atomicWrite(path, a.previous, 0o644)
}

func (a *Applicator) path() (string, error) {
	if !filepath.IsAbs(a.RootDir) {
		return "", fmt.Errorf("browser policy root must be absolute")
	}
	var relative string
	levelDirectory := "recommended"
	if a.Resource.Level == models.BrowserPolicyLevelMandatory {
		levelDirectory = "managed"
	}
	switch a.Resource.Browser {
	case models.BrowserChromium:
		relative = filepath.Join("etc", "chromium", "policies", levelDirectory, "remotr-"+a.Resource.Name+".json")
	case models.BrowserGoogleChrome:
		relative = filepath.Join("etc", "opt", "chrome", "policies", levelDirectory, "remotr-"+a.Resource.Name+".json")
	case models.BrowserFirefox:
		relative = filepath.Join("etc", "firefox", "policies", "policies.json")
	default:
		return "", fmt.Errorf("unsupported browser %q", a.Resource.Browser)
	}
	return filepath.Join(a.RootDir, relative), nil
}

func (a *Applicator) observed(document map[string]any, exists bool) (any, bool, error) {
	if !exists {
		return nil, false, nil
	}
	if a.Resource.Browser != models.BrowserFirefox {
		value, present := document[a.Resource.PolicyName]
		return value, present, nil
	}
	policiesValue, exists := document["policies"]
	if !exists {
		return nil, false, nil
	}
	policies, ok := policiesValue.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("firefox policies document has invalid policies object")
	}
	value, present := policies[a.Resource.PolicyName]
	return value, present, nil
}

func (a *Applicator) write(path string, previous []byte, existed bool) error {
	document := map[string]any{}
	if existed {
		if err := json.Unmarshal(previous, &document); err != nil {
			return fmt.Errorf("decode managed browser policy: %w", err)
		}
	}
	if a.Resource.Browser == models.BrowserFirefox {
		policies := map[string]any{}
		if current, ok := document["policies"]; ok {
			var valid bool
			policies, valid = current.(map[string]any)
			if !valid {
				return fmt.Errorf("firefox policies document has invalid policies object")
			}
		}
		if a.Resource.Lifecycle == models.LifecycleAbsent {
			delete(policies, a.Resource.PolicyName)
		} else {
			value, _ := a.Resource.Value.JSONValue()
			policies[a.Resource.PolicyName] = value
		}
		document["policies"] = policies
	} else {
		if a.Resource.Lifecycle == models.LifecycleAbsent {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		value, _ := a.Resource.Value.JSONValue()
		document = map[string]any{a.Resource.PolicyName: value}
	}
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return atomicWrite(path, body, 0o644)
}

func readDocument(path string) (map[string]any, bool, error) {
	body, exists, err := readRaw(path)
	if err != nil || !exists {
		return nil, exists, err
	}
	document := map[string]any{}
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, true, fmt.Errorf("decode managed browser policy: %w", err)
	}
	return document, true, nil
}

func readRaw(path string) ([]byte, bool, error) {
	body, err := os.ReadFile(path) // #nosec G304 -- provider-owned path from validated identifiers.
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-browser-policy-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func sameJSON(a, b any) bool {
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	return aErr == nil && bErr == nil && bytes.Equal(aJSON, bJSON)
}

func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

type support struct {
	typeName models.BrowserPolicyValueType
	levels   map[models.BrowserPolicyLevel]bool
}

var chromiumPolicies = map[string]support{
	"HomepageLocation":             typed(models.BrowserValueString, true),
	"HomepageIsNewTabPage":         typed(models.BrowserValueBoolean, true),
	"RestoreOnStartup":             typed(models.BrowserValueInteger, true),
	"RestoreOnStartupURLs":         typed(models.BrowserValueStringList, true),
	"URLBlocklist":                 typed(models.BrowserValueStringList, true),
	"URLAllowlist":                 typed(models.BrowserValueStringList, true),
	"BrowserSignin":                typed(models.BrowserValueInteger, false),
	"PasswordManagerEnabled":       typed(models.BrowserValueBoolean, true),
	"ProxyMode":                    typed(models.BrowserValueString, true),
	"ProxyServer":                  typed(models.BrowserValueString, true),
	"ProxyPacUrl":                  typed(models.BrowserValueString, true),
	"ProxyBypassList":              typed(models.BrowserValueStringList, true),
	"AutoSelectCertificateForUrls": typed(models.BrowserValueStringList, false),
}

var firefoxPolicies = map[string]support{
	"Homepage":               typed(models.BrowserValueString, false),
	"DisableTelemetry":       typed(models.BrowserValueBoolean, false),
	"DisablePrivateBrowsing": typed(models.BrowserValueBoolean, false),
	"BlockAboutConfig":       typed(models.BrowserValueBoolean, false),
	"WebsiteFilter":          typed(models.BrowserValueObject, false),
	"Preferences":            typed(models.BrowserValueObject, false),
	"Certificates":           typed(models.BrowserValueObject, false),
}

func typed(typeName models.BrowserPolicyValueType, recommended bool) support {
	levels := map[models.BrowserPolicyLevel]bool{models.BrowserPolicyLevelMandatory: true}
	if recommended {
		levels[models.BrowserPolicyLevelRecommended] = true
	}
	return support{typeName: typeName, levels: levels}
}

func supported(resource models.BrowserPolicyResource) bool {
	catalog := chromiumPolicies
	if resource.Browser == models.BrowserFirefox {
		catalog = firefoxPolicies
	}
	entry, ok := catalog[resource.PolicyName]
	if !ok || !entry.levels[resource.Level] {
		return false
	}
	if resource.Lifecycle == models.LifecycleAbsent {
		return true
	}
	return resource.Value != nil && resource.Value.Type == entry.typeName
}
