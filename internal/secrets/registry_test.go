package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-LSM-028, OS-AEC-072: rollback retains an exact prior version reference,
// which blocks deletion unless an authorized operator explicitly abandons it.
func TestRegistryServiceProtectsReferencedPriorVersionUntilAuthorizedAbandonment(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryVersionRepository()
	service := newTestRegistryServiceWithRepository(t, repository, nil, nil)
	service.now = func() time.Time { return now }
	for _, material := range []string{"prior-version-canary", "replacement-version-canary"} {
		if _, err := service.Upload(context.Background(), UploadRequest{Name: "wifi/credential", Fleet: "production", Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Activate(context.Background(), ActivationRequest{Name: "wifi/credential", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	reference, err := service.RetainRollbackReference(context.Background(), RollbackReferenceRequest{
		Name: "wifi/credential", Version: "1", ResourceAddress: "office/wifi",
		ArtifactDigest: "sha256:replacement", Attempt: 1, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reference.Reference != "remotr:wifi/credential@1" || reference.Fingerprint == "" || reference.Status != RollbackReferenceArmed {
		t.Fatalf("rollback reference = %#v", reference)
	}
	if _, err := service.Activate(context.Background(), ActivationRequest{Name: "wifi/credential", Version: "2", ActorID: "operator-2"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteVersion(context.Background(), DeleteVersionRequest{Name: "wifi/credential", Version: "1", ActorID: "operator-2"}); !errors.Is(err, ErrVersionReferenced) {
		t.Fatalf("ordinary deletion error = %v, want referenced-version refusal", err)
	}
	if err := service.DeleteVersion(context.Background(), DeleteVersionRequest{Name: "wifi/credential", Version: "1", ActorID: "operator-2", AbandonRecovery: true}); !errors.Is(err, ErrRecoveryAbandonmentUnauthorized) {
		t.Fatalf("unauthorized abandonment error = %v", err)
	}

	authorized, err := NewRegistryService(repository, service.envelope, nil, nil, WithRecoveryAbandonmentAuthorizer(abandonAuthorizer{"operator-3": true}))
	if err != nil {
		t.Fatal(err)
	}
	authorized.now = service.now
	if err := authorized.DeleteVersion(context.Background(), DeleteVersionRequest{Name: "wifi/credential", Version: "1", ActorID: "operator-3", AbandonRecovery: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := authorized.GetMetadata(context.Background(), "wifi/credential", "1"); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("deleted version lookup error = %v", err)
	}
	encoded, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("prior-version-canary")) || bytes.Contains(encoded, []byte("replacement-version-canary")) {
		t.Fatalf("rollback metadata exposed secret material: %s", encoded)
	}
	var classified executor.SafeSummary
	if err := json.Unmarshal(encoded, &classified); err != nil {
		t.Fatalf("rollback reference metadata is not classified: %v", err)
	}
	if err := classified.Validate(); err != nil || len(classified.Fields) == 0 {
		t.Fatalf("classified rollback reference metadata = %+v, %v", classified, err)
	}
}

func TestRegistryServiceBoundsRollbackReferenceRetentionAndProtectsActiveVersion(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	service := newTestRegistryService(t, nil, nil)
	service.now = func() time.Time { return now }
	if _, err := service.Upload(context.Background(), UploadRequest{Name: "service/token", Fleet: "production", Material: []byte("boundary-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(context.Background(), ActivationRequest{Name: "service/token", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	base := RollbackReferenceRequest{Name: "service/token", Version: "1", ResourceAddress: "base/service", ArtifactDigest: "sha256:artifact", Attempt: 1}
	for _, expiresAt := range []time.Time{now, now.Add(24*time.Hour + time.Nanosecond)} {
		request := base
		request.ExpiresAt = expiresAt
		if _, err := service.RetainRollbackReference(context.Background(), request); err == nil {
			t.Fatalf("RetainRollbackReference(expires=%s) succeeded", expiresAt)
		}
	}
	if err := service.DeleteVersion(context.Background(), DeleteVersionRequest{Name: "service/token", Version: "1", ActorID: "operator-1"}); !errors.Is(err, ErrVersionActive) {
		t.Fatalf("active version deletion error = %v", err)
	}
}

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
	if err != nil || string(activeOne.Material) != "version-one" || activeOne.ActivationGeneration != firstActivation.ActivationGeneration {
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
	if err != nil || string(activeTwo.Material) != "version-two" || activeTwo.ActivationGeneration != secondActivation.ActivationGeneration {
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
	return newTestRegistryServiceWithRepository(t, NewMemoryVersionRepository(), planner, gate)
}

func newTestRegistryServiceWithRepository(t *testing.T, repository VersionRepository, planner ActivationPlanner, gate RolloutGate) *RegistryService {
	t.Helper()
	keyring, err := NewKeyring("kek-test", map[string][]byte{"kek-test": bytes.Repeat([]byte{0xc1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewRegistryService(repository, envelope, planner, gate)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type abandonAuthorizer map[string]bool

func (a abandonAuthorizer) AuthorizeRecoveryAbandonment(_ context.Context, request RecoveryAbandonmentRequest) bool {
	return a[request.ActorID]
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
