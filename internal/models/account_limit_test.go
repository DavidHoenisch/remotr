package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalStructuredAccountLimits(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: access
    resources:
      - kind: accountLimit
        name: build
        entries:
          - {domain: "@build", type: soft, item: nofile, value: "65536"}
          - {domain: "@build", type: hard, item: nproc, value: "4096"}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].AccountLimits) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].AccountLimits[0]
	if resource.Kind != models.ResourceKindAccountLimit || len(resource.Entries) != 2 || resource.Entries[0].Type != models.AccountLimitSoft {
		t.Fatalf("account limits = %#v", resource)
	}
}
