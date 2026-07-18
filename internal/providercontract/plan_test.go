package providercontract_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-086: provider plan evidence is closed and secret-safe before the
// server can derive an authoritative high-risk plan from it.
func TestPlanDescriptorRejectsFreeFormAndSecretBearingEvidence(t *testing.T) {
	tests := []struct {
		name   string
		effect providercontract.PlanEffect
		want   string
	}{
		{
			name:   "free-form effect",
			effect: providercontract.PlanEffect{Code: "restart whatever the provider decides"},
			want:   "effect code",
		},
		{
			name: "raw secret projection",
			effect: providercontract.PlanEffect{
				Code: providercontract.EffectResourceUpdate,
				Details: executor.SafeSummary{Fields: []executor.SafeField{{
					Path:        "credential",
					Sensitivity: executor.SafeSecret,
					Projection:  executor.SafeValue,
					Text:        "secret-canary-plan-evidence",
				}}},
			},
			want: "details",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := providercontract.PlanDescriptor{
				Effects:       []providercontract.PlanEffect{test.effect},
				RollbackClass: providercontract.RollbackNone,
			}
			if err := descriptor.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPlanDescriptorRejectsDuplicateActivationTargets(t *testing.T) {
	target := providercontract.ActivationTarget{
		Kind: providercontract.ActivationRestart, Target: "telemetry.service",
	}
	descriptor := providercontract.PlanDescriptor{
		Effects:           []providercontract.PlanEffect{{Code: providercontract.EffectResourceUpdate}},
		RollbackClass:     providercontract.RollbackTransactional,
		ActivationTargets: []providercontract.ActivationTarget{target, target},
		BaselineEligible:  true,
	}
	if err := descriptor.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate activation") {
		t.Fatalf("Validate() error = %v, want duplicate activation rejection", err)
	}
}
