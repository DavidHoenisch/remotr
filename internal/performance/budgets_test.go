package performance_test

import (
	"encoding/json"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func TestParseBudgetsRequiresEveryApprovedMetricAndMutationPolicy(t *testing.T) {
	metrics := make(map[string]performance.MetricBudget)
	for _, name := range performance.RequiredBudgetMetrics() {
		metrics[name] = performance.MetricBudget{Maximum: 1, Unit: "count"}
	}
	input, err := json.Marshal(performance.BudgetFile{
		SchemaVersion: 1,
		ApprovedAt:    "2026-07-18",
		Environment:   "controlled-remotr-benchmark",
		Regression: performance.RegressionBudget{
			ControlledLatencyPercent: 20, SharedAllocationPercent: 10,
		},
		Metrics:    metrics,
		SoakGrowth: performance.GrowthLimits{},
		Mutation: performance.MutationBudget{
			Tool: "mewt@3.0.1", BlockingSeverity: "high",
			NoNewUnexplainedSurvivors: true, WeeklyComprehensive: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := performance.ParseBudgets(input); err != nil {
		t.Fatalf("valid budget file: %v", err)
	}

	metrics["fleet.warmup.p95_typo"] = performance.MetricBudget{Maximum: 1, Unit: "count"}
	input, err = json.Marshal(performance.BudgetFile{
		SchemaVersion: 1, ApprovedAt: "2026-07-18", Environment: "controlled-remotr-benchmark",
		Regression: performance.RegressionBudget{ControlledLatencyPercent: 20, SharedAllocationPercent: 10},
		Metrics:    metrics, Mutation: performance.MutationBudget{
			Tool: "mewt@3.0.1", BlockingSeverity: "high",
			NoNewUnexplainedSurvivors: true, WeeklyComprehensive: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := performance.ParseBudgets(input); err == nil {
		t.Fatal("budget file with an unknown metric was accepted")
	}
	delete(metrics, "fleet.warmup.p95_typo")

	delete(metrics, performance.RequiredBudgetMetrics()[0])
	input, err = json.Marshal(performance.BudgetFile{
		SchemaVersion: 1, ApprovedAt: "2026-07-18", Environment: "controlled-remotr-benchmark",
		Regression: performance.RegressionBudget{ControlledLatencyPercent: 20, SharedAllocationPercent: 10},
		Metrics:    metrics, Mutation: performance.MutationBudget{
			Tool: "mewt@3.0.1", BlockingSeverity: "high",
			NoNewUnexplainedSurvivors: true, WeeklyComprehensive: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := performance.ParseBudgets(input); err == nil {
		t.Fatal("budget file with a missing required metric was accepted")
	}
}
