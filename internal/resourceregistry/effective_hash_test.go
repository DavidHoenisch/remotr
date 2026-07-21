package resourceregistry_test

import (
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
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

type multiEffectiveHashResolver struct {
	requests  []secrets.ResolveRequest
	materials map[string][]byte
}

func (r *multiEffectiveHashResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.requests = append(r.requests, request)
	return secrets.Resolved{
		Provider: secrets.ProviderRemotr, Version: "7", ActivationGeneration: uint64(len(r.requests)),
		Fingerprint: "sha256:safe-metadata", Material: r.materials[request.Reference],
	}, nil
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

// OS-UPM-054 through OS-UPM-056: Landscape registration-key and CA identity
// metadata use distinct purposes and secret material has no hash representation.
func TestUbuntuProEffectiveHashScopesLandscapeSecretIdentities(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(`kind: ubuntuPro
name: primary-subscription
lifecycle: attached
tokenRef: remotr:ubuntu-pro/production@active
landscape:
  state: enrolled
  accountName: production
  computerTitle: workstation
  registrationKeyRef: remotr:landscape/registration-key@active
  caRef: remotr:landscape/ca@active
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
	resolver := &multiEffectiveHashResolver{materials: map[string][]byte{
		"remotr:ubuntu-pro/production@active":      []byte("ubuntu-pro-hash-token-canary"),
		"remotr:landscape/registration-key@active": []byte("landscape-registration-key-hash-canary"),
		"remotr:landscape/ca@active":               []byte("landscape-ca-hash-canary"),
	}}
	hash, err := resource.ResolveEffectiveHash(
		context.Background(), "ubuntu-pro/primary-subscription", "ubuntu-pro", "sha256:active-artifact", resolver,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := effectivehash.Validate(hash); err != nil {
		t.Fatalf("effective hash = %q: %v", hash, err)
	}
	wantPurpose := map[string]string{
		"tokenRef":                     "ubuntu-pro-token",
		"landscape.registrationKeyRef": "landscape-registration-key",
		"landscape.caRef":              "landscape-ca",
	}
	if len(resolver.requests) != len(wantPurpose) {
		t.Fatalf("Resolve() requests = %#v", resolver.requests)
	}
	for _, request := range resolver.requests {
		path := ""
		switch request.Reference {
		case "remotr:ubuntu-pro/production@active":
			path = "tokenRef"
		case "remotr:landscape/registration-key@active":
			path = "landscape.registrationKeyRef"
		case "remotr:landscape/ca@active":
			path = "landscape.caRef"
		}
		if path == "" || request.Purpose != wantPurpose[path] || request.ArtifactDigest != "sha256:active-artifact" || request.ResourceAddress != "ubuntu-pro/primary-subscription" {
			t.Errorf("Resolve() request = %#v", request)
		}
	}
	for reference, material := range resolver.materials {
		for index, value := range material {
			if value != 0 {
				t.Fatalf("%s material byte %d was not cleared", reference, index)
			}
		}
	}
}
