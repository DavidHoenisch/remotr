package server

import (
	"bytes"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// OS-LSM-074: planning multiple fleets is one fail-closed transition. A
// failure in a later fleet cannot leave earlier Change requests behind or
// advance the selected secret generation.
func TestSecretActivationPlanningFailureIsAtomicAcrossFleets(t *testing.T) {
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "duplicate-change-id" }})
	repoDir := t.TempDir()
	const desired = `schemaVersion: 1
configurations:
  - name: subscriptions
    resources:
      - kind: ubuntuPro
        name: primary
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/shared@active
`
	artifactStore := &OnDemandArtifactResolver{RepoRoot: repoDir}
	reports := registry.NewMemory()
	uses := make([]secrets.ActivationUse, 0, 2)
	for _, fleet := range []string{"engineering", "production"} {
		writeTestFleetDesired(t, repoDir, fleet, desired)
		_, digest, err := resolveFleetDesiredArtifact(t.Context(), artifactStore, repoDir, fleet, "release-1")
		if err != nil {
			t.Fatal(err)
		}
		endpointID := fleet + "-endpoint"
		if err := reports.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: fleet}); err != nil {
			t.Fatal(err)
		}
		reports.SetEndpointStateReport(endpointID, registry.DriftSummary{ReleaseRef: "release-1", Digest: digest, ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9, Items: []registry.StateReportItem{{
			Address: "subscriptions/primary", Provider: "ubuntu-pro", ProviderRevision: "ubuntu-pro-v1",
			EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Status:        registry.StateDrifted, PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
		}}})
		uses = append(uses, secrets.ActivationUse{
			Fleet: fleet, ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token", Risk: models.RiskSensitive,
			Provider: "ubuntu-pro", ReleaseRef: "release-1", ArtifactDigest: digest, EndpointIDs: []string{endpointID},
		})
	}
	deriver := &ChangePlanDeriver{ConfigRepoPath: repoDir, ArtifactStore: artifactStore, StateReports: reports}
	coordinator := NewSecretActivationCoordinator(changes, deriver)
	keyring, err := secrets.NewKeyring("kek-test", map[string][]byte{"kek-test": bytes.Repeat([]byte{0xe1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := secrets.NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	service, err := secrets.NewRegistryService(secrets.NewMemoryVersionRepository(), envelope, coordinator, coordinator)
	if err != nil {
		t.Fatal(err)
	}
	deriver.Secrets = service
	if _, err := service.Upload(t.Context(), secrets.UploadRequest{Name: "ubuntu-pro/shared", Scope: secrets.ScopeGlobal, Material: []byte("global-canary"), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Activate(t.Context(), secrets.ActivationRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Activate(t.Context(), secrets.ActivationRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1", Uses: uses}); err == nil {
		t.Fatal("activation succeeded after duplicate Change request id")
	}
	metadata, err := service.GetMetadata(t.Context(), "ubuntu-pro/shared", "1")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ActivationGeneration != 1 || len(metadata.Rollouts) != 0 {
		t.Fatalf("secret activation changed after planning failure: %#v", metadata)
	}
	if requests := changes.List(); len(requests) != 0 {
		t.Fatalf("partial Change requests remained after planning failure: %#v", requests)
	}
}
