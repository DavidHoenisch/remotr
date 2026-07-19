package main

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/loadtest"
	"github.com/DavidHoenisch/remotr/internal/performance"
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

func TestSoakScenarioRequiresPassingGrowthAnalysis(t *testing.T) {
	report := loadtest.Report{
		Waves:         []loadtest.Wave{{Name: "soak-0", Summary: loadtest.Summary{Requests: 400, Successes: 400}}},
		GrowthSamples: []performance.GrowthSample{{}, {}, {}},
		Growth:        &performance.GrowthReport{Passed: true, Samples: 3},
	}
	if !scenarioPassed("soak", report) {
		t.Fatal("passing soak growth evidence was rejected")
	}
	report.Growth.Passed = false
	report.Growth.Violations = []performance.GrowthViolation{{Metric: "server_goroutines", Delta: 3, Limit: 1}}
	if scenarioPassed("soak", report) {
		t.Fatal("monotonic resource growth was accepted")
	}
}

func TestSteadyLoadBudgetRejectsLatencyErrorsAndDatabaseExhaustion(t *testing.T) {
	budgets := performance.BudgetFile{Metrics: map[string]performance.MetricBudget{
		"fleet.warmup.p95_ns":         {Maximum: float64(350 * time.Millisecond), Unit: "ns"},
		"fleet.unchanged.p95_ns":      {Maximum: float64(250 * time.Millisecond), Unit: "ns"},
		"fleet.errors":                {Maximum: 0, Unit: "count"},
		"database.backends":           {Maximum: 32, Unit: "count"},
		"database.deadlocks":          {Maximum: 0, Unit: "count"},
		"server.heap_bytes":           {Maximum: 100, Unit: "bytes"},
		"server.goroutines":           {Maximum: 10, Unit: "count"},
		"server.cpu_jiffies_per_wave": {Maximum: 10, Unit: "jiffies/wave"},
	}}
	report := loadtest.Report{
		Waves: []loadtest.Wave{
			{Name: "warmup", Summary: loadtest.Summary{Requests: 400, Successes: 400, P95: 300 * time.Millisecond}},
			{Name: "unchanged", Summary: loadtest.Summary{Requests: 400, Successes: 400, P95: 200 * time.Millisecond}},
		},
		DatabaseAfter: loadtest.DatabaseMetrics{Backends: 25},
	}
	if err := checkSteadyLoadBudgets(report, budgets); err != nil {
		t.Fatalf("in-budget load rejected: %v", err)
	}
	report.Waves[1].Summary.Errors = 1
	if err := checkSteadyLoadBudgets(report, budgets); err == nil {
		t.Fatal("load with an error was accepted")
	}
	report.Waves[1].Summary.Errors = 0
	report.DatabaseAfter.Backends = 33
	if err := checkSteadyLoadBudgets(report, budgets); err == nil {
		t.Fatal("load with exhausted database budget was accepted")
	}
	report.DatabaseAfter.Backends = 25
	report.GrowthSamples = []performance.GrowthSample{{ServerHeapBytes: 101, ServerGoroutines: 10}}
	if err := checkSteadyLoadBudgets(report, budgets); err == nil {
		t.Fatal("load with over-budget server heap was accepted")
	}
	report.GrowthSamples = []performance.GrowthSample{
		{ServerHeapBytes: 100, ServerGoroutines: 10, ServerCPUJiffies: 100},
		{ServerHeapBytes: 100, ServerGoroutines: 10, ServerCPUJiffies: 119},
		{ServerHeapBytes: 100, ServerGoroutines: 10, ServerCPUJiffies: 120},
	}
	if err := checkSteadyLoadBudgets(report, budgets); err == nil {
		t.Fatal("load with over-budget server CPU per wave was accepted")
	}
}
