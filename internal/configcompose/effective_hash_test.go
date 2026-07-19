package configcompose_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestEffectiveResourcesDeriveCompositionHashesFromTrustedProviderSelections(t *testing.T) {
	state, err := models.ParseState(bytes.NewBufferString(`schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: service
        name: ssh
        provider: systemd
        scope: system
        service: ssh.service
        enabled: false
        active: true
`))
	if err != nil {
		t.Fatal(err)
	}
	resources, err := configcompose.EffectiveResources(context.Background(), state, map[string]configcompose.ProviderSelection{
		"base/ssh": {ID: "systemd"},
	}, "sha256:artifact", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].Address != "base/ssh" || resources[0].ProviderID != "systemd" || resources[0].ProviderRevision != "service-state-v1" || resources[0].HashContractVersion != 1 {
		t.Fatalf("effective composition resources = %+v", resources)
	}
	want, err := effectivehash.Sum(effectivehash.Input{
		ResourceAddress: "base/ssh", ResourceKind: "service",
		Provider: effectivehash.ProviderIdentity{ID: "systemd", ContractRevision: "service-state-v1"},
		Desired: effectivehash.Object{
			"name": effectivehash.String("ssh"), "provider": effectivehash.String("systemd"),
			"scope": effectivehash.String("system"), "service": effectivehash.String("ssh.service"),
			"enabled": effectivehash.Boolean(false), "active": effectivehash.Boolean(true),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resources[0].EffectiveHash != want {
		t.Fatalf("composition effective hash = %q, want %q", resources[0].EffectiveHash, want)
	}
	if _, err := configcompose.EffectiveResources(context.Background(), state, nil, "sha256:artifact", nil); err == nil {
		t.Fatal("composition accepted a resource without trusted provider selection")
	}
}
