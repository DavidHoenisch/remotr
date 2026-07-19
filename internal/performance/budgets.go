package performance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const budgetSchemaVersion = 1

var requiredBudgetMetrics = []string{
	"fleet.warmup.p95_ns",
	"fleet.unchanged.p95_ns",
	"fleet.errors",
	"server.heap_bytes",
	"server.goroutines",
	"server.cpu_jiffies_per_wave",
	"database.backends",
	"database.deadlocks",
	"postgres.change_control.ns_op",
	"postgres.compiled_lookup.ns_op",
	"postgres.compiled_lookup.bytes_op",
	"postgres.endpoint_checkin.ns_op",
	"postgres.telemetry_report.ns_op",
	"postgres.telemetry_inventory.ns_op",
	"postgres.fleet_report.ns_op",
	"agent.compliant_1000.ns_op",
	"agent.compliant_1000.bytes_op",
	"agent.drifted_1000.ns_op",
	"agent.drifted_1000.bytes_op",
	"agent.peak_rss_bytes",
	"agent.goroutines",
	"agent.report_bytes",
	"agent.rollback_growth_bytes",
}

// MetricBudget is an approved upper bound for one machine-readable metric.
type MetricBudget struct {
	Maximum float64 `json:"maximum"`
	Unit    string  `json:"unit"`
}

// RegressionBudget defines the comparison thresholds used by controlled and
// shared-runner performance gates.
type RegressionBudget struct {
	ControlledLatencyPercent float64 `json:"controlledLatencyPercent"`
	SharedAllocationPercent  float64 `json:"sharedAllocationPercent"`
}

// MutationBudget defines the adopted mutation gate and scheduled campaign.
type MutationBudget struct {
	Tool                      string `json:"tool"`
	BlockingSeverity          string `json:"blockingSeverity"`
	NoNewUnexplainedSurvivors bool   `json:"noNewUnexplainedSurvivors"`
	WeeklyComprehensive       bool   `json:"weeklyComprehensive"`
}

// BudgetFile is the versioned assurance policy shared by local and CI gates.
type BudgetFile struct {
	SchemaVersion int                     `json:"schemaVersion"`
	ApprovedAt    string                  `json:"approvedAt"`
	Environment   string                  `json:"environment"`
	Regression    RegressionBudget        `json:"regression"`
	Metrics       map[string]MetricBudget `json:"metrics"`
	SoakGrowth    GrowthLimits            `json:"soakGrowth"`
	Mutation      MutationBudget          `json:"mutation"`
}

// RequiredBudgetMetrics returns a copy of the complete metric contract.
func RequiredBudgetMetrics() []string {
	return append([]string(nil), requiredBudgetMetrics...)
}

// ParseBudgets decodes and validates the approved assurance policy.
func ParseBudgets(data []byte) (BudgetFile, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var budgets BudgetFile
	if err := decoder.Decode(&budgets); err != nil {
		return BudgetFile{}, fmt.Errorf("decode performance budgets: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return BudgetFile{}, err
	}
	if budgets.SchemaVersion != budgetSchemaVersion {
		return BudgetFile{}, fmt.Errorf("unsupported performance budget schema %d", budgets.SchemaVersion)
	}
	if _, err := time.Parse(time.DateOnly, budgets.ApprovedAt); err != nil {
		return BudgetFile{}, fmt.Errorf("approvedAt must be YYYY-MM-DD: %w", err)
	}
	if strings.TrimSpace(budgets.Environment) == "" {
		return BudgetFile{}, fmt.Errorf("environment is required")
	}
	if budgets.Regression.ControlledLatencyPercent <= 0 || budgets.Regression.SharedAllocationPercent <= 0 {
		return BudgetFile{}, fmt.Errorf("regression percentages must be positive")
	}
	expectedMetrics := make(map[string]struct{}, len(requiredBudgetMetrics))
	for _, name := range requiredBudgetMetrics {
		expectedMetrics[name] = struct{}{}
	}
	for name := range budgets.Metrics {
		if _, ok := expectedMetrics[name]; !ok {
			return BudgetFile{}, fmt.Errorf("unknown performance metric %q", name)
		}
	}
	for _, name := range requiredBudgetMetrics {
		metric, ok := budgets.Metrics[name]
		if !ok {
			return BudgetFile{}, fmt.Errorf("required metric %q is missing", name)
		}
		if metric.Maximum < 0 || strings.TrimSpace(metric.Unit) == "" {
			return BudgetFile{}, fmt.Errorf("metric %q must have a non-negative maximum and unit", name)
		}
	}
	if err := validateGrowthLimits(budgets.SoakGrowth); err != nil {
		return BudgetFile{}, err
	}
	if budgets.Mutation.Tool != "mewt@3.0.1" || budgets.Mutation.BlockingSeverity != "high" ||
		!budgets.Mutation.NoNewUnexplainedSurvivors || !budgets.Mutation.WeeklyComprehensive {
		return BudgetFile{}, fmt.Errorf("mutation policy must pin mewt@3.0.1, block high severity, reject unexplained survivors, and run weekly")
	}
	return budgets, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("performance budgets contain multiple JSON values")
		}
		return fmt.Errorf("decode trailing performance budget data: %w", err)
	}
	return nil
}

func validateGrowthLimits(limits GrowthLimits) error {
	values := []int64{
		limits.ServerHeapBytes, limits.ServerGoroutines, limits.DatabaseBackends, limits.DatabaseRows,
		limits.AgentRSSBytes, limits.AgentGoroutines, limits.TemporaryBytes, limits.RollbackBytes,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("soak growth limits must be non-negative")
		}
	}
	return nil
}
