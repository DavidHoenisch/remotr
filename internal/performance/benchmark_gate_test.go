package performance_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func TestEvaluateBenchmarkBudgetsRequiresEveryControlledMetric(t *testing.T) {
	metrics := make(map[string]performance.MetricBudget)
	for _, name := range performance.RequiredBenchmarkMetrics() {
		metrics[name] = performance.MetricBudget{Maximum: 100, Unit: "test"}
	}
	lines := []string{
		"BenchmarkChangeControlStateRoundTrip400Endpoints-24 1 90 ns/op",
		"BenchmarkPostgresCompiledArtifactLookup-24 1 90 ns/op 80 B/op",
		"BenchmarkPostgresEndpointCheckIn-24 1 90 ns/op",
		"BenchmarkPostgresTelemetryWrite/state-report-24 1 90 ns/op",
		"BenchmarkPostgresTelemetryWrite/system-info-upsert-24 1 90 ns/op",
		"BenchmarkPostgresFleetReporting-24 1 90 ns/op",
		"BenchmarkAgentFullCycleCompliant/resources=1000-24 1 90 ns/op 80 B/op 70 report-bytes/op 0 rollback-storage-bytes 90 peak-RSS-bytes 5 goroutines",
		"BenchmarkAgentFullCycleDriftedApply/resources=1000-24 1 90 ns/op 80 B/op 95 report-bytes/op 0 rollback-storage-bytes 90 peak-RSS-bytes 5 goroutines",
	}
	report, err := performance.EvaluateBenchmarkBudgets(strings.NewReader(strings.Join(lines, "\n")), performance.BudgetFile{Metrics: metrics})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || len(report.Observed) != len(performance.RequiredBenchmarkMetrics()) {
		t.Fatalf("report=%+v", report)
	}

	metrics["postgres.compiled_lookup.ns_op"] = performance.MetricBudget{Maximum: 89, Unit: "ns/op"}
	report, err = performance.EvaluateBenchmarkBudgets(strings.NewReader(strings.Join(lines, "\n")), performance.BudgetFile{Metrics: metrics})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Violations) != 1 || report.Violations[0].Metric != "postgres.compiled_lookup.ns_op" {
		t.Fatalf("over-budget report=%+v", report)
	}

	_, err = performance.EvaluateBenchmarkBudgets(strings.NewReader(strings.Join(lines[:1], "\n")), performance.BudgetFile{Metrics: metrics})
	if err == nil {
		t.Fatal("incomplete controlled benchmark output was accepted")
	}
}

func TestEvaluateRelativeBenchmarksGatesDeterministicMetricsOnSharedRunners(t *testing.T) {
	before := strings.NewReader("BenchmarkParser-24 10 100 ns/op 100 B/op 10 allocs/op\n")
	after := strings.NewReader("BenchmarkParser-24 10 200 ns/op 111 B/op 10 allocs/op\n")
	budgets := BudgetFileWithRegression(20, 10)
	report, err := performance.EvaluateRelativeBenchmarks(before, after, budgets, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Violations) != 1 || report.Violations[0].Unit != "B/op" {
		t.Fatalf("shared report=%+v", report)
	}

	before = strings.NewReader("BenchmarkParser-24 10 100 ns/op 100 B/op\n")
	after = strings.NewReader("BenchmarkParser-24 10 121 ns/op 100 B/op\n")
	report, err = performance.EvaluateRelativeBenchmarks(before, after, budgets, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Violations) != 1 || report.Violations[0].Unit != "ns/op" {
		t.Fatalf("controlled report=%+v", report)
	}
}

func TestEvaluateRelativeBenchmarksRejectsMissingHeadMetric(t *testing.T) {
	before := strings.NewReader(strings.Join([]string{
		"BenchmarkParser-24 10 100 ns/op 100 B/op",
		"BenchmarkSerializer-24 10 100 ns/op 100 B/op",
	}, "\n"))
	after := strings.NewReader("BenchmarkParser-24 10 100 ns/op 100 B/op\n")

	_, err := performance.EvaluateRelativeBenchmarks(before, after, BudgetFileWithRegression(20, 10), false)
	if err == nil {
		t.Fatal("paired benchmark gate accepted an omitted deterministic metric")
	}
}

func BudgetFileWithRegression(latency, allocations float64) performance.BudgetFile {
	return performance.BudgetFile{Regression: performance.RegressionBudget{
		ControlledLatencyPercent: latency,
		SharedAllocationPercent:  allocations,
	}}
}
