// remotr-load drives opt-in authenticated Sync load against disposable infrastructure.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DavidHoenisch/remotr/internal/loadtest"
	"github.com/DavidHoenisch/remotr/internal/performance"
)

func main() {
	var cfg loadtest.Config
	allow := flag.Bool("allow-load", false, "confirm a disposable load environment")
	allowFaults := flag.Bool("allow-faults", false, "confirm controlled pause/unpause of a disposable Compose service")
	flag.StringVar(&cfg.ServerURL, "server-url", os.Getenv("REMOTR_LOAD_SERVER_URL"), "Remotr server URL")
	flag.StringVar(&cfg.CAPath, "ca", os.Getenv("REMOTR_LOAD_CA"), "CA certificate path")
	flag.StringVar(&cfg.DatabaseURL, "database-url", os.Getenv("REMOTR_LOAD_DATABASE_URL"), "disposable Postgres URL")
	flag.StringVar(&cfg.Fleet, "fleet", os.Getenv("REMOTR_LOAD_FLEET"), "preconfigured disposable fleet")
	flag.StringVar(&cfg.RunID, "run-id", os.Getenv("REMOTR_LOAD_RUN_ID"), "unique load-run prefix")
	flag.IntVar(&cfg.EndpointCount, "endpoints", 0, "number of unique endpoints")
	flag.IntVar(&cfg.Concurrency, "concurrency", 0, "maximum simultaneous Sync requests")
	flag.DurationVar(&cfg.RequestTimeout, "request-timeout", 30*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.EnrollmentTTL, "enrollment-ttl", time.Hour, "one-time enrollment token lifetime")
	steadyCycles := flag.Int("steady-cycles", 0, "unchanged Sync waves after one artifact warm-up")
	pollInterval := flag.Duration("poll-interval", 30*time.Second, "steady Sync polling interval")
	scenario := flag.String("scenario", "steady", "workload scenario: steady, soak, startup-reconnect, release-fanout, telemetry-heavy, capability-mixed, outage-recovery, policy-shaped-outage-recovery, or overload")
	composeFile := flag.String("compose-file", "", "disposable Compose file for fault or soak scenarios")
	faultService := flag.String("fault-service", "", "Compose service to pause for the outage-recovery scenario")
	growthLimitsPath := flag.String("growth-limits", "test/performance/budgets.json", "versioned performance budget JSON")
	diagnosticsService := flag.String("diagnostics-service", "remotr-server", "Compose service exposing loopback-only performance diagnostics")
	agentServices := flag.String("agent-services", "agent-debian,agent-arch", "comma-separated Compose agent services measured during soak")
	flag.Parse()

	if !*allow {
		fmt.Fprintln(os.Stderr, "refusing load: pass --allow-load only for disposable test infrastructure")
		os.Exit(2)
	}
	budgets, err := loadBudgets(*growthLimitsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load configuration: read performance budgets:", err)
		os.Exit(2)
	}
	harness, err := loadtest.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load configuration:", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := harness.Provision(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "provision load endpoints:", err)
		os.Exit(1)
	}
	defer harness.Close()
	var result loadtest.Report
	switch *scenario {
	case "steady":
		result, err = harness.MeasuredSteadyUnchanged(ctx, *steadyCycles, *pollInterval)
	case "soak":
		if *steadyCycles < 2 || strings.TrimSpace(*composeFile) == "" {
			fmt.Fprintln(os.Stderr, "load configuration: soak requires --steady-cycles >= 2 and --compose-file")
			os.Exit(2)
		}
		services := strings.Split(*agentServices, ",")
		for index := range services {
			services[index] = strings.TrimSpace(services[index])
		}
		probe := composeGrowthProbe{composeFile: *composeFile, diagnosticsService: *diagnosticsService, agentServices: services}
		result, err = harness.MeasuredSoak(ctx, *steadyCycles, *pollInterval, probe)
		if err == nil {
			growth, growthErr := performance.AnalyzeGrowth(result.GrowthSamples, budgets.SoakGrowth)
			if growthErr != nil {
				err = growthErr
			} else {
				result.Growth = &growth
			}
		}
	case "startup-reconnect":
		if *steadyCycles != 0 {
			fmt.Fprintln(os.Stderr, "load configuration: --steady-cycles is only valid for the steady scenario")
			os.Exit(2)
		}
		result, err = harness.MeasuredStartupReconnectRecovery(ctx)
	case "release-fanout":
		if *steadyCycles != 0 {
			fmt.Fprintln(os.Stderr, "load configuration: --steady-cycles is only valid for the steady scenario")
			os.Exit(2)
		}
		result, err = harness.MeasuredReleaseFanout(ctx)
	case "telemetry-heavy":
		if *steadyCycles != 0 {
			fmt.Fprintln(os.Stderr, "load configuration: --steady-cycles is only valid for the steady scenario")
			os.Exit(2)
		}
		result, err = harness.MeasuredTelemetryHeavy(ctx)
	case "capability-mixed":
		if *steadyCycles != 0 {
			fmt.Fprintln(os.Stderr, "load configuration: --steady-cycles is only valid for the steady scenario")
			os.Exit(2)
		}
		result, err = harness.MeasuredCapabilityMixed(ctx)
	case "outage-recovery":
		if !*allowFaults || strings.TrimSpace(*composeFile) == "" || strings.TrimSpace(*faultService) == "" {
			fmt.Fprintln(os.Stderr, "load configuration: outage-recovery requires --allow-faults, --compose-file, and --fault-service for disposable infrastructure")
			os.Exit(2)
		}
		result, err = harness.MeasuredOutageRecovery(ctx, composeFault{file: *composeFile, service: *faultService})
	case "policy-shaped-outage-recovery":
		if !*allowFaults || strings.TrimSpace(*composeFile) == "" || strings.TrimSpace(*faultService) == "" {
			fmt.Fprintln(os.Stderr, "load configuration: policy-shaped-outage-recovery requires --allow-faults, --compose-file, and --fault-service for disposable infrastructure")
			os.Exit(2)
		}
		result, err = harness.MeasuredShapedOutageRecovery(ctx, *pollInterval, composeFault{file: *composeFile, service: *faultService})
	case "overload":
		result, err = harness.MeasuredOverload(ctx)
	default:
		fmt.Fprintf(os.Stderr, "load configuration: unknown scenario %q\n", *scenario)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "measure load workload:", err)
		os.Exit(1)
	}
	if *scenario == "steady" || *scenario == "soak" {
		if budgetErr := checkSteadyLoadBudgets(result, budgets); budgetErr != nil {
			fmt.Fprintln(os.Stderr, "measure load workload:", budgetErr)
			os.Exit(1)
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "encode load summary:", err)
		os.Exit(1)
	}
	if !scenarioPassed(*scenario, result) {
		os.Exit(1)
	}
}

