package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseStateCanonicalProviderNeutralServices(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`
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
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Services) != 2 {
		t.Fatalf("services = %+v", state.Configurations)
	}
	system, user := state.Configurations[0].Services[0], state.Configurations[0].Services[1]
	if system.Provider != models.ServiceProviderSystemd || system.Scope != models.ServiceScopeSystem || system.Service != "ssh.service" || system.Enabled == nil || !*system.Enabled || system.Masked == nil || *system.Masked {
		t.Fatalf("system service = %+v", system)
	}
	if user.Provider != models.ServiceProviderSystemd || user.Scope != models.ServiceScopeUser || user.Users != "interactive" || !user.Linger {
		t.Fatalf("user service = %+v", user)
	}
}

func TestParseStateProviderNeutralServiceRejectsUnsupportedProviderSemantics(t *testing.T) {
	tests := []struct {
		name, resource, want string
	}{
		{"openrc mask", "provider: openrc\n        scope: system\n        service: sshd\n        masked: true", "openrc does not support masked state"},
		{"system user selector", "provider: systemd\n        scope: system\n        service: ssh.service\n        users: interactive", "system service must not declare users or linger"},
		{"user selector required", "provider: systemd\n        scope: user\n        service: desktop.service", "user service requires users: interactive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := models.ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: service\n        name: example\n        " + tt.resource + "\n"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseState() error = %v, want %q", err, tt.want)
			}
		})
	}
}
