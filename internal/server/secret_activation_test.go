package server

import (
	"bytes"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// OS-LSM-074: a secret activation Change request carries target evidence only
// for the selected consumer and its dependency closure. Unrelated high-risk
// preflight evidence cannot make the narrowed canonical plan invalid.
func TestSecretActivationPlanScopesTargetPreflightToSelectedResource(t *testing.T) {
	const (
		subscriptionHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		auditHash        = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	derived := configcompose.DerivedFleetPlan{
		Plan: changecontrol.FleetPlan{
			Fleet: "engineering", ReleaseRef: "release-1", ArtifactDigest: "artifact-1", HashContractVersion: 1,
			Resources: []changecontrol.ResourcePlan{
				{Address: "audit/load", DesiredHash: auditHash, Risk: models.RiskSensitive, Provider: "command", ProviderRevision: "command-v1"},
				{Address: "subscriptions/primary", DesiredHash: subscriptionHash, Risk: models.RiskSensitive, Provider: "ubuntu-pro", ProviderRevision: "ubuntu-pro-v1"},
			},
			Targets: []changecontrol.TargetEvidence{{
				EndpointID: "endpoint-1", Compatible: true, PreflightReady: true,
				ResourcePreflights: []changecontrol.ResourcePreflightEvidence{
					{Address: "audit/load", Ready: true},
					{Address: "subscriptions/primary", Ready: true},
				},
			}},
		},
		TrustedIdentities: []changecontrol.CanonicalResourceIdentity{
			{Address: "audit/load", EffectiveHash: auditHash, Provider: "command", ProviderRevision: "command-v1", HashContractVersion: 1},
			{Address: "subscriptions/primary", EffectiveHash: subscriptionHash, Provider: "ubuntu-pro", ProviderRevision: "ubuntu-pro-v1", HashContractVersion: 1},
		},
	}
	plan := secrets.ActivationPlan{
		Name: "ubuntu-pro/prod-engineering", Version: "2", Generation: 2, ActorID: "operator-1",
		Uses: []secrets.ActivationUse{{Fleet: "engineering", ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token", Risk: models.RiskSensitive}},
	}

	fleetPlan, trusted, err := activationFleetPlan(derived, plan, []int{0})
	if err != nil {
		t.Fatal(err)
	}
	if len(fleetPlan.Resources) != 1 || len(fleetPlan.Targets) != 1 || len(fleetPlan.Targets[0].ResourcePreflights) != 1 || fleetPlan.Targets[0].ResourcePreflights[0].Address != "subscriptions/primary" {
		t.Fatalf("narrowed activation plan = %#v", fleetPlan)
	}
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { return "change-1" }})
	if _, err := changes.CreateCanonicalChangeRequestBatch([]changecontrol.CanonicalFleetPlan{{Plan: fleetPlan, Trusted: trusted}}, "operator-1"); err != nil {
		t.Fatalf("create canonical activation Change request: %v", err)
	}
}

func TestSecretActivationRejectsPartialTargetConfigurationEvidence(t *testing.T) {
	state, err := models.ParseState(bytes.NewBufferString(`schemaVersion: 1
configurations:
  - name: subscriptions
    targetDistros: [Ubuntu]
    resources:
      - kind: ubuntuPro
        name: primary
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/prod-engineering@active
      - kind: file
        name: marker
        path: /tmp/remotr-marker
        content: present
`))
	if err != nil {
		t.Fatal(err)
	}
	report := registry.StateReport{Items: []registry.StateReportItem{{
		Address: "subscriptions/primary", Provider: "ubuntu-pro", ProviderRevision: "ubuntu-pro-v1",
		EffectiveHashStatus: "authorization_required", Status: registry.StateDrifted,
		PreflightStatus: registry.PlanPreflightReady, PreflightReason: "preflight_ready",
	}}}
	if _, _, accepted := stateForReportedTarget(state, report); accepted {
		t.Fatal("partial target configuration evidence was accepted")
	}
}

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
