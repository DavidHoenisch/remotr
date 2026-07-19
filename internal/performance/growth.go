package performance

import "fmt"

// GrowthSample is one bounded observation from a repeated workload.
type GrowthSample struct {
	ServerHeapBytes  int64 `json:"serverHeapBytes"`
	ServerGoroutines int64 `json:"serverGoroutines"`
	ServerCPUJiffies int64 `json:"serverCpuJiffies"`
	DatabaseBackends int64 `json:"databaseBackends"`
	DatabaseRows     int64 `json:"databaseRows"`
	AgentRSSBytes    int64 `json:"agentRSSBytes"`
	AgentGoroutines  int64 `json:"agentGoroutines"`
	TemporaryBytes   int64 `json:"temporaryBytes"`
	RollbackBytes    int64 `json:"rollbackBytes"`
}

// GrowthLimits contains approved maximum retained deltas.
type GrowthLimits struct {
	ServerHeapBytes  int64 `json:"serverHeapBytes"`
	ServerGoroutines int64 `json:"serverGoroutines"`
	DatabaseBackends int64 `json:"databaseBackends"`
	DatabaseRows     int64 `json:"databaseRows"`
	AgentRSSBytes    int64 `json:"agentRSSBytes"`
	AgentGoroutines  int64 `json:"agentGoroutines"`
	TemporaryBytes   int64 `json:"temporaryBytes"`
	RollbackBytes    int64 `json:"rollbackBytes"`
}

// GrowthViolation identifies one monotonically retained metric beyond budget.
type GrowthViolation struct {
	Metric string `json:"metric"`
	Delta  int64  `json:"delta"`
	Limit  int64  `json:"limit"`
}

// GrowthReport is the machine-readable outcome of a soak observation window.
type GrowthReport struct {
	Passed     bool              `json:"passed"`
	Samples    int               `json:"samples"`
	Violations []GrowthViolation `json:"violations"`
}

// AnalyzeGrowth rejects monotonic retained growth beyond an approved bound.
func AnalyzeGrowth(samples []GrowthSample, limits GrowthLimits) (GrowthReport, error) {
	if len(samples) < 3 {
		return GrowthReport{}, fmt.Errorf("growth analysis requires at least three samples")
	}
	report := GrowthReport{Passed: true, Samples: len(samples), Violations: []GrowthViolation{}}
	metrics := []struct {
		name  string
		limit int64
		read  func(GrowthSample) int64
	}{
		{"server_heap_bytes", limits.ServerHeapBytes, func(sample GrowthSample) int64 { return sample.ServerHeapBytes }},
		{"server_goroutines", limits.ServerGoroutines, func(sample GrowthSample) int64 { return sample.ServerGoroutines }},
		{"database_backends", limits.DatabaseBackends, func(sample GrowthSample) int64 { return sample.DatabaseBackends }},
		{"database_rows", limits.DatabaseRows, func(sample GrowthSample) int64 { return sample.DatabaseRows }},
		{"agent_rss_bytes", limits.AgentRSSBytes, func(sample GrowthSample) int64 { return sample.AgentRSSBytes }},
		{"agent_goroutines", limits.AgentGoroutines, func(sample GrowthSample) int64 { return sample.AgentGoroutines }},
		{"temporary_bytes", limits.TemporaryBytes, func(sample GrowthSample) int64 { return sample.TemporaryBytes }},
		{"rollback_bytes", limits.RollbackBytes, func(sample GrowthSample) int64 { return sample.RollbackBytes }},
	}
	for _, metric := range metrics {
		values := make([]int64, len(samples))
		for index, sample := range samples {
			values[index] = metric.read(sample)
		}
		delta := monotonicDelta(values)
		if delta <= metric.limit {
			continue
		}
		report.Passed = false
		report.Violations = append(report.Violations, GrowthViolation{Metric: metric.name, Delta: delta, Limit: metric.limit})
	}
	return report, nil
}

func monotonicDelta(values []int64) int64 {
	for index := 1; index < len(values); index++ {
		if values[index] < values[index-1] {
			return 0
		}
	}
	return values[len(values)-1] - values[0]
}
