package ubuntuproqualification_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/ubuntuproqualification"
)

const apiRevision = "ubuntu-pro-api-v32"

func TestRepositoryManifestStartsAtExactUnadvertisedBoundary(t *testing.T) {
	t.Parallel()

	manifest, err := ubuntuproqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-pro.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	wantReleases := []string{"20.04", "22.04", "24.04", "26.04"}
	if len(manifest.BaseRows) != len(wantReleases) {
		t.Fatalf("base row count = %d, want %d", len(manifest.BaseRows), len(wantReleases))
	}
	for _, release := range wantReleases {
		row, ok := manifest.BaseRow(release, "amd64", apiRevision)
		if !ok {
			t.Errorf("missing exact Ubuntu %s amd64 %s base row", release, apiRevision)
			continue
		}
		if row.Distribution != "ubuntu" || row.Status != "untested" {
			t.Errorf("base row %s = %+v, want exact Ubuntu and untested", release, row)
		}
		for _, selector := range []string{
			"make:provider-matrix-vm-ubuntu-pro-" + release,
			"make:provider-matrix-vm-ubuntu-pro-negative-identities",
			"make:provider-matrix-vm-ubuntu-pro-secret-canary",
		} {
			if !slices.Contains(row.RequiredSelectors, selector) {
				t.Errorf("base row %s missing selector %q", release, selector)
			}
		}
	}

	wantNegativeCases := []string{"pop-os", "linux-mint", "conflicting-os-release", "interim-ubuntu", "future-ubuntu"}
	for _, id := range wantNegativeCases {
		if !manifest.HasNegativeCase(id) {
			t.Errorf("missing negative qualification case %q", id)
		}
	}
	for _, id := range []string{"ubuntu-pre-20.04", "ubuntu-non-amd64", "ubuntu-core", "ubuntu-container", "ubuntu-wsl", "ubuntu-derivatives"} {
		if !manifest.HasNonClaim(id) {
			t.Errorf("missing explicit platform non-claim %q", id)
		}
	}
}

func TestBaseAttachmentCannotManufactureServiceCapabilities(t *testing.T) {
	t.Parallel()

	manifest, err := ubuntuproqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-pro.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	manifest = manifest.Clone()
	manifest.BaseRows[0].Status = "passing"
	if err := ubuntuproqualification.Validate(manifest); err != nil {
		t.Fatalf("Validate() after isolated base promotion = %v", err)
	}

	capabilities := manifest.AdvertisedCapabilities(ubuntuproqualification.Target{
		Distribution: "ubuntu",
		Release:      "20.04",
		Architecture: "amd64",
		APIRevision:  apiRevision,
	})
	if !slices.Equal(capabilities, []string{"resource:ubuntu-pro"}) {
		t.Fatalf("advertised capabilities = %v, want attachment only", capabilities)
	}

	services := []string{"esm-infra", "esm-apps", "livepatch", "usg", "fips", "fips-updates", "realtime-kernel", "ros", "ros-updates", "anbox-cloud", "landscape"}
	for _, release := range []string{"20.04", "22.04", "24.04", "26.04"} {
		for _, service := range services {
			if !manifest.HasCapabilityTuple("service", service, "", release, "amd64", apiRevision) {
				t.Errorf("missing service tuple %s/%s/amd64/%s", service, release, apiRevision)
			}
			if service == "landscape" {
				continue
			}
			for _, mode := range []string{"full", "access-only"} {
				if !manifest.HasCapabilityTuple("enable-mode", service, mode, release, "amd64", apiRevision) {
					t.Errorf("missing enable-mode tuple %s/%s/%s", service, mode, release)
				}
			}
			for _, behavior := range []string{"retain-packages", "purge"} {
				if !manifest.HasCapabilityTuple("disable-behavior", service, behavior, release, "amd64", apiRevision) {
					t.Errorf("missing disable-behavior tuple %s/%s/%s", service, behavior, release)
				}
			}
		}
		for _, variant := range []string{"intel-iotg", "raspi"} {
			if !manifest.HasCapabilityTuple("variant", "realtime-kernel", variant, release, "amd64", apiRevision) {
				t.Errorf("missing real-time kernel variant tuple %s/%s", variant, release)
			}
		}
		for _, environment := range []string{"saas", "self-hosted"} {
			if !manifest.HasCapabilityTuple("landscape-environment", "landscape", environment, release, "amd64", apiRevision) {
				t.Errorf("missing Landscape environment tuple %s/%s", environment, release)
			}
		}
	}
	for _, row := range manifest.CapabilityRows {
		if row.Status != "untested" {
			t.Errorf("capability row %q status = %q, want untested", row.ID, row.Status)
		}
	}
}

// OS-UPM-060 and OS-LPC-022: invocation-only access-only success cannot
// publish a capability until an independently reviewed durable Check exists.
func TestAccessOnlyRowsRemainUnadvertisedWithoutObservation(t *testing.T) {
	t.Parallel()

	manifest, err := ubuntuproqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-pro.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	accessOnlyRows := 0
	for _, row := range manifest.CapabilityRows {
		if row.Kind != "enable-mode" || row.Value != "access-only" {
			continue
		}
		accessOnlyRows++
		if row.Status != "untested" && row.Status != "unsupported" {
			t.Errorf("access-only row %q status = %q, want untested or unsupported", row.ID, row.Status)
		}
	}
	if accessOnlyRows == 0 {
		t.Fatal("qualification manifest has no access-only evidence rows")
	}
	for _, release := range []string{"20.04", "22.04", "24.04", "26.04"} {
		capabilities := manifest.AdvertisedCapabilities(ubuntuproqualification.Target{
			Distribution: "ubuntu", Release: release, Architecture: "amd64", APIRevision: apiRevision,
		})
		for _, capability := range capabilities {
			if strings.HasSuffix(capability, "/access-only") {
				t.Errorf("release %s advertises unobservable capability %q", release, capability)
			}
		}
	}
}
