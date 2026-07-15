package resourceregistry_test

import (
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/networkfiles"
	servicecontracts "github.com/DavidHoenisch/remotr/internal/applicators/services"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemd"
	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

func TestRegistryRoutesFileBackedNetworkProfileProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: networkProfile\nname: uplink\nprovider: netplan\nselector: {name: eth0}\nprofileName: office\nprofileType: ethernet\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{StateDir: "/var/lib/remotr", Facts: facts.Facts{Network: facts.NetworkNetplan}})
	provider, ok := handler.(*networkfiles.Applicator)
	if err != nil || !ok || provider.StateDir != "/var/lib/remotr" || provider.Resource.Provider != models.NetworkProviderNetplan {
		t.Fatalf("NewProvider() = %#v, %v", handler, err)
	}
}

func TestDefaultRegistryCoversEveryCurrentResourceContract(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := map[models.ResourceKind]bool{
		models.ResourceKindPackage: false, models.ResourceKindAPTSigningKey: false, models.ResourceKindAPTRepository: false, models.ResourceKindSysctl: false, models.ResourceKindKernelModule: false, models.ResourceKindHostname: false, models.ResourceKindHostLocale: false, models.ResourceKindTimeSync: false, models.ResourceKindMount: false, models.ResourceKindSwap: false, models.ResourceKindFile: false,
		models.ResourceKindDirectory:     false,
		models.ResourceKindLink:          false,
		models.ResourceKindGroup:         false,
		models.ResourceKindAuthorizedKey: false,
		models.ResourceKindKnownHost:     false,
		models.ResourceKindSudo:          false,
		models.ResourceKindUserFile:      false, models.ResourceKindDownload: false,
		models.ResourceKindUser: false, models.ResourceKindSystemd: false,
		models.ResourceKindEndpointSchedule: false,
		models.ResourceKindSystemdUser:      false, models.ResourceKindBootstrap: false,
		models.ResourceKindService:      false,
		models.ResourceKindSystemdUnit:  false,
		models.ResourceKindReboot:       false,
		models.ResourceKindAgentInstall: false, models.ResourceKindFirewall: false, models.ResourceKindHostsEntry: false, models.ResourceKindDNSResolver: false, models.ResourceKindRoute: false, models.ResourceKindNetworkProfile: false,
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

func TestRegistryDoesNotAdvertiseDeferredServiceProviders(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: service\nname: ssh\nprovider: openrc\nscope: system\nservice: sshd\nactive: true\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	if handler != nil || err == nil {
		t.Fatalf("NewProvider() = %T, %v", handler, err)
	}
	var unavailable servicecontracts.ProviderNotAdvertisedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("NewProvider() error = %T %v", err, err)
	}
}

func TestRegistryAdaptsProviderNeutralServiceScopesToSystemdProviders(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, yaml string
		assert     func(t *testing.T, handler any)
	}{
		{
			name: "system",
			yaml: "kind: service\nname: ssh\nprovider: systemd\nscope: system\nservice: ssh.service\nenabled: true\nactive: true\nmasked: false\n",
			assert: func(t *testing.T, handler any) {
				provider, ok := handler.(*systemd.Applicator)
				if !ok || provider.Resource.Unit != "ssh.service" || provider.Resource.Masked == nil {
					t.Fatalf("system provider = %#v", handler)
				}
			},
		},
		{
			name: "user",
			yaml: "kind: service\nname: desktop-agent\nprovider: systemd\nscope: user\nservice: desktop-agent.service\nusers: interactive\nlinger: true\nenabled: true\nactive: true\nmasked: false\n",
			assert: func(t *testing.T, handler any) {
				provider, ok := handler.(*systemduser.Applicator)
				if !ok || provider.Resource.Unit != "desktop-agent.service" || provider.Resource.Masked == nil {
					t.Fatalf("user provider = %#v", handler)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(test.yaml), &node); err != nil {
				t.Fatal(err)
			}
			resource, err := registry.Decode(node.Content[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := resource.Validate(); err != nil {
				t.Fatal(err)
			}
			handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
			if err != nil {
				t.Fatal(err)
			}
			test.assert(t, handler)
		})
	}
}

func TestRegistryBuildsHostLocaleProvider(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("kind: hostLocale\nname: utc\ntimezone: UTC\n"), &node); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(node.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if resource.Kind() != models.ResourceKindHostLocale || resource.Name() != "utc" {
		t.Fatalf("decoded identity = %q/%q", resource.Kind(), resource.Name())
	}
	handler, err := resource.NewProvider(resourceregistry.FactoryContext{})
	if err != nil || handler == nil {
		t.Fatalf("NewProvider() = %T, %v", handler, err)
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
