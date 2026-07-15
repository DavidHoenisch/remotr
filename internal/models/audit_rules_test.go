package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalNamedAuditRules(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: security
    resources:
      - kind: auditRules
        name: identity
        rules:
          - -w /etc/passwd -p wa -k identity
          - -a always,exit -F arch=b64 -S execve -k process
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].AuditRules) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].AuditRules[0]
	if resource.Kind != models.ResourceKindAuditRules || len(resource.Rules) != 2 {
		t.Fatalf("audit rules = %#v", resource)
	}
}
