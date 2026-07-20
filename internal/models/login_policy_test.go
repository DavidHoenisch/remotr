package models_test

import (
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
