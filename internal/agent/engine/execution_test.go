package engine_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-AEC-012, OS-AEC-013: only drifted, policy-permitted, dependency-ready,
// and preflight-passing resources reach Apply.
func TestEngineApplyAllSkipsIneligibleResources(t *testing.T) {
	checkFailure := errors.New("probe unavailable")
	preflightFailure := errors.New("validator rejected staged state")
	tests := []struct {
		name        string
		resources   []engine.ExecutionResource
		policy      engine.Policy
		runner      executil.Runner
		wantApplied []string
		wantSkipped []string
		wantFailure string
	}{
		{
			name: "compliant resource",
			resources: []engine.ExecutionResource{{
				Address: "cfg/compliant", Name: "compliant", Kind: engine.KindCommand,
				Handler: executionHandler{check: executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}},
			}},
		},
		{
			name: "unsupported resource",
			resources: []engine.ExecutionResource{{
				Address: "cfg/unsupported", Name: "unsupported", Kind: engine.KindCommand,
				Handler: executionHandler{check: executor.CheckResult{Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable}, applyErr: errors.New("Apply must not run")},
			}},
			wantSkipped: []string{"cfg/unsupported"},
		},
		{
			name: "check failure",
			resources: []engine.ExecutionResource{{
				Address: "cfg/check-failed", Name: "check-failed", Kind: engine.KindCommand,
				Handler: executionHandler{check: executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, Err: checkFailure}, applyErr: errors.New("Apply must not run")},
			}},
			wantSkipped: []string{"cfg/check-failed"},
		},
		{
			name: "deferred resource",
			resources: []engine.ExecutionResource{{
				Address: "cfg/deferred", Name: "deferred", Kind: engine.KindCommand,
				Handler: executionHandler{check: executor.CheckResult{Status: executor.Deferred, ReasonCode: executor.ReasonDeferred}, applyErr: errors.New("Apply must not run")},
			}},
			wantSkipped: []string{"cfg/deferred"},
		},
		{
			name: "report policy",
			resources: []engine.ExecutionResource{{
				Address: "cfg/report-only", Name: "report-only", Kind: engine.KindCommand,
				Handler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}, applyErr: errors.New("Apply must not run")},
			}},
			policy:      engine.PolicyReport,
			wantSkipped: []string{"cfg/report-only"},
		},
		{
			name: "blocked dependency",
			resources: []engine.ExecutionResource{
				{Address: "cfg/dependency", Name: "dependency", Kind: engine.KindCommand, Handler: executionHandler{check: executor.CheckResult{Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable}, applyErr: errors.New("Apply must not run")}},
				{Address: "cfg/dependent", Name: "dependent", Kind: engine.KindCommand, DependsOn: []string{"cfg/dependency"}, Handler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}, applyErr: errors.New("Apply must not run")}},
			},
			wantSkipped: []string{"cfg/dependency", "cfg/dependent"},
		},
		{
			name: "failed preflight",
			resources: []engine.ExecutionResource{{
				Address: "cfg/preflight", Name: "preflight", Kind: engine.KindCommand,
				PreApplyValidation: []string{"validate"},
				Handler:            executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}, applyErr: errors.New("Apply must not run")},
			}},
			runner:      &executil.MockRunner{Next: map[string]executil.MockResult{"validate []": {Err: preflightFailure}}},
			wantFailure: "cfg/preflight",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := engine.NewForExecution(tt.resources, tt.runner)
			if err != nil {
				t.Fatal(err)
			}
			result := eng.ApplyAll(context.Background(), tt.policy)
			if !slices.Equal(result.Applied, tt.wantApplied) || !slices.Equal(result.Skipped, tt.wantSkipped) {
				t.Fatalf("ApplyAll() = %+v, want applied=%v skipped=%v", result, tt.wantApplied, tt.wantSkipped)
			}
			if tt.wantFailure == "" && result.Failed != nil {
				t.Fatalf("ApplyAll() failure = %+v", result.Failed)
			}
			if tt.wantFailure != "" && (result.Failed == nil || result.Failed.Address != tt.wantFailure) {
				t.Fatalf("ApplyAll() failure = %+v, want %q", result.Failed, tt.wantFailure)
			}
		})
	}
}

