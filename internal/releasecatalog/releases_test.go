package releasecatalog

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentReleaseCatalogPublishesProtocolEligibilityOnly(t *testing.T) {
	release, ok, err := AgentReleaseByVersion("v0.6.9")
	if err != nil || !ok {
		t.Fatalf("release=%+v ok=%t err=%v", release, ok, err)
	}
	if !release.CapabilityDocument || !release.UpgradeEligible || release.Revoked || release.Integrity != "sha256-manifest" || len(release.Schemas) != 2 || len(release.Platforms) != 1 || len(release.Architectures) != 2 {
		t.Fatalf("release metadata = %+v", release)
	}
	if _, ok, err := AgentReleaseByVersion("v9.9.9"); err != nil || ok {
		t.Fatalf("unknown release ok=%t err=%v", ok, err)
	}
}

func TestGeneratedCatalogsMatchValidatedCheckedInSources(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "releases.yaml", want: generatedAgentReleasesSourceSHA256},
		{path: filepath.Join("..", "..", "test", "qualification", "ubuntu-pro.yaml"), want: generatedUbuntuProSourceSHA256},
	}
	for _, test := range tests {
		raw, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatal(err)
		}
		got := fmt.Sprintf("%x", sha256.Sum256(raw))
		if got != test.want {
			t.Errorf("generated source %s is stale: got %s want %s", test.path, test.want, got)
		}
	}
}

func TestUbuntuProCatalogIsImmutableAndContainsNoCredentialCanary(t *testing.T) {
	const secretCanary = "remotr-production-token-canary-7f50cbb1"
	if strings.Contains(string(generatedUbuntuProYAML), secretCanary) || strings.Contains(string(generatedAgentReleasesYAML), secretCanary) {
		t.Fatal("generated release payload retained secret canary")
	}
	first, err := UbuntuProQualification()
	if err != nil {
		t.Fatal(err)
	}
	first.BaseRows[0].RequiredSelectors[0] = secretCanary
	second, err := UbuntuProQualification()
	if err != nil {
		t.Fatal(err)
	}
	if second.BaseRows[0].RequiredSelectors[0] == secretCanary {
		t.Fatal("caller mutation contaminated frozen catalog")
	}
}

func TestAgentReleaseCatalogRejectsDuplicateIncompleteAndUnknownMetadata(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "duplicate", raw: "version: 1\nreleases:\n  - {version: v1, schemas: [1], platforms: [linux], architectures: [x86]}\n  - {version: v1, schemas: [1], platforms: [linux], architectures: [x86]}\n"},
		{name: "incomplete", raw: "version: 1\nreleases:\n  - {version: v1, schemas: [], platforms: [linux], architectures: [x86]}\n"},
		{name: "unknown field", raw: "version: 1\nunknown: true\nreleases: []\n"},
		{name: "revoked eligible", raw: "version: 1\nreleases:\n  - {version: v1, upgrade_eligible: true, revoked: true, integrity: sha256-manifest, schemas: [1], platforms: [linux], architectures: [x86]}\n"},
		{name: "missing integrity", raw: "version: 1\nreleases:\n  - {version: v1, upgrade_eligible: true, schemas: [1], platforms: [linux], architectures: [x86]}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeAgentReleases([]byte(test.raw)); err == nil || strings.Contains(err.Error(), test.raw) {
				t.Fatalf("decode error = %v", err)
			}
		})
	}
}
