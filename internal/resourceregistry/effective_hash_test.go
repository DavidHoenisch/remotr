package resourceregistry_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

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