// OS-AEC-029: every non-normal risk class is non-enforcing by default and
// can run only with explicit enforcement plus a successful preflight.
func TestEngineAppliesRiskyResourcesOnlyAfterPreflight(t *testing.T) {
	drift := executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}
	enforce := true

	for _, risk := range []models.RiskClass{
		models.RiskSensitive,
		models.RiskConnectivity,
		models.RiskAccess,
		models.RiskBoot,
		models.RiskDestructive,
	} {
		t.Run(string(risk)+" defaults to non-enforcing", func(t *testing.T) {
			eng, err := engine.NewForExecution([]engine.ExecutionResource{{
				Address: "cfg/risky", Name: "risky", Kind: engine.KindCommand, Risk: risk,
				Handler: executionHandler{check: drift, applyErr: errors.New("Apply must not run")},
			}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
			if !slices.Equal(result.Skipped, []string{"cfg/risky"}) || len(result.Applied) != 0 || result.Failed != nil {
				t.Fatalf("ApplyAll() = %+v, want safe default skip", result)
			}
		})
	}

	t.Run("explicitly enforced resource with preflight applies", func(t *testing.T) {
		eng, err := engine.NewForExecution([]engine.ExecutionResource{{
			Address: "cfg/risky", Name: "risky", Kind: engine.KindCommand, Risk: models.RiskConnectivity, Enforce: &enforce,
			Handler: riskPreflightHandler{executionHandler: executionHandler{check: drift}},
		}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
		if !slices.Equal(result.Applied, []string{"cfg/risky"}) || result.Failed != nil {
			t.Fatalf("ApplyAll() = %+v, want applied risky resource", result)
		}
	})

	t.Run("explicit enforcement without preflight remains blocked", func(t *testing.T) {
		eng, err := engine.NewForExecution([]engine.ExecutionResource{{
			Address: "cfg/risky", Name: "risky", Kind: engine.KindCommand, Risk: models.RiskConnectivity, Enforce: &enforce,
			Handler: executionHandler{check: drift, applyErr: errors.New("Apply must not run")},
		}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
		if result.Failed == nil || result.Failed.Address != "cfg/risky" || len(result.Applied) != 0 {
			t.Fatalf("ApplyAll() = %+v, want preflight failure", result)
		}
	})
}

// OS-AEC-065: resources sharing a lock domain must not apply concurrently,
// and a cancelled lock wait must return without starting the second Apply.
func TestEngineSerializesSharedLockDomains(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "cfg/package", Name: "package", Kind: engine.KindPackage,
		LockDomains: []string{"package-database"},
		Handler: blockingApplyHandler{
			executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
			started:          started,
			release:          release,
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan engine.ApplyResult, 1)
	go func() { firstDone <- eng.ApplyAll(context.Background(), engine.PolicyAuto) }()
	<-started

	secondCtx, cancel := context.WithCancel(context.Background())
	cancel()
	second := eng.ApplyAll(secondCtx, engine.PolicyAuto)
	if second.Failed == nil || !errors.Is(second.Failed.Err, context.Canceled) || len(second.Applied) != 0 {
		t.Fatalf("second ApplyAll() = %+v, want cancelled lock wait", second)
	}

	close(release)
	first := <-firstDone
	if !slices.Equal(first.Applied, []string{"cfg/package"}) || first.Failed != nil {
		t.Fatalf("first ApplyAll() = %+v, want one applied resource", first)
	}
}

// OS-AEC-066: successful resource results produce one ordered, deduplicated
// activation plan that crosses the controlled activation boundary once.
func TestEngineCollectsAndExecutesOrderedActivationSignals(t *testing.T) {
	recorder := &activationRecorder{}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{
			Address: "cfg/first", Name: "first", Kind: engine.KindPackage,
			Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
				Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort,
				Activation: []executor.ActivationSignal{
					{Kind: executor.ActivationRestart, Target: "example.service"},
					{Kind: executor.ActivationDaemonReload},
					{Kind: executor.ActivationRebootRequired},
				},
			}},
		},
		{
			Address: "cfg/second", Name: "second", Kind: engine.KindCommand, DependsOn: []string{"cfg/first"},
			Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
				Status: executor.Changed, RebootRequired: executor.RebootRequired, RollbackClass: executor.RollbackTransactional,
				Activation: []executor.ActivationSignal{
					{Kind: executor.ActivationDaemonReload},
					{Kind: executor.ActivationReload, Target: "example.service"},
					{Kind: executor.ActivationLogoutRequired},
					{Kind: executor.ActivationNextBoot},
				},
			}},
		},
	}, nil, engine.WithActivator(recorder))
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	want := []executor.ActivationSignal{
		{Kind: executor.ActivationDaemonReload},
		{Kind: executor.ActivationReload, Target: "example.service"},
		{Kind: executor.ActivationRestart, Target: "example.service"},
		{Kind: executor.ActivationLogoutRequired},
		{Kind: executor.ActivationNextBoot},
		{Kind: executor.ActivationRebootRequired},
	}
	if !slices.Equal(result.Activations, want) || !slices.Equal(recorder.signals, want) || result.Failed != nil {
		t.Fatalf("ApplyAll() = %+v; activation boundary = %v; want %v", result, recorder.signals, want)
	}
}

