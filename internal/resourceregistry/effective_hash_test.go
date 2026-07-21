package resourceregistry_test

import (
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"gopkg.in/yaml.v3"
)

type effectiveHashResolver struct {
	request    secrets.ResolveRequest
	material   []byte
	version    string
	generation uint64
}

func (r *effectiveHashResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.request = request
	return secrets.Resolved{
		Provider: secrets.ProviderRemotr, Version: r.version, ActivationGeneration: r.generation,
		Fingerprint: "sha256:safe-metadata", Material: r.material,
	}, nil
}

func TestDecodedResourceDerivesSharedCanonicalEffectiveHash(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(`kind: service
name: ssh
provider: systemd
scope: system
service: ssh.service
enabled: false
active: true
`), &document); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}

	got, err := resource.EffectiveHash("base/ssh", "systemd", nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := effectivehash.Sum(effectivehash.Input{
		ResourceAddress: "base/ssh",
		ResourceKind:    "service",
		Provider: effectivehash.ProviderIdentity{
			ID: "systemd", ContractRevision: "service-state-v1",
		},
		Desired: effectivehash.Object{
			"name":     effectivehash.String("ssh"),
			"provider": effectivehash.String("systemd"),
			"scope":    effectivehash.String("system"),
			"service":  effectivehash.String("ssh.service"),
			"enabled":  effectivehash.Boolean(false),
			"active":   effectivehash.Boolean(true),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("resource hash = %q, want shared canonical hash %q", got, want)
	}
	if resource.ProviderContractRevision() != "service-state-v1" {
		t.Fatalf("provider contract revision = %q", resource.ProviderContractRevision())
	}
}

func TestEffectiveHashOmitsWildcardSecretProviderOptions(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	hash := func(secret string) string {
		t.Helper()
		var document yaml.Node
		raw := []byte("kind: package\nname: curl\npresent: true\nproviderOptions:\n  apt:\n    credential: " + secret + "\n")
		if err := yaml.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		resource, err := registry.Decode(document.Content[0])
		if err != nil {
			t.Fatal(err)
		}
		got, err := resource.EffectiveHash("base/curl", "apt", nil)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if first, second := hash("canary-one"), hash("canary-two"); first != second {
		t.Fatalf("wildcard-secret provider option changed effective hash: %q != %q", first, second)
	}
}

func TestEffectiveHashCoversTypedAndScalarPublicValues(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("typed resource without authored source", func(t *testing.T) {
		configuration := &models.Configuration{Packages: []models.Package{{Name: "curl", Present: true}}}
		resources, err := registry.Resources(configuration)
		if err != nil || len(resources) != 1 {
			t.Fatalf("Resources() = %d, %v", len(resources), err)
		}
		if _, err := resources[0].EffectiveHash("base/curl", "apt", nil); err != nil {
			t.Fatalf("EffectiveHash() typed resource: %v", err)
		}
	})
	for _, test := range []struct {
		name string
		yaml string
	}{
		{name: "null", yaml: "kind: package\nname: curl\npresent: true\nproviderOptions: null\n"},
		{name: "negative integer", yaml: "kind: aptRepository\nname: archive\nurl: https://archive.example.test/ubuntu\nsuites: [noble]\ncomponents: [main]\npriority: -5\n"},
		{name: "unsigned integer", yaml: "kind: group\nname: developers\ngroup: developers\nlifecycle: present\ngid: 200\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document yaml.Node
			if err := yaml.Unmarshal([]byte(test.yaml), &document); err != nil {
				t.Fatal(err)
			}
			resource, err := registry.Decode(document.Content[0])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := resource.EffectiveHash("base/"+resource.Name(), "native", nil); err != nil {
				t.Fatalf("EffectiveHash(): %v", err)
			}
		})
	}
}

func TestEffectiveHashReplacesSecretReferenceWithResolvedSafeIdentity(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(`kind: networkProfile
name: office
provider: network-manager
selector: {name: wlan0, type: wifi}
profileName: office
profileType: wifi
ssid: corp
credentialRef: remotr:wifi/office@active
audit: false
enforce: true
rollbackTimeout: 2m
`), &document); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	identity := effectivehash.SecretIdentity{
		Path: "credentialRef", Provider: "remotr", Name: "wifi/office",
		Version: "2", ActivationGeneration: 7, Purpose: "network-credential",
	}
	got, err := resource.EffectiveHash("base/office", "network-manager", []effectivehash.SecretIdentity{identity})
	if err != nil {
		t.Fatal(err)
	}
	want, err := effectivehash.Sum(effectivehash.Input{
		ResourceAddress: "base/office", ResourceKind: "networkProfile",
		Provider: effectivehash.ProviderIdentity{ID: "network-manager", ContractRevision: "networkProfile-v1"},
		Desired: effectivehash.Object{
			"name": effectivehash.String("office"), "provider": effectivehash.String("network-manager"),
			"selector":    effectivehash.Object{"name": effectivehash.String("wlan0"), "type": effectivehash.String("wifi")},
			"profileName": effectivehash.String("office"), "profileType": effectivehash.String("wifi"),
			"ssid": effectivehash.String("corp"), "audit": effectivehash.Boolean(false),
			"enforce": effectivehash.Boolean(true), "rollbackTimeout": effectivehash.String("2m"),
		},
		Secrets: []effectivehash.SecretIdentity{identity},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("secret-resolved resource hash = %q, want %q", got, want)
	}
	if _, err := resource.EffectiveHash("base/office", "network-manager", nil); err == nil {
		t.Fatal("secret-following resource hashed without resolved safe identity")
	}
}

// OS-UPM-010 through OS-UPM-013 and OS-UPM-016: Ubuntu Pro effective hashes
// consume safe reference identity only, never bearer-token bytes.
func TestUbuntuProEffectiveHashUsesSafeTokenIdentityAndClearsMaterial(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(`kind: ubuntuPro
name: primary-subscription
lifecycle: attached
tokenRef: remotr:ubuntu-pro/production@active
`), &document); err != nil {
		t.Fatal(err)
	}
	resource, err := registry.Decode(document.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}

	hashWith := func(material, version string, generation uint64) (string, *effectiveHashResolver) {
		t.Helper()
		resolver := &effectiveHashResolver{material: []byte(material), version: version, generation: generation}
		hash, err := resource.ResolveEffectiveHash(
			context.Background(), "ubuntu-pro/primary-subscription", "ubuntu-pro", "sha256:active-artifact", resolver,
		)
		if err != nil {
			t.Fatal(err)
		}
		for index, value := range resolver.material {
			if value != 0 {
				t.Fatalf("resolved token byte %d was not cleared", index)
			}
		}
		return hash, resolver
	}
	first, resolver := hashWith("ubuntu-pro-effective-hash-token-canary", "7", 3)
	second, _ := hashWith("different-token-bytes", "7", 3)
	rotated, _ := hashWith("different-token-bytes", "8", 4)
	if first != second {
		t.Fatalf("token material affected effective hash: %q != %q", first, second)
	}
	if first == rotated {
		t.Fatalf("safe token version metadata did not affect effective hash: %q", first)
	}
	wantRequest := secrets.ResolveRequest{
		Reference: "remotr:ubuntu-pro/production@active", ArtifactDigest: "sha256:active-artifact",
		ResourceAddress: "ubuntu-pro/primary-subscription", Purpose: "ubuntu-pro-token",
	}
	if resolver.request != wantRequest {
		t.Fatalf("Resolve() request = %#v, want %#v", resolver.request, wantRequest)
	}
}
