package resourceregistry_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

func TestDefaultRegistryCoversEveryCurrentResourceContract(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := map[models.ResourceKind]bool{
		models.ResourceKindPackage: false, models.ResourceKindAPTSigningKey: false, models.ResourceKindAPTRepository: false, models.ResourceKindSysctl: false, models.ResourceKindHostname: false, models.ResourceKindFile: false,
		models.ResourceKindDirectory:     false,
		models.ResourceKindLink:          false,
		models.ResourceKindGroup:         false,
		models.ResourceKindAuthorizedKey: false,
		models.ResourceKindKnownHost:     false,
		models.ResourceKindSudo:          false,
		models.ResourceKindUserFile:      false, models.ResourceKindDownload: false,
		models.ResourceKindUser: false, models.ResourceKindSystemd: false,
		models.ResourceKindSystemdUser: false, models.ResourceKindBootstrap: false,
		models.ResourceKindAgentInstall: false, models.ResourceKindFirewall: false,
		models.ResourceKindCommand: false,
	}
	for _, definition := range registry.Definitions() {
		if _, expected := wantKinds[definition.Kind]; !expected {
			t.Fatalf("unexpected registered kind %q", definition.Kind)
		}
		wantKinds[definition.Kind] = true
		if definition.Decode == nil || definition.Validate == nil || definition.Metadata == nil ||
			definition.DefaultRisk == nil || definition.ProviderFactory == nil ||
			definition.OrderingTier == nil || definition.LockDomains == nil || !definition.Sensitivity.Valid() {
			t.Fatalf("kind %q has incomplete registry contract: %#v", definition.Kind, definition)
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Errorf("kind %q is not registered", kind)
		}
	}
}

func TestRegistryDecodesValidatesAndBuildsProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: file\nname: motd\npath: /etc/motd\ncontent: managed\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if resource.Kind() != models.ResourceKindFile || resource.Name() != "motd" {
		t.Fatalf("decoded identity = %q/%q", resource.Kind(), resource.Name())
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if resource.Sensitivity() != resourceregistry.SensitivityPublic || resource.DefaultRisk() != models.RiskNormal {
		t.Fatalf("classification = %q/%q", resource.Sensitivity(), resource.DefaultRisk())
	}
	if resource.OrderingTier() != 1 || len(resource.LockDomains()) != 0 {
		t.Fatalf("ordering/locks = %d/%v", resource.OrderingTier(), resource.LockDomains())
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	if err != nil || handler == nil {
		t.Fatalf("NewProvider() = %T, %v", handler, err)
	}
}

// OS-LIA-009: known_hosts entries manage outbound host trust, so unlike
// authoritative SSH authorization they are normal-risk and may converge
// without an access-recovery preflight.
func TestKnownHostDefaultsToNormalRisk(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	data := []byte("kind: knownHost\nname: git\nlifecycle: present\nownership: named\nscope: system\nhosts: [git.example]\ntype: ssh-ed25519\nkey: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W\nfingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4\nhashing: plain\n")
	if err := yaml.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	if resource.DefaultRisk() != models.RiskNormal {
		t.Fatalf("knownHost default risk = %q, want %q", resource.DefaultRisk(), models.RiskNormal)
	}
}