func checkSteadyLoadBudgets(result loadtest.Report, budgets performance.BudgetFile) error {
	if len(result.Waves) < 2 {
		return fmt.Errorf("steady load budget requires warmup and unchanged waves")
	}
	warmupP95 := float64(result.Waves[0].Summary.P95)
	var errors int
	var unchangedP95 time.Duration
	for index, wave := range result.Waves {
		errors += wave.Summary.Errors
		if index > 0 && wave.Summary.P95 > unchangedP95 {
			unchangedP95 = wave.Summary.P95
		}
	}
	checks := []struct {
		name  string
		value float64
	}{
		{"fleet.warmup.p95_ns", warmupP95},
		{"fleet.unchanged.p95_ns", float64(unchangedP95)},
		{"fleet.errors", float64(errors)},
		{"database.backends", float64(result.DatabaseAfter.Backends)},
		{"database.deadlocks", float64(result.DatabaseDelta.Deadlocks)},
	}
	if len(result.GrowthSamples) > 0 {
		var maxHeap, maxGoroutines int64
		for _, sample := range result.GrowthSamples {
			if sample.ServerHeapBytes > maxHeap {
				maxHeap = sample.ServerHeapBytes
			}
			if sample.ServerGoroutines > maxGoroutines {
				maxGoroutines = sample.ServerGoroutines
			}
		}
		checks = append(checks,
			struct {
				name  string
				value float64
			}{"server.heap_bytes", float64(maxHeap)},
			struct {
				name  string
				value float64
			}{"server.goroutines", float64(maxGoroutines)},
		)
		if len(result.GrowthSamples) > 1 {
			var maxCPUJiffiesPerWave int64
			for index := 1; index < len(result.GrowthSamples); index++ {
				delta := result.GrowthSamples[index].ServerCPUJiffies - result.GrowthSamples[index-1].ServerCPUJiffies
				if delta < 0 {
					return fmt.Errorf("server CPU counter moved backwards")
				}
				if delta > maxCPUJiffiesPerWave {
					maxCPUJiffiesPerWave = delta
				}
			}
			checks = append(checks, struct {
				name  string
				value float64
			}{"server.cpu_jiffies_per_wave", float64(maxCPUJiffiesPerWave)})
		}
	}
	for _, check := range checks {
		budget, ok := budgets.Metrics[check.name]
		if !ok {
			return fmt.Errorf("required load budget %q is missing", check.name)
		}
		if check.value > budget.Maximum {
			return fmt.Errorf("%s %.0f exceeds approved maximum %.0f %s", check.name, check.value, budget.Maximum, budget.Unit)
		}
	}
	return nil
}

