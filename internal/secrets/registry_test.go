package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestRegistryServiceUploadCreatesInactiveEncryptedVersionWithoutPlaintextReadback(t *testing.T) {
	service := newTestRegistryService(t, nil, nil)
	material := []byte("registry-secret-canary")
	metadata, err := service.Upload(context.Background(), UploadRequest{
		Name: "repositories/private", Fleet: "production", Material: material, ActorID: "operator-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "repositories/private" || metadata.Version != "1" || metadata.Active || metadata.Revoked || metadata.Fingerprint == "" || metadata.CreatedBy != "operator-1" {
		t.Fatalf("metadata = %#v", metadata)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, material) {
		t.Fatal("upload metadata exposed plaintext")
	}
	listed, err := service.ListMetadata(context.Background(), "repositories/private")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Version != "1" || listed[0].Active {
		t.Fatalf("listed metadata = %#v", listed)
	}
	if _, err := service.Resolve(context.Background(), ResolveRequest{
		Reference: "remotr:repositories/private@active", Fleet: "production",
		ResourceAddress: "base/private", Purpose: "repository-credential",
	}); err == nil {
		t.Fatal("inactive upload changed active endpoint resolution")
	}
}

func TestRegistryServiceExactAndActiveSelectionHaveIndependentEffectiveHashes(t *testing.T) {
	service := newTestRegistryService(t, nil, nil)
	for _, material := range []string{"version-one", "version-two"} {
		if _, err := service.Upload(context.Background(), UploadRequest{Name: "vpn/key", Fleet: "production", Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	firstActivation, err := service.Activate(context.Background(), ActivationRequest{Name: "vpn/key", Version: "1", ActorID: "operator-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !firstActivation.Active || firstActivation.ActivationGeneration != 1 {
		t.Fatalf("first activation = %#v", firstActivation)
	}
	activeOne, err := service.Resolve(context.Background(), ResolveRequest{Reference: "remotr:vpn/key@active", Fleet: "production", ResourceAddress: "base/vpn", Purpose: "network-credential"})
	if err != nil || string(activeOne.Material) != "version-one" {
		t.Fatalf("active one = %#v err=%v", activeOne, err)
	}
	pinnedTwo, err := service.Resolve(context.Background(), ResolveRequest{Reference: "remotr:vpn/key@2", Fleet: "production", ResourceAddress: "base/vpn", Purpose: "network-credential"})
	if err != nil || string(pinnedTwo.Material) != "version-two" {
		t.Fatalf("pinned two = %#v err=%v", pinnedTwo, err)
	}
	pinnedHashBefore, err := EffectiveReferenceHash(ProviderRemotr, "vpn/key", "2", 0, "network-credential")
	if err != nil {
		t.Fatal(err)
	}
	activeHashBefore, err := EffectiveReferenceHash(ProviderRemotr, "vpn/key", "1", firstActivation.ActivationGeneration, "network-credential")
	if err != nil {
		t.Fatal(err)
	}

	secondActivation, err := service.Activate(context.Background(), ActivationRequest{Name: "vpn/key", Version: "2", ActorID: "operator-2"})
	if err != nil {
		t.Fatal(err)
	}
	activeTwo, err := service.Resolve(context.Background(), ResolveRequest{Reference: "remotr:vpn/key@active", Fleet: "production", ResourceAddress: "base/vpn", Purpose: "network-credential"})
	if err != nil || string(activeTwo.Material) != "version-two" {
		t.Fatalf("active two = %#v err=%v", activeTwo, err)
	}
	pinnedOne, err := service.Resolve(context.Background(), ResolveRequest{Reference: "remotr:vpn/key@1", Fleet: "production", ResourceAddress: "base/vpn", Purpose: "network-credential"})
	if err != nil || string(pinnedOne.Material) != "version-one" {
		t.Fatalf("pinned one = %#v err=%v", pinnedOne, err)
	}
	pinnedHashAfter, _ := EffectiveReferenceHash(ProviderRemotr, "vpn/key", "2", 0, "network-credential")
	activeHashAfter, _ := EffectiveReferenceHash(ProviderRemotr, "vpn/key", "2", secondActivation.ActivationGeneration, "network-credential")
	if pinnedHashAfter != pinnedHashBefore {
		t.Fatal("activation changed an exact version hash")
	}
	if activeHashAfter == activeHashBefore {
		t.Fatal("activation did not invalidate the prior active effective hash")
	}
}

func TestRegistryServiceActivationRequiresHighRiskRolloutBeforeResolution(t *testing.T) {
	planner := &recordingActivationPlanner{}
	gate := &recordingRolloutGate{active: make(map[string]bool)}
	service := newTestRegistryService(t, planner, gate)
	if _, err := service.Upload(context.Background(), UploadRequest{Name: "wifi/credential", Fleet: "production", Material: []byte("wifi-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := service.Activate(context.Background(), ActivationRequest{
		Name: "wifi/credential", Version: "1", ActorID: "operator-1",
		Uses: []ActivationUse{{Fleet: "production", ResourceAddress: "office/wifi", Purpose: "network-credential", Risk: models.RiskConnectivity, Provider: "network-manager", ReleaseRef: "release-1", ArtifactDigest: "sha256:artifact", EndpointIDs: []string{"endpoint-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(planner.plans) != 1 || len(metadata.Rollouts) != 1 || metadata.Rollouts[0].ChangeRequestID != "change-1" || metadata.Rollouts[0].EffectiveHash == "" {
		t.Fatalf("plan=%#v metadata=%#v", planner.plans, metadata)
	}
	request := ResolveRequest{Reference: "remotr:wifi/credential@active", Fleet: "production", EndpointID: "endpoint-1", ResourceAddress: "office/wifi", Purpose: "network-credential"}
	if _, err := service.Resolve(context.Background(), request); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("pending rollout resolution err = %v", err)
	}
	gate.active["change-1"] = true
	resolved, err := service.Resolve(context.Background(), request)
	if err != nil || string(resolved.Material) != "wifi-canary" {
		t.Fatalf("authorized resolution = %#v err=%v", resolved, err)
	}
}

func TestRegistryServiceRevocationBlocksFutureResolutionWithoutClaimingEndpointErasure(t *testing.T) {
	service := newTestRegistryService(t, nil, nil)
	if _, err := service.Upload(context.Background(), UploadRequest{Name: "users/alice-hash", Fleet: "production", Material: []byte("revocation-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(context.Background(), ActivationRequest{Name: "users/alice-hash", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := service.Revoke(context.Background(), RevokeRequest{Name: "users/alice-hash", Version: "1", ActorID: "operator-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.Revoked || !metadata.ResolutionBlocked || metadata.EndpointCopyStatus != EndpointCopyRotationRequired || metadata.RevokedBy != "operator-2" {
		t.Fatalf("revocation metadata = %#v", metadata)
	}
	for _, reference := range []string{"remotr:users/alice-hash@active", "remotr:users/alice-hash@1"} {
		_, err := service.Resolve(context.Background(), ResolveRequest{Reference: reference, Fleet: "production", ResourceAddress: "base/alice", Purpose: "password-hash"})
		if !errors.Is(err, ErrVersionRevoked) {
			t.Fatalf("reference %q resolution err = %v", reference, err)
		}
	}
}

func newTestRegistryService(t *testing.T, planner ActivationPlanner, gate RolloutGate) *RegistryService {
	t.Helper()
	keyring, err := NewKeyring("kek-test", map[string][]byte{"kek-test": bytes.Repeat([]byte{0xc1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRegistryService(NewMemoryVersionRepository(), envelope, planner, gate)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type recordingActivationPlanner struct {
	plans []ActivationPlan
}

func (p *recordingActivationPlanner) CreateActivationRollouts(_ context.Context, plan ActivationPlan) ([]RolloutBinding, error) {
	p.plans = append(p.plans, plan)
	use := plan.Uses[0]
	return []RolloutBinding{{Fleet: use.Fleet, ResourceAddress: use.ResourceAddress, Purpose: use.Purpose, Risk: use.Risk, EffectiveHash: use.EffectiveHash, ChangeRequestID: "change-1"}}, nil
}

type recordingRolloutGate struct {
	active map[string]bool
}

func (g *recordingRolloutGate) RolloutActive(_ context.Context, changeRequestID string) bool {
	return g.active[changeRequestID]
}
