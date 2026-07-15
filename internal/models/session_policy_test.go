package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestSessionPolicyValidation(t *testing.T) {
	base := models.SessionPolicyResource{
		Name: "workstation", Provider: models.DesktopSettingProviderGSettings,
		Selector:    models.InteractiveUserSelector{Mode: models.InteractiveUserSelectionAll},
		LockEnabled: sessionBool(true),
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*models.SessionPolicyResource)
	}{
		{name: "empty", mutate: func(r *models.SessionPolicyResource) { r.LockEnabled = nil }},
		{name: "bad manual proxy", mutate: func(r *models.SessionPolicyResource) {
			r.Proxy = &models.SessionProxyPolicy{Mode: models.SessionProxyManual}
		}},
		{name: "bad auto proxy", mutate: func(r *models.SessionPolicyResource) {
			r.Proxy = &models.SessionProxyPolicy{Mode: models.SessionProxyAutomatic, AutomaticURL: "relative.pac"}
		}},
		{name: "bad mime", mutate: func(r *models.SessionPolicyResource) {
			r.DefaultApplications = map[string]string{"not-a-mime": "browser.desktop"}
		}},
		{name: "bad desktop file", mutate: func(r *models.SessionPolicyResource) {
			r.DefaultApplications = map[string]string{"text/html": "../../browser.desktop"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := base
			test.mutate(&resource)
			if err := resource.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid session policy")
			}
		})
	}
}

func TestParseStateCanonicalSessionPolicy(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: workstation
    resources:
      - kind: sessionPolicy
        name: baseline
        provider: gsettings
        selector: {mode: all-interactive}
        lockEnabled: true
        idleTimeoutSeconds: 300
        proxy:
          mode: manual
          httpHost: proxy.example.test
          httpPort: 8080
          ignoreHosts: [localhost]
        disableUserSwitching: true
        defaultApplications:
          text/html: browser.desktop
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].SessionPolicies) != 1 {
		t.Fatalf("state = %#v", state)
	}
}

func sessionBool(value bool) *bool { return &value }