// OS-ESM-009: local execution failures are runtime evidence, not
// configuration drift when the installed schedule still matches.
func TestEngineReportsScheduleRuntimeSeparatelyFromCompliance(t *testing.T) {
	exitCode := 23
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/nightly-backup", Name: "nightly-backup", Kind: engine.KindEndpointSchedule,
		Handler: scheduleRuntimeHandler{
			executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}},
			runtime: executor.ScheduleRuntimeTelemetry{
				Status:            executor.ScheduleRunFailed,
				ExitCode:          &exitCode,
				MissedRunBehavior: executor.ScheduleMissedRunCatchUp,
			},
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	report := eng.CheckAll(context.Background())
	if !report.InCompliance || len(report.Items) != 1 || report.Items[0].Status != executor.Compliant {
		t.Fatalf("configuration report = %+v, want compliant", report)
	}
	if len(report.ScheduleRuntime) != 1 {
		t.Fatalf("schedule runtime = %+v, want one separate result", report.ScheduleRuntime)
	}
	runtime := report.ScheduleRuntime[0]
	if runtime.Address != "base/nightly-backup" || runtime.Status != executor.ScheduleRunFailed || runtime.ExitCode == nil || *runtime.ExitCode != exitCode {
		t.Fatalf("schedule runtime = %+v", runtime)
	}
}

// OS-PRM-014: explicit key/repository dependencies order a single metadata
// refresh before every dependent APT package transaction.
func TestEngineCoalescesAPTRefreshAfterRepositoryDependencies(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"apt-get [update]": {},
	}}
	first := &cacheRefreshHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}}
	second := &cacheRefreshHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{Address: "base/vendor-key", Name: "vendor-key", Kind: engine.KindAPTSigningKey, Handler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}},
		{Address: "base/vendor-repository", Name: "vendor-repository", Kind: engine.KindAPTRepository, DependsOn: []string{"base/vendor-key"}, Handler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}},
		{Address: "base/first-package", Name: "first-package", Kind: engine.KindPackage, DependsOn: []string{"base/vendor-repository"}, Handler: first},
		{Address: "base/second-package", Name: "second-package", Kind: engine.KindPackage, DependsOn: []string{"base/vendor-repository"}, Handler: second},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if result.Failed != nil || !slices.Equal(result.Applied, []string{"base/vendor-key", "base/vendor-repository", "base/first-package", "base/second-package"}) {
		t.Fatalf("ApplyAll() = %+v", result)
	}
	updates := 0
	for _, call := range runner.Calls {
		if call.Name == "apt-get" && slices.Equal(call.Args, []string{"update"}) {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("apt metadata refreshes = %d, want exactly one; calls=%#v", updates, runner.Calls)
	}
}

type executionHandler struct {
	check    executor.CheckResult
	applyErr error
}

func (executionHandler) Name() string        { return "controlled" }
func (executionHandler) Description() string { return "controlled handler" }
func (h executionHandler) State(context.Context) (any, bool) {
	return nil, h.check.Status == executor.Compliant
}
func (h executionHandler) Check(context.Context) executor.CheckResult { return h.check }
func (h executionHandler) Apply(context.Context) error                { return h.applyErr }
func (executionHandler) Revert(context.Context) error                 { return appErr.ErrNoOp }

type cacheRefreshHandler struct {
	executionHandler
	refresh func(context.Context) error
}

func (h *cacheRefreshHandler) SetCacheRefresh(refresh func(context.Context) error) {
	h.refresh = refresh
}
func (h *cacheRefreshHandler) RefreshCache(ctx context.Context) error {
	if h.refresh == nil {
		return nil
	}
	return h.refresh(ctx)
}

type riskPreflightHandler struct {
	executionHandler
	preflightErr error
}

func (h riskPreflightHandler) Preflight(context.Context) error { return h.preflightErr }

type blockingApplyHandler struct {
	executionHandler
	started chan<- struct{}
	release <-chan struct{}
}

func (h blockingApplyHandler) Apply(ctx context.Context) error {
	close(h.started)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type activationHandler struct {
	executionHandler
	result executor.ApplyResult
}

func (h activationHandler) ApplyResult(context.Context) executor.ApplyResult { return h.result }

type scheduleRuntimeHandler struct {
	executionHandler
	runtime executor.ScheduleRuntimeTelemetry
}

func (h scheduleRuntimeHandler) ScheduleRuntime(context.Context) (executor.ScheduleRuntimeTelemetry, bool) {
	return h.runtime, true
}

type activationRecorder struct {
	signals []executor.ActivationSignal
}

func (r *activationRecorder) Activate(_ context.Context, signals []executor.ActivationSignal) error {
	r.signals = append([]executor.ActivationSignal(nil), signals...)
	return nil
}
