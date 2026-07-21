package capabilitymatrix

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-UPM-034 through OS-UPM-036 and OS-UPM-043 through OS-UPM-045: an
// authored Ubuntu Pro resource carries only its exact catalog tuple
// requirements. Defaults are capability-bearing behavior, not a bypass.
func TestUbuntuProRequirementsAreExactAndCatalogDerived(t *testing.T) {
	resource := &models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Kind: models.ResourceKindUbuntuPro, Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription",
		TokenRef:     "remotr:ubuntu-pro/production@active",
		Services: []models.UbuntuProService{
			{Name: "esm-infra", State: models.UbuntuProServiceEnabled, EnableMode: models.UbuntuProEnableAccessOnly},
			{Name: "realtime-kernel", State: models.UbuntuProServiceEnabled, Variant: "raspi"},
			{Name: "livepatch", State: models.UbuntuProServiceDisabled, DisableMode: models.UbuntuProPurgePackages},
			{Name: "ros", State: models.UbuntuProServiceDisabled},
		},
		Landscape: &models.UbuntuProLandscape{
			State: models.UbuntuProLandscapeEnrolled, AccountName: "production", ComputerTitle: "workstation",
			ServerURL: "https://landscape.example.test/message-system", PingURL: "https://landscape.example.test/ping",
		},
	}
	want := []string{
		"provider:ubuntu-pro-disable/livepatch/purge",
		"provider:ubuntu-pro-disable/ros/retain-packages",
		"provider:ubuntu-pro-landscape/self-hosted",
		"provider:ubuntu-pro-option/esm-infra/access-only",
		"provider:ubuntu-pro-option/realtime-kernel/full",
		"provider:ubuntu-pro-service/esm-infra",
		"provider:ubuntu-pro-service/landscape",
		"provider:ubuntu-pro-service/livepatch",
		"provider:ubuntu-pro-service/realtime-kernel",
		"provider:ubuntu-pro-service/ros",
		"provider:ubuntu-pro-variant/realtime-kernel/raspi",
		"resource:ubuntu-pro",
		"schema:1",
	}
	if got := Requirements(models.ResourceKindUbuntuPro, resource); !slices.Equal(got, want) {
		t.Fatalf("Requirements() = %#v, want %#v", got, want)
	}
	for _, sibling := range []string{
		"provider:ubuntu-pro-option/esm-infra/full",
		"provider:ubuntu-pro-variant/realtime-kernel/intel-iotg",
		"provider:ubuntu-pro-disable/ros/purge",
		"provider:ubuntu-pro-landscape/saas",
		"provider:ubuntu-pro-service/fips",
	} {
		if slices.Contains(want, sibling) {
			t.Fatalf("test expectation accidentally includes sibling capability %q", sibling)
		}
	}
}
