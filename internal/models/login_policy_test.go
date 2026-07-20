package models_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalProviderOwnedLoginPolicyAndRejectAuthselect(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: access
    targetDistros: [debian, ubuntu]
    resources:
      - kind: loginPolicy
        name: baseline
        provider: pam-auth-update
        enforce: true
        recoveryPrincipals: [recovery]
        rules:
          - {section: auth, control: required, module: pam_faillock.so, arguments: [preauth, deny=5]}
          - {section: account, control: required, module: pam_faillock.so}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].LoginPolicies) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].LoginPolicies[0]
	if resource.Kind != models.ResourceKindLoginPolicy || resource.Priority != 900 || resource.Provider != models.LoginPolicyPAMAuthUpdate || len(resource.Rules) != 2 {
		t.Fatalf("login policy = %#v", resource)
	}

	_, err = models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: rpm-access
    resources:
      - kind: loginPolicy
        name: baseline
        provider: authselect
        priority: 900
        recoveryPrincipals: [recovery]
        rules:
          - {section: auth, control: required, module: pam_faillock.so}
`))
	if err == nil || !strings.Contains(err.Error(), "authselect") || !strings.Contains(err.Error(), "RPM-family roadmap") {
		t.Fatalf("authselect parse error = %v", err)
	}
}

// OS-AEC-052: one pam-auth-update profile can target all sessions or only
// interactive sessions, but cannot represent both scopes independently.
func TestParseLoginPolicyRejectsMixedSessionScopes(t *testing.T) {
	_, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: access
    targetDistros: [ubuntu]
    resources:
      - kind: loginPolicy
        name: mixed-session-scopes
        provider: pam-auth-update
        recoveryPrincipals: [recovery]
        rules:
          - {section: session, control: optional, module: pam_umask.so}
          - {section: session-interactive, control: optional, module: pam_motd.so}
`))
	if err == nil || !strings.Contains(err.Error(), "session and session-interactive") {
		t.Fatalf("mixed session-scope parse error = %v", err)
	}
}

func TestLoginPolicyValidationBoundaries(t *testing.T) {
	base := models.LoginPolicyResource{
		Name: "boundary", Provider: models.LoginPolicyPAMAuthUpdate, Priority: 900,
		RecoveryPrincipals: []string{"recovery"},
		Rules:              []models.PAMRule{{Section: models.PAMAuth, Control: "required", Module: "pam_unix.so"}},
	}
	tests := []struct {
		name    string
		mutate  func(*models.LoginPolicyResource)
		wantErr bool
	}{
		{name: "minimum priority", mutate: func(resource *models.LoginPolicyResource) { resource.Priority = 1 }},
		{name: "maximum priority", mutate: func(resource *models.LoginPolicyResource) { resource.Priority = 10000 }},
		{name: "priority below minimum", mutate: func(resource *models.LoginPolicyResource) { resource.Priority = -1 }, wantErr: true},
		{name: "priority above maximum", mutate: func(resource *models.LoginPolicyResource) { resource.Priority = 10001 }, wantErr: true},
		{name: "all sessions", mutate: func(resource *models.LoginPolicyResource) {
			resource.Rules = []models.PAMRule{{Section: models.PAMSession, Control: "optional", Module: "pam_umask.so"}}
		}},
		{name: "interactive sessions only", mutate: func(resource *models.LoginPolicyResource) {
			resource.Rules = []models.PAMRule{{Section: models.PAMSessionInteractive, Control: "optional", Module: "pam_umask.so"}}
		}},
		{name: "mixed session scopes", mutate: func(resource *models.LoginPolicyResource) {
			resource.Rules = []models.PAMRule{
				{Section: models.PAMSession, Control: "optional", Module: "pam_umask.so"},
				{Section: models.PAMSessionInteractive, Control: "optional", Module: "pam_umask.so"},
			}
		}, wantErr: true},
		{name: "absolute module", mutate: func(resource *models.LoginPolicyResource) {
			resource.Rules[0].Module = "/usr/lib/security/pam_example.so"
		}},
		{name: "whitespace argument", mutate: func(resource *models.LoginPolicyResource) {
			resource.Rules[0].Arguments = []string{"deny=3 unlock_time=60"}
		}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := base
			resource.RecoveryPrincipals = append([]string(nil), base.RecoveryPrincipals...)
			resource.Rules = append([]models.PAMRule(nil), base.Rules...)
			test.mutate(&resource)
			err := resource.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr=%t", err, test.wantErr)
			}
		})
	}
}

func FuzzParseCanonicalLoginPolicy(f *testing.F) {
	f.Add("auth", "required", "pam_unix.so", "nullok", 900)
	f.Add("password", "requisite", "pam_pwquality.so", "retry=3", 10000)
	f.Add("session-interactive", "optional", "pam_umask.so", "umask=0077", 1)
	f.Fuzz(func(t *testing.T, section, control, module, argument string, priority int) {
		if len(section) > 64 || len(control) > 64 || len(module) > 256 || len(argument) > 256 {
			return
		}
		document := fmt.Sprintf(`schemaVersion: 1
configurations:
  - name: fuzz
    targetDistros: [ubuntu]
    resources:
      - kind: loginPolicy
        name: fuzz
        provider: pam-auth-update
        priority: %d
        recoveryPrincipals: [recovery]
        rules:
          - section: %q
            control: %q
            module: %q
            arguments: [%q]
`, priority, section, control, module, argument)
		state, err := models.ParseState(strings.NewReader(document))
		if err != nil {
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].LoginPolicies) != 1 {
			t.Fatalf("accepted canonical login-policy shape = %#v", state.Configurations)
		}
		resource := state.Configurations[0].LoginPolicies[0]
		if resource.Name != "fuzz" || resource.Provider != models.LoginPolicyPAMAuthUpdate || len(resource.Rules) != 1 {
			t.Fatalf("accepted canonical login policy = %#v", resource)
		}
		if err := resource.Validate(); err != nil {
			t.Fatalf("parser accepted invalid login policy: %v", err)
		}
	})
}
