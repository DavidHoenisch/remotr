package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestBrowserPolicyValidation(t *testing.T) {
	base := models.BrowserPolicyResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "homepage", Browser: models.BrowserChromium, PolicyName: "HomepageLocation",
		Scope: models.BrowserPolicyScopeSystem, Level: models.BrowserPolicyLevelMandatory,
		Value: &models.BrowserPolicyValue{Type: models.BrowserValueString, Value: "https://example.test"},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*models.BrowserPolicyResource)
	}{
		{name: "invalid browser", mutate: func(r *models.BrowserPolicyResource) { r.Browser = "unknown" }},
		{name: "unsafe policy", mutate: func(r *models.BrowserPolicyResource) { r.PolicyName = "../../Policy" }},
		{name: "user scope", mutate: func(r *models.BrowserPolicyResource) { r.Scope = "user" }},
		{name: "invalid level", mutate: func(r *models.BrowserPolicyResource) { r.Level = "suggested" }},
		{name: "missing value", mutate: func(r *models.BrowserPolicyResource) { r.Value = nil }},
		{name: "typed mismatch", mutate: func(r *models.BrowserPolicyResource) { r.Value.Value = true }},
		{name: "absent value", mutate: func(r *models.BrowserPolicyResource) {
			r.Lifecycle = models.LifecycleAbsent
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := base
			value := *base.Value
			resource.Value = &value
			test.mutate(&resource)
			if err := resource.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid browser policy")
			}
		})
	}
}

func TestParseStateCanonicalBrowserPolicy(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: browsers
    resources:
      - kind: browserPolicy
        name: homepage
        browser: chromium
        policyName: HomepageLocation
        scope: system
        level: mandatory
        value: {type: string, value: "https://example.test"}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].BrowserPolicies) != 1 {
		t.Fatalf("state = %#v", state)
	}
}

func FuzzBrowserPolicyValueValidation(f *testing.F) {
	f.Add("string", "value")
	f.Add("boolean", "true")
	f.Fuzz(func(t *testing.T, typeName, value string) {
		policy := models.BrowserPolicyValue{Type: models.BrowserPolicyValueType(typeName), Value: value}
		_ = policy.Validate()
	})
}
