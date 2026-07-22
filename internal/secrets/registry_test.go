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

// OS-LSM-065: one global version is retained while any fleet still owns an
// unexpired protected rollback reference.
func TestRegistryServiceGlobalRollbackRetentionAccountsForEveryFleet(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	repository := NewMemoryVersionRepository()
	service := newTestRegistryServiceWithRepository(t, repository, nil, nil)
	service.now = func() time.Time { return now }
	for _, material := range []string{"shared-prior", "shared-replacement"} {
		if _, err := service.Upload(t.Context(), UploadRequest{Name: "ubuntu-pro/shared", Scope: ScopeGlobal, Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Activate(t.Context(), ActivationRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	for index, fleet := range []string{"engineering", "production"} {
		if _, err := service.RetainRollbackReference(t.Context(), RollbackReferenceRequest{
			Name: "ubuntu-pro/shared", Version: "1", ResourceAddress: fleet + "/subscriptions-primary",
			ArtifactDigest: "sha256:" + fleet, Attempt: index + 1, ExpiresAt: now.Add(time.Duration(index+1) * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Activate(t.Context(), ActivationRequest{Name: "ubuntu-pro/shared", Version: "2", ActorID: "operator-2"}); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteVersion(t.Context(), DeleteVersionRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-2"}); !errors.Is(err, ErrVersionReferenced) {
		t.Fatalf("cross-Fleet retained deletion err=%v", err)
	}

	authorized, err := NewRegistryService(repository, service.envelope, nil, nil, WithRecoveryAbandonmentAuthorizer(abandonAuthorizer{"global-admin": true}))
	if err != nil {
		t.Fatal(err)
	}
	authorized.now = service.now
	if err := authorized.DeleteVersion(t.Context(), DeleteVersionRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "global-admin", AbandonRecovery: true}); err != nil {
		t.Fatal(err)
	}
	if active, err := repository.ListActiveRollbackReferences(t.Context(), "ubuntu-pro/shared", "1", now); err != nil || len(active) != 0 {
		t.Fatalf("rollback cleanup = %#v err=%v", active, err)
	}
}

func TestRegistryServiceGlobalRotationAndRevokeExposeBoundedImpact(t *testing.T) {
	const hash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	service := newTestRegistryService(t, completeActivationPlanner(hash, ""), nil)
	for _, material := range []string{"shared-one", "shared-two"} {
		if _, err := service.Upload(t.Context(), UploadRequest{Name: "service/shared", Scope: ScopeGlobal, Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	uses := []ActivationUse{
		{Fleet: "engineering", ResourceAddress: "base/service", Purpose: "repository-credential", Risk: models.RiskNormal},
		{Fleet: "production", ResourceAddress: "base/service", Purpose: "repository-credential", Risk: models.RiskNormal},
	}
	first, err := service.Activate(t.Context(), ActivationRequest{Name: "service/shared", Version: "1", ActorID: "operator-1", Uses: uses})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Activate(t.Context(), ActivationRequest{Name: "service/shared", Version: "2", ActorID: "operator-2", Uses: uses})
	if err != nil {
		t.Fatal(err)
	}
	if first.ActivationGeneration != 1 || second.ActivationGeneration != 2 || second.AffectedFleetCount != 2 {
		t.Fatalf("global rotation first=%#v second=%#v", first, second)
	}
	revoked, err := service.Revoke(t.Context(), RevokeRequest{Name: "service/shared", Version: "2", ActorID: "operator-3"})
	if err != nil {
		t.Fatal(err)
	}
	if !revoked.ResolutionBlocked || revoked.AffectedFleetCount != 2 || revoked.EndpointCopyStatus != EndpointCopyRotationRequired {
		t.Fatalf("global revocation = %#v", revoked)
	}
	versions, err := service.ListMetadata(t.Context(), "service/shared")
	if err != nil || len(versions) != 2 {
		t.Fatalf("global version history = %#v err=%v", versions, err)
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

func TestRegistryServiceUploadKeepsLogicalSecretScopeImmutable(t *testing.T) {
	tests := []struct {
		name       string
		first      UploadRequest
		later      UploadRequest
		wantSecond bool
	}{
		{
			name:       "global remains global",
			first:      UploadRequest{Scope: ScopeGlobal},
			later:      UploadRequest{Scope: ScopeGlobal},
			wantSecond: true,
		},
		{
			name:  "global cannot become fleet",
			first: UploadRequest{Scope: ScopeGlobal},
			later: UploadRequest{Fleet: "production"},
		},
		{
			name:       "same fleet remains valid",
			first:      UploadRequest{Fleet: "production"},
			later:      UploadRequest{Fleet: "production"},
			wantSecond: true,
		},
		{
			name:  "fleet identifier cannot change",
			first: UploadRequest{Fleet: "production"},
			later: UploadRequest{Fleet: "engineering"},
		},
		{
			name:  "endpoint cannot become global",
			first: UploadRequest{EndpointID: "endpoint-1"},
			later: UploadRequest{Scope: ScopeGlobal},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newTestRegistryService(t, nil, nil)
			first := test.first
			first.Name, first.Material, first.ActorID = "immutable/scope", []byte("first-canary"), "operator-1"
			if _, err := service.Upload(t.Context(), first); err != nil {
				t.Fatal(err)
			}
			later := test.later
			later.Name, later.Material, later.ActorID = first.Name, []byte("later-canary"), "operator-2"
			metadata, err := service.Upload(t.Context(), later)
			if test.wantSecond {
				if err != nil || metadata.Version != "2" {
					t.Fatalf("matching scope upload = %#v, %v", metadata, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("scope-changing upload succeeded: %#v", metadata)
			}
			versions, listErr := service.ListMetadata(t.Context(), first.Name)
			if listErr != nil || len(versions) != 1 || versions[0].Version != "1" {
				t.Fatalf("versions after rejection = %#v, %v", versions, listErr)
			}
		})
	}
}

func TestRegistryServiceExactAndActiveSelectionHaveIndependentEffectiveHashes(t *testing.T) {
	service := newTestRegistryService(t, nil, nil)
	for _, material := range []string{"version-one", "version-two"} {
		if _, err := service.Upload(context.Background(), UploadRequest{Name: "vpn/key", Fleet: "production", Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	uses := []ActivationUse{{Fleet: "production", ResourceAddress: "base/vpn", Purpose: "network-credential", Risk: models.RiskNormal}}
	firstActivation, err := service.Activate(context.Background(), ActivationRequest{Name: "vpn/key", Version: "1", ActorID: "operator-1", Uses: uses})
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

	secondActivation, err := service.Activate(context.Background(), ActivationRequest{Name: "vpn/key", Version: "2", ActorID: "operator-2", Uses: uses})
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

// OS-LSM-076: legacy active state with no exact consumer binding is denied;
// absence is not an unrestricted authorization path.
func TestRegistryServiceActiveResolutionRequiresExactRolloutBinding(t *testing.T) {
	service := newTestRegistryService(t, nil, nil)
	if _, err := service.Upload(t.Context(), UploadRequest{Name: "ubuntu-pro/shared", Scope: ScopeGlobal, Material: []byte("missing-binding-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(t.Context(), ActivationRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(t.Context(), ResolveRequest{
		Reference: "remotr:ubuntu-pro/shared@active", Fleet: "engineering", EndpointID: "endpoint-1",
		ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token",
	})
	if !errors.Is(err, ErrUnauthorized) || len(resolved.Material) != 0 {
		t.Fatalf("missing binding resolved=%#v err=%v", resolved, err)
	}
}

func TestRegistryServiceActiveResolutionRequiresExactRolloutTuple(t *testing.T) {
	const hash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	use := ActivationUse{
		Fleet: "engineering", ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token", Risk: models.RiskNormal,
	}
	service := newTestRegistryService(t, completeActivationPlanner(hash, ""), nil)
	if _, err := service.Upload(t.Context(), UploadRequest{
		Name: "ubuntu-pro/shared", Scope: ScopeGlobal, Material: []byte("exact-rollout-canary"), ActorID: "operator-1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(t.Context(), ActivationRequest{
		Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1", Uses: []ActivationUse{use},
	}); err != nil {
		t.Fatal(err)
	}
	exact := ResolveRequest{
		Reference: "remotr:ubuntu-pro/shared@active", Fleet: use.Fleet, EndpointID: "endpoint-1",
		ResourceAddress: use.ResourceAddress, Purpose: use.Purpose,
	}
	resolved, err := service.Resolve(t.Context(), exact)
	if err != nil || string(resolved.Material) != "exact-rollout-canary" {
		t.Fatalf("exact rollout resolved=%#v err=%v", resolved, err)
	}
	for name, mutate := range map[string]func(*ResolveRequest){
		"wrong fleet":   func(request *ResolveRequest) { request.Fleet = "production" },
		"wrong address": func(request *ResolveRequest) { request.ResourceAddress = "subscriptions/secondary" },
		"wrong purpose": func(request *ResolveRequest) { request.Purpose = "repository-credential" },
	} {
		t.Run(name, func(t *testing.T) {
			request := exact
			mutate(&request)
			resolved, err := service.Resolve(t.Context(), request)
			if !errors.Is(err, ErrUnauthorized) || len(resolved.Material) != 0 {
				t.Fatalf("mismatched rollout resolved=%#v err=%v", resolved, err)
			}
		})
	}
}

func TestRegistryServiceActivationPlanningFailuresPreservePriorGeneration(t *testing.T) {
	const hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	baseUse := ActivationUse{Fleet: "engineering", ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token", Risk: models.RiskSensitive}
	tests := []struct {
		name    string
		uses    []ActivationUse
		planner activationPlannerFunc
	}{
		{
			name: "scope violation", uses: []ActivationUse{{Fleet: "production", ResourceAddress: baseUse.ResourceAddress, Purpose: baseUse.Purpose, Risk: baseUse.Risk}},
			planner: completeActivationPlanner(hash, "change-1"),
		},
		{
			name: "duplicate consumer", uses: []ActivationUse{baseUse, baseUse},
			planner: func(_ context.Context, plan ActivationPlan) ([]RolloutBinding, error) {
				return []RolloutBinding{
					{Fleet: plan.Uses[0].Fleet, ResourceAddress: plan.Uses[0].ResourceAddress, Purpose: plan.Uses[0].Purpose, Risk: plan.Uses[0].Risk, EffectiveHash: hash, ChangeRequestID: "change-1"},
					{Fleet: plan.Uses[1].Fleet, ResourceAddress: plan.Uses[1].ResourceAddress, Purpose: plan.Uses[1].Purpose, Risk: plan.Uses[1].Risk, EffectiveHash: hash, ChangeRequestID: "change-2"},
				}, nil
			},
		},
		{name: "omitted consumer", uses: []ActivationUse{baseUse}, planner: func(context.Context, ActivationPlan) ([]RolloutBinding, error) { return nil, nil }},
		{name: "missing high-risk Change request", uses: []ActivationUse{baseUse}, planner: completeActivationPlanner(hash, "")},
		{name: "planner failure", uses: []ActivationUse{baseUse}, planner: func(context.Context, ActivationPlan) ([]RolloutBinding, error) {
			return nil, errors.New("planner unavailable")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newTestRegistryService(t, test.planner, nil)
			if _, err := service.Upload(t.Context(), UploadRequest{Name: "ubuntu-pro/engineering", Scope: ScopeFleet, Fleet: "engineering", Material: []byte("version-one"), ActorID: "operator-1"}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Activate(t.Context(), ActivationRequest{Name: "ubuntu-pro/engineering", Version: "1", ActorID: "operator-1"}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Upload(t.Context(), UploadRequest{Name: "ubuntu-pro/engineering", Scope: ScopeFleet, Fleet: "engineering", Material: []byte("version-two"), ActorID: "operator-1"}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Activate(t.Context(), ActivationRequest{Name: "ubuntu-pro/engineering", Version: "2", ActorID: "operator-2", Uses: test.uses}); err == nil {
				t.Fatal("activation succeeded")
			}
			assertActiveVersionAndGeneration(t, service, "ubuntu-pro/engineering", "1", 1)
		})
	}
}

func TestRegistryServiceLowerRiskCompleteBindingMayCommitWithoutChangeRequest(t *testing.T) {
	const hash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	use := ActivationUse{Fleet: "engineering", ResourceAddress: "repositories/private", Purpose: "repository-credential", Risk: models.RiskNormal}
	service := newTestRegistryService(t, completeActivationPlanner(hash, ""), nil)
	if _, err := service.Upload(t.Context(), UploadRequest{Name: "repositories/private", Fleet: "engineering", Material: []byte("low-risk-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := service.Activate(t.Context(), ActivationRequest{Name: "repositories/private", Version: "1", ActorID: "operator-1", Uses: []ActivationUse{use}})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ActivationGeneration != 1 || len(metadata.Rollouts) != 1 || metadata.Rollouts[0].ChangeRequestID != "" || metadata.Rollouts[0].EffectiveHash != hash {
		t.Fatalf("lower-risk activation = %#v", metadata)
	}
}

func TestRegistryServiceActivationPersistenceFailurePreservesPriorGeneration(t *testing.T) {
	repository := &failingActivateVersionRepository{VersionRepository: NewMemoryVersionRepository()}
	service := newTestRegistryServiceWithRepository(t, repository, nil, nil)
	for _, material := range []string{"version-one", "version-two"} {
		if _, err := service.Upload(t.Context(), UploadRequest{Name: "service/token", Fleet: "engineering", Material: []byte(material), ActorID: "operator-1"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Activate(t.Context(), ActivationRequest{Name: "service/token", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	repository.fail = true
	if _, err := service.Activate(t.Context(), ActivationRequest{Name: "service/token", Version: "2", ActorID: "operator-2"}); err == nil {
		t.Fatal("activation succeeded despite persistence failure")
	}
	assertActiveVersionAndGeneration(t, service, "service/token", "1", 1)
}

func TestRegistryServiceRetainsFleetAndEndpointScopeWithoutGlobalFallback(t *testing.T) {
	service := newTestRegistryService(t, nil, nil)
	for _, upload := range []UploadRequest{
		{Name: "fleet/token", Fleet: "engineering", Material: []byte("fleet-canary"), ActorID: "operator-1"},
		{Name: "endpoint/token", EndpointID: "endpoint-1", Material: []byte("endpoint-canary"), ActorID: "operator-1"},
	} {
		if _, err := service.Upload(t.Context(), upload); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name      string
		request   ResolveRequest
		want      string
		wantError error
	}{
		{name: "fleet match", request: ResolveRequest{Reference: "remotr:fleet/token@1", Fleet: "engineering", EndpointID: "endpoint-1", ResourceAddress: "base/token", Purpose: "repository-credential"}, want: "fleet-canary"},
		{name: "fleet mismatch", request: ResolveRequest{Reference: "remotr:fleet/token@1", Fleet: "production", EndpointID: "endpoint-2", ResourceAddress: "base/token", Purpose: "repository-credential"}, wantError: ErrUnauthorized},
		{name: "endpoint match", request: ResolveRequest{Reference: "remotr:endpoint/token@1", Fleet: "engineering", EndpointID: "endpoint-1", ResourceAddress: "base/token", Purpose: "repository-credential"}, want: "endpoint-canary"},
		{name: "endpoint mismatch", request: ResolveRequest{Reference: "remotr:endpoint/token@1", Fleet: "engineering", EndpointID: "endpoint-2", ResourceAddress: "base/token", Purpose: "repository-credential"}, wantError: ErrUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved, err := service.Resolve(t.Context(), test.request)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) || len(resolved.Material) != 0 {
					t.Fatalf("resolved=%#v err=%v", resolved, err)
				}
				return
			}
			if err != nil || string(resolved.Material) != test.want {
				t.Fatalf("resolved=%#v err=%v", resolved, err)
			}
		})
	}
	if _, err := service.Upload(t.Context(), UploadRequest{Name: "fleet/token", Scope: ScopeGlobal, Material: []byte("fallback-canary"), ActorID: "operator-2"}); !errors.Is(err, ErrScopeImmutable) {
		t.Fatalf("same-name global fallback upload err=%v", err)
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

type activationPlannerFunc func(context.Context, ActivationPlan) ([]RolloutBinding, error)

func (f activationPlannerFunc) CreateActivationRollouts(ctx context.Context, plan ActivationPlan) ([]RolloutBinding, error) {
	return f(ctx, plan)
}

func completeActivationPlanner(hash, changeRequestID string) activationPlannerFunc {
	return func(_ context.Context, plan ActivationPlan) ([]RolloutBinding, error) {
		bindings := make([]RolloutBinding, len(plan.Uses))
		for index, use := range plan.Uses {
			bindings[index] = RolloutBinding{
				Fleet: use.Fleet, ResourceAddress: use.ResourceAddress, Purpose: use.Purpose,
				Risk: use.Risk, EffectiveHash: hash, ChangeRequestID: changeRequestID,
			}
		}
		return bindings, nil
	}
}

type failingActivateVersionRepository struct {
	VersionRepository
	fail bool
}

func (r *failingActivateVersionRepository) ActivateVersion(ctx context.Context, name, version string, generation uint64, actor string, rollouts []RolloutBinding) (StoredVersion, error) {
	if r.fail {
		return StoredVersion{}, errors.New("activation persistence unavailable")
	}
	return r.VersionRepository.ActivateVersion(ctx, name, version, generation, actor, rollouts)
}

func assertActiveVersionAndGeneration(t *testing.T, service *RegistryService, name, wantVersion string, wantGeneration uint64) {
	t.Helper()
	versions, err := service.ListMetadata(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if version.Active {
			if version.Version != wantVersion || version.ActivationGeneration != wantGeneration {
				t.Fatalf("active version = %#v", version)
			}
			return
		}
	}
	t.Fatalf("no active version in %#v", versions)
}
