package main

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/loadtest"
)

func TestCapabilityMixedScenarioRequiresPopulationOutcomesAndBoundedCache(t *testing.T) {
	populationCounts := map[string]int{
		"compatible": 80, "blocked-existing": 80, "unmanaged-new": 80,
		"telemetry-carrying": 80, "reconnecting": 80,
	}
	targetPopulations := map[string]loadtest.Summary{
		"compatible":         {Requests: 80, Successes: 80},
		"blocked-existing":   {Requests: 80, Successes: 80, CapabilityBlocked: 80},
		"unmanaged-new":      {Requests: 80, Successes: 80, CapabilityBlocked: 80, Unmanaged: 80},
		"telemetry-carrying": {Requests: 80, Successes: 80, CapabilityBlocked: 80},
		"reconnecting":       {Requests: 80, Successes: 80},
	}
	report := loadtest.Report{
		PopulationCounts: populationCounts,
		DatabaseDelta:    loadtest.DatabaseDelta{ArtifactVariantCount: 4},
		Waves: []loadtest.Wave{
			{Name: "capability-baseline-offer", Summary: loadtest.Summary{Requests: 320, Successes: 320}},
			{Name: "capability-baseline-active", Summary: loadtest.Summary{Requests: 320, Successes: 320, Unchanged: 320}},
			{Name: "capability-mixed-target", Summary: loadtest.Summary{Requests: 400, Successes: 400, CapabilityBlocked: 240, Unmanaged: 80}, Populations: targetPopulations},
			{Name: "capability-reconnect", Summary: loadtest.Summary{Requests: 80, Successes: 80, Unchanged: 80, StartSpread: time.Second}},
		},
	}
	if !scenarioPassed("capability-mixed", report) {
		t.Fatal("complete capability-mixed evidence was rejected")
	}
	report.DatabaseDelta.ArtifactVariantCount = 5
	if scenarioPassed("capability-mixed", report) {
		t.Fatal("endpoint-scaled variant-cache growth was accepted")
	}
}
