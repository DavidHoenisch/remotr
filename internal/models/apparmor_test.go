package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseCanonicalAppArmorProfileModesAndContent(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: security
    targetDistros: [Ubuntu]
    resources:
      - kind: appArmorProfile
        name: service
        profile: usr.bin.service
        mode: complain
        content: |
          profile usr.bin.service {
            /usr/bin/service ix,
          }
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].AppArmorProfiles) != 1 {
		t.Fatalf("parsed state = %#v", state)
	}
	resource := state.Configurations[0].AppArmorProfiles[0]
	if resource.Kind != models.ResourceKindAppArmorProfile || resource.Mode != models.AppArmorComplain {
		t.Fatalf("AppArmor profile = %#v", resource)
	}
}