type composeFault struct {
	file    string
	service string
}

func (f composeFault) Degrade(ctx context.Context) error {
	return f.run(ctx, "pause")
}

func (f composeFault) Recover(ctx context.Context) error {
	return f.run(ctx, "unpause")
}

func (f composeFault) run(ctx context.Context, action string) error {
	output, err := exec.CommandContext(ctx, "docker", "compose", "-f", f.file, action, f.service).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose %s %s: %w: %s", action, f.service, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func scenarioPassed(scenario string, result loadtest.Report) bool {
	switch scenario {
	case "soak":
		if result.Growth == nil || !result.Growth.Passed || len(result.GrowthSamples) < 3 || result.Growth.Samples != len(result.GrowthSamples) {
			return false
		}
		for _, wave := range result.Waves {
			if wave.Summary.Errors > 0 {
				return false
			}
		}
		return true
	case "overload":
		return len(result.Waves) == 1 && result.Waves[0].Summary.Overloaded > 0 && result.Waves[0].Summary.Overloaded == result.Waves[0].Summary.Errors
	case "outage-recovery":
		return len(result.Waves) == 3 && result.Waves[0].Summary.Errors == 0 && result.Waves[1].Summary.Errors > 0 && result.Waves[2].Summary.Errors == 0
	case "policy-shaped-outage-recovery":
		if len(result.Waves) != 3 || result.Waves[0].Summary.Errors != 0 || result.Waves[1].Summary.Errors == 0 || result.Waves[2].Summary.Errors != 0 {
			return false
		}
		for _, wave := range result.Waves {
			summary := wave.Summary
			if summary.Requests == 0 || summary.StartSpread < 250*time.Millisecond || summary.MaxStartsPer100ms >= (summary.Requests+3)/4 {
				return false
			}
		}
		return true
	case "capability-mixed":
		if len(result.Waves) != 4 || len(result.PopulationCounts) != 5 || result.DatabaseDelta.ArtifactVariantCount != 4 {
			return false
		}
		for _, wave := range result.Waves {
			if wave.Summary.Errors != 0 || wave.Summary.Successes != wave.Summary.Requests {
				return false
			}
		}
		target := result.Waves[2]
		if target.Name != "capability-mixed-target" || target.Summary.CapabilityBlocked == 0 || target.Summary.Unmanaged == 0 {
			return false
		}
		compatible := target.Populations["compatible"]
		blocked := target.Populations["blocked-existing"]
		unmanaged := target.Populations["unmanaged-new"]
		telemetry := target.Populations["telemetry-carrying"]
		reconnecting := target.Populations["reconnecting"]
		if compatible.CapabilityBlocked != 0 || reconnecting.CapabilityBlocked != 0 ||
			blocked.CapabilityBlocked != blocked.Requests || blocked.Unmanaged != 0 ||
			unmanaged.CapabilityBlocked != unmanaged.Requests || unmanaged.Unmanaged != unmanaged.Requests ||
			telemetry.CapabilityBlocked != telemetry.Requests || telemetry.Unmanaged != 0 {
			return false
		}
		reconnectWave := result.Waves[3]
		return reconnectWave.Name == "capability-reconnect" && reconnectWave.Summary.Unchanged == reconnectWave.Summary.Requests && reconnectWave.Summary.StartSpread > 0
	default:
		for _, wave := range result.Waves {
			if wave.Summary.Errors > 0 {
				return false
			}
		}
		return true
	}
}
