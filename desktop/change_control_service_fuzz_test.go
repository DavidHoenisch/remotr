package main

import "testing"

func FuzzBaselineAdoptionPlanParsingIsBoundedAndFleetScoped(f *testing.F) {
	f.Add([]byte(`{"fleet":"production","release_ref":"release-1","artifact_digest":"sha256:artifact","targets":[{"endpoint_id":"endpoint-a"}],"resources":[{"address":"base/sudo","desired_hash":"sha256:sudo","risk":"access"}]}`), "production")
	f.Add([]byte(`{"fleet":"other"}`), "production")
	f.Add([]byte(`not-json`), "production")
	f.Add(make([]byte, baselineAdoptionPlanLimit+1), "production")

	f.Fuzz(func(t *testing.T, raw []byte, fleet string) {
		if len(raw) > baselineAdoptionPlanLimit+1 {
			raw = raw[:baselineAdoptionPlanLimit+1]
		}
		if len(fleet) > 255 {
			fleet = fleet[:255]
		}
		plan, err := parseBaselineAdoptionPlan(raw, fleet)
		if err != nil {
			return
		}
		if plan.Fleet != fleet {
			t.Fatalf("parsed Fleet = %q, want exact selected Fleet %q", plan.Fleet, fleet)
		}
		if len(plan.Targets) == 0 || len(plan.Targets) > changeTargetLimit {
			t.Fatalf("parsed target count = %d", len(plan.Targets))
		}
		if len(plan.Resources) == 0 || len(plan.Resources) > changeResourceLimit {
			t.Fatalf("parsed resource count = %d", len(plan.Resources))
		}
	})
}
