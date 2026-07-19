package performance

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var benchmarkCPUCounter = regexp.MustCompile(`-[0-9]+$`)

var requiredBenchmarkMetrics = []string{
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

// BenchmarkViolation identifies a controlled measurement beyond its approved
// absolute limit.
type BenchmarkViolation struct {
	Metric  string  `json:"metric"`
	Value   float64 `json:"value"`
	Maximum float64 `json:"maximum"`
	Unit    string  `json:"unit"`
}

// BenchmarkBudgetReport is the machine-readable controlled benchmark gate.
type BenchmarkBudgetReport struct {
	Passed     bool                 `json:"passed"`
	Observed   map[string]float64   `json:"observed"`
	Violations []BenchmarkViolation `json:"violations"`
}

// RelativeBenchmarkViolation identifies a paired benchmark regression.
type RelativeBenchmarkViolation struct {
	Benchmark string  `json:"benchmark"`
	Unit      string  `json:"unit"`
	Before    float64 `json:"before"`
	After     float64 `json:"after"`
	Maximum   float64 `json:"maximum"`
}

// RelativeBenchmarkReport is a paired base/head comparison. Shared runners
// gate deterministic allocation and byte metrics; controlled runners also
// gate latency.
type RelativeBenchmarkReport struct {
	Passed     bool                         `json:"passed"`
	Compared   int                          `json:"compared"`
	Violations []RelativeBenchmarkViolation `json:"violations"`
}

// RequiredBenchmarkMetrics returns the complete controlled benchmark contract.
func RequiredBenchmarkMetrics() []string {
	return append([]string(nil), requiredBenchmarkMetrics...)
}

// EvaluateBenchmarkBudgets parses standard Go benchmark output and checks the
// maximum observed sample for every required native benchmark metric.
func EvaluateBenchmarkBudgets(input io.Reader, budgets BudgetFile) (BenchmarkBudgetReport, error) {
	report := BenchmarkBudgetReport{Passed: true, Observed: map[string]float64{}, Violations: []BenchmarkViolation{}}
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}
		name := benchmarkCPUCounter.ReplaceAllString(fields[0], "")
		measurements := make(map[string]float64)
		for index := 2; index+1 < len(fields); index += 2 {
			value, err := strconv.ParseFloat(fields[index], 64)
			if err != nil {
				continue
			}
			measurements[fields[index+1]] = value
		}
		for metric, value := range benchmarkMetricValues(name, measurements) {
			if previous, exists := report.Observed[metric]; !exists || value > previous {
				report.Observed[metric] = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return BenchmarkBudgetReport{}, err
	}
	for _, metric := range requiredBenchmarkMetrics {
		value, ok := report.Observed[metric]
		if !ok {
			return BenchmarkBudgetReport{}, fmt.Errorf("controlled benchmark output omitted %q", metric)
		}
		budget, ok := budgets.Metrics[metric]
		if !ok {
			return BenchmarkBudgetReport{}, fmt.Errorf("performance budgets omitted %q", metric)
		}
		if value > budget.Maximum {
			report.Passed = false
			report.Violations = append(report.Violations, BenchmarkViolation{Metric: metric, Value: value, Maximum: budget.Maximum, Unit: budget.Unit})
		}
	}
	return report, nil
}

// EvaluateRelativeBenchmarks compares average base/head measurements using
// the approved shared-runner and controlled-runner relative thresholds.
func EvaluateRelativeBenchmarks(before, after io.Reader, budgets BudgetFile, controlled bool) (RelativeBenchmarkReport, error) {
	base, err := parseBenchmarkAverages(before)
	if err != nil {
		return RelativeBenchmarkReport{}, err
	}
	head, err := parseBenchmarkAverages(after)
	if err != nil {
		return RelativeBenchmarkReport{}, err
	}
	report := RelativeBenchmarkReport{Passed: true, Violations: []RelativeBenchmarkViolation{}}
	for key, baseMeasurement := range base {
		if !relativeGateUnit(baseMeasurement.unit, controlled) {
			continue
		}
		headMeasurement, ok := head[key]
		if !ok {
			return RelativeBenchmarkReport{}, fmt.Errorf("head benchmark output omitted %s %s", baseMeasurement.name, baseMeasurement.unit)
		}
		threshold := budgets.Regression.SharedAllocationPercent
		if baseMeasurement.unit == "ns/op" {
			threshold = budgets.Regression.ControlledLatencyPercent
		}
		maximum := baseMeasurement.value * (1 + threshold/100)
		report.Compared++
		if headMeasurement.value > maximum {
			report.Passed = false
			report.Violations = append(report.Violations, RelativeBenchmarkViolation{
				Benchmark: baseMeasurement.name, Unit: baseMeasurement.unit,
				Before: baseMeasurement.value, After: headMeasurement.value, Maximum: maximum,
			})
		}
	}
	if report.Compared == 0 {
		return RelativeBenchmarkReport{}, fmt.Errorf("benchmark pair had no comparable gated metrics")
	}
	return report, nil
}

type benchmarkAverage struct {
	name  string
	unit  string
	value float64
}

func parseBenchmarkAverages(input io.Reader) (map[string]benchmarkAverage, error) {
	type aggregate struct {
		name, unit string
		total      float64
		count      int
	}
	aggregates := make(map[string]aggregate)
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}
		name := benchmarkCPUCounter.ReplaceAllString(fields[0], "")
		for index := 2; index+1 < len(fields); index += 2 {
			value, err := strconv.ParseFloat(fields[index], 64)
			if err != nil {
				continue
			}
			unit := fields[index+1]
			key := name + "\x00" + unit
			got := aggregates[key]
			got.name, got.unit, got.total, got.count = name, unit, got.total+value, got.count+1
			aggregates[key] = got
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	averages := make(map[string]benchmarkAverage, len(aggregates))
	for key, got := range aggregates {
		averages[key] = benchmarkAverage{name: got.name, unit: got.unit, value: got.total / float64(got.count)}
	}
	return averages, nil
}

func relativeGateUnit(unit string, controlled bool) bool {
	if controlled && unit == "ns/op" {
		return true
	}
	if unit == "B/op" || unit == "allocs/op" {
		return true
	}
	return strings.HasSuffix(unit, "_bytes") || strings.HasSuffix(unit, "-bytes/op") || unit == "rollback-storage-bytes"
}

func benchmarkMetricValues(name string, values map[string]float64) map[string]float64 {
	metrics := make(map[string]float64)
	set := func(metric, unit string) {
		if value, ok := values[unit]; ok {
			metrics[metric] = value
		}
	}
	switch name {
	case "BenchmarkChangeControlStateRoundTrip400Endpoints":
		set("postgres.change_control.ns_op", "ns/op")
	case "BenchmarkPostgresCompiledArtifactLookup":
		set("postgres.compiled_lookup.ns_op", "ns/op")
		set("postgres.compiled_lookup.bytes_op", "B/op")
	case "BenchmarkPostgresEndpointCheckIn":
		set("postgres.endpoint_checkin.ns_op", "ns/op")
	case "BenchmarkPostgresTelemetryWrite/state-report":
		set("postgres.telemetry_report.ns_op", "ns/op")
	case "BenchmarkPostgresTelemetryWrite/system-info-upsert":
		set("postgres.telemetry_inventory.ns_op", "ns/op")
	case "BenchmarkPostgresFleetReporting":
		set("postgres.fleet_report.ns_op", "ns/op")
	case "BenchmarkAgentFullCycleCompliant/resources=1000":
		set("agent.compliant_1000.ns_op", "ns/op")
		set("agent.compliant_1000.bytes_op", "B/op")
		set("agent.report_bytes", "report-bytes/op")
		set("agent.rollback_growth_bytes", "rollback-storage-bytes")
		set("agent.peak_rss_bytes", "peak-RSS-bytes")
		set("agent.goroutines", "goroutines")
	case "BenchmarkAgentFullCycleDriftedApply/resources=1000":
		set("agent.drifted_1000.ns_op", "ns/op")
		set("agent.drifted_1000.bytes_op", "B/op")
		set("agent.report_bytes", "report-bytes/op")
		set("agent.rollback_growth_bytes", "rollback-storage-bytes")
		set("agent.peak_rss_bytes", "peak-RSS-bytes")
		set("agent.goroutines", "goroutines")
	}
	return metrics
}
