package performance_test

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func TestAnalyzeGrowthRejectsMonotonicGoroutineRetention(t *testing.T) {
	report, err := performance.AnalyzeGrowth([]performance.GrowthSample{
		{ServerGoroutines: 10},
		{ServerGoroutines: 11},
		{ServerGoroutines: 12},
		{ServerGoroutines: 13},
	}, performance.GrowthLimits{ServerGoroutines: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Violations) != 1 || report.Violations[0].Metric != "server_goroutines" {
		t.Fatalf("growth report = %+v", report)
	}
}

func TestAnalyzeGrowthCoversEverySoakResourceAndIgnoresRecoveredSpikes(t *testing.T) {
	growing, err := performance.AnalyzeGrowth([]performance.GrowthSample{
		{ServerHeapBytes: 1, ServerGoroutines: 1, DatabaseBackends: 1, DatabaseRows: 1, AgentRSSBytes: 1, AgentGoroutines: 1, TemporaryBytes: 1, RollbackBytes: 1},
		{ServerHeapBytes: 2, ServerGoroutines: 2, DatabaseBackends: 2, DatabaseRows: 2, AgentRSSBytes: 2, AgentGoroutines: 2, TemporaryBytes: 2, RollbackBytes: 2},
		{ServerHeapBytes: 3, ServerGoroutines: 3, DatabaseBackends: 3, DatabaseRows: 3, AgentRSSBytes: 3, AgentGoroutines: 3, TemporaryBytes: 3, RollbackBytes: 3},
	}, performance.GrowthLimits{
		ServerHeapBytes: 1, ServerGoroutines: 1, DatabaseBackends: 1, DatabaseRows: 1,
		AgentRSSBytes: 1, AgentGoroutines: 1, TemporaryBytes: 1, RollbackBytes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if growing.Passed || len(growing.Violations) != 8 {
		t.Fatalf("growing report = %+v", growing)
	}

	recovered, err := performance.AnalyzeGrowth([]performance.GrowthSample{
		{ServerHeapBytes: 1},
		{ServerHeapBytes: 3},
		{ServerHeapBytes: 2},
	}, performance.GrowthLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if !recovered.Passed {
		t.Fatalf("recovered spike was treated as retained growth: %+v", recovered)
	}
}
