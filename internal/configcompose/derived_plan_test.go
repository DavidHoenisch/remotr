package configcompose_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/effectivehash"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestDerivedFleetPlanUsesCanonicalResourceAndProviderDescriptorEvidence(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`
schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: sudo
        name: operators
        lifecycle: present
        ownership: fragment
        subjects: ["%operators"]
        commands: [ALL]
        recoveryPrincipals: [recovery]
`))
	if err != nil {
		t.Fatal(err)
	}
	derived, err := configcompose.DeriveFleetPlan(
		context.Background(), "engineering", "release-1", "sha256:artifact", state,
		map[string]configcompose.ProviderSelection{"base/operators": {ID: "sudo"}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if derived.Plan.Fleet != "engineering" || derived.Plan.ReleaseRef != "release-1" || derived.Plan.ArtifactDigest != "sha256:artifact" || derived.Plan.HashContractVersion != effectivehash.SchemaVersion {
		t.Fatalf("fleet plan identity = %+v", derived.Plan)
	}
	if len(derived.Plan.Resources) != 1 || len(derived.TrustedIdentities) != 1 {
		t.Fatalf("derived plan = %+v trusted = %+v", derived.Plan.Resources, derived.TrustedIdentities)
	}
	resource := derived.Plan.Resources[0]
	if resource.Address != "base/operators" || resource.Risk != models.RiskAccess || resource.Provider != "sudo" || resource.ProviderRevision != "sudo-v1" {
		t.Fatalf("resource identity = %+v", resource)
	}
	if effectivehash.Validate(resource.DesiredHash) != nil || derived.TrustedIdentities[0].EffectiveHash != resource.DesiredHash {
		t.Fatalf("canonical hash evidence = plan:%q trusted:%q", resource.DesiredHash, derived.TrustedIdentities[0].EffectiveHash)
	}
	if len(resource.PredictedEffects) != 1 || resource.PredictedEffects[0].Code != changecontrol.EffectSudoPolicyReplace || resource.RollbackClass != "transactional" || !resource.BaselineEligible {
		t.Fatalf("provider descriptor evidence = %+v", resource)
	}
}
