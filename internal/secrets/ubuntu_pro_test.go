package secrets_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// OS-UPM-054 through OS-UPM-056: Landscape registration-key and CA
// references are independent exact-purpose grants on one resource address.
func TestArtifactAuthorizesExactLandscapeSecretPurposes(t *testing.T) {
	const registrationReference = "remotr:landscape/registration-key-canary@active"
	const caReference = "remotr:landscape/ca-canary@7"
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
  - name: ubuntu-pro
    resources:
      - kind: ubuntuPro
        name: primary-subscription
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/production@active
        landscape:
          state: enrolled
          accountName: production
          computerTitle: workstation
          registrationKeyRef: ` + registrationReference + `
          caRef: ` + caReference + `
`))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		address   string
		reference string
		purpose   string
		want      bool
	}{
		{"registration key", "ubuntu-pro/primary-subscription", registrationReference, "landscape-registration-key", true},
		{"CA", "ubuntu-pro/primary-subscription", caReference, "landscape-ca", true},
		{"registration key as CA", "ubuntu-pro/primary-subscription", registrationReference, "landscape-ca", false},
		{"CA as registration key", "ubuntu-pro/primary-subscription", caReference, "landscape-registration-key", false},
		{"token purpose", "ubuntu-pro/primary-subscription", registrationReference, "ubuntu-pro-token", false},
		{"wrong resource", "sibling/primary-subscription", caReference, "landscape-ca", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := secrets.ArtifactAuthorizes(state, test.address, test.reference, test.purpose); got != test.want {
				t.Fatalf("ArtifactAuthorizes() = %t, want %t", got, test.want)
			}
		})
	}
}
