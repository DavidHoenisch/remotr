package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestEngineRejectsMismatchedExecutionLeaseHashBeforeProvider(t *testing.T) {
	enforce := true
	now := time.Date(2035, 1, 2, 3, 4, 5, 0, time.UTC)
	const currentHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const staleHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	handler := &countingRiskHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/firewall", Name: "firewall", Kind: engine.KindFirewall,
		Provider: "nftables", ProviderRevision: "firewall-v1", EffectiveHash: currentHash,
		Risk: models.RiskConnectivity, Enforce: &enforce, Handler: handler,
	}}, nil, engine.WithExecutionLeases([]changecontrol.ExecutionLease{{
		ID: "lease-1", ChangeRequestID: "change-1", EndpointID: "endpoint-1",
		HashContractVersion: 1,
		ResourceHashes:      map[string]string{"base/firewall": staleHash},
		IssuedAt:            now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}}), engine.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if result.Failed == nil || result.Failed.Address != "base/firewall" || result.Failed.Err.ReasonCode != "hash_mismatch" {
		t.Fatalf("ApplyAll() = %+v, want canonical hash rejection", result)
	}
	if handler.preflights != 0 || handler.applies != 0 {
		t.Fatalf("provider ran before hash rejection: preflights=%d applies=%d", handler.preflights, handler.applies)
	}
}

// OS-AEC-086: current endpoint plan evidence includes a non-enforcing
// high-risk provider preflight without applying the resource.
func TestEngineCheckAllReportsNonEnforcingHighRiskPreflight(t *testing.T) {
	handler := &countingRiskHandler{executionHandler: executionHandler{check: executor.CheckResult{
		Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
	}}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/sudo", Name: "sudo", Kind: engine.KindSudo,
		Provider: "sudo", ProviderRevision: "sudo-v1",
		EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Risk:          models.RiskAccess, Handler: handler,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	report := eng.CheckAll(t.Context())
	if handler.preflights != 1 {
		t.Fatalf("preflight calls = %d, want one non-enforcing check", handler.preflights)
	}
	if len(report.Items) != 1 || report.Items[0].PreflightStatus != engine.PreflightReady {
		t.Fatalf("plan preflight evidence = %+v", report.Items)
	}
	if handler.applies != 0 {
		t.Fatalf("non-enforcing Check applied resource %d time(s)", handler.applies)
	}
}

func TestEngineCheckAllClassifiesPreflightFailureWithoutProviderText(t *testing.T) {
	const canary = "preflight-provider-secret-canary"
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/sudo", Name: "sudo", Kind: engine.KindSudo,
		Provider: "sudo", ProviderRevision: "sudo-v1",
		EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Risk:          models.RiskAccess,
		Handler: riskPreflightHandler{
			executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
			preflightErr:     errors.New(canary),
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	report := eng.CheckAll(t.Context())
	if len(report.Items) != 1 || report.Items[0].PreflightStatus != engine.PreflightBlocked || report.Items[0].PreflightReason != executor.ReasonPreflightFailed {
		t.Fatalf("blocked plan preflight evidence = %+v", report.Items)
	}
	if strings.Contains(fmt.Sprintf("%+v", report), canary) {
		t.Fatalf("preflight report retained provider canary: %+v", report)
	}
}

// OS-AEC-087: normal transactional dependencies participate in the same
// non-enforcing safety evidence as their high-risk dependents.
func TestEngineCheckAllBlocksHighRiskDependentWhenRollbackReservationFails(t *testing.T) {
	const canary = "rollback-capacity-secret-canary"
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{
			Address: "base/config", Name: "config", Kind: engine.KindFile,
			Provider: "file", ProviderRevision: "file-v1",
			EffectiveHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Risk:          models.RiskNormal, RollbackClass: executor.RollbackTransactional,
			Handler: rollbackPreflightHandler{
				executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
				err:              errors.New(canary),
			},
		},
		{
			Address: "base/sudo", Name: "sudo", Kind: engine.KindSudo,
			Provider: "sudo", ProviderRevision: "sudo-v1",
			EffectiveHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			DependsOn:     []string{"base/config"}, Risk: models.RiskAccess,
			Handler: riskPreflightHandler{executionHandler: executionHandler{check: executor.CheckResult{
				Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
			}}},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	report := eng.CheckAll(t.Context())
	if len(report.Items) != 2 || report.Items[0].Address != "base/config" || report.Items[0].PreflightStatus != engine.PreflightBlocked || report.Items[0].PreflightReason != executor.ReasonRollbackReservationFailed {
		t.Fatalf("dependency preflight evidence = %+v", report.Items)
	}
	if report.Items[1].Address != "base/sudo" || report.Items[1].PreflightStatus != engine.PreflightBlocked || report.Items[1].PreflightReason != executor.ReasonDependencyBlocked {
		t.Fatalf("dependent preflight evidence = %+v", report.Items)
	}
	if strings.Contains(fmt.Sprintf("%+v", report), canary) {
		t.Fatalf("dependency report retained provider canary: %+v", report)
	}
}

type countingRiskHandler struct {
	executionHandler
	preflights int
	applies    int
}

func (h *countingRiskHandler) Preflight(context.Context) error {
	h.preflights++
	return nil
}

func (h *countingRiskHandler) Apply(context.Context) error {
	h.applies++
	return nil
}

// OS-AEC-074: provider-controlled desired/observed text, diagnostics, and
// errors must be converted at the execution boundary before reports, logs, or
// authenticated Sync telemetry can retain them.
func TestEngineAndSyncRejectArbitraryProviderCanary(t *testing.T) {
	const canary = "provider-boundary-secret-canary"
	handler := activationHandler{
		executionHandler: executionHandler{check: executor.CheckResult{
			Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
			DesiredSummary: canary, ObservedSummary: canary,
			Subresults: []executor.CheckSubresult{{
				Target: "target", Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
				DesiredSummary: canary, ObservedSummary: canary,
			}},
		}},
		result: executor.ApplyResult{
			Status: executor.Failed, RebootRequired: executor.RebootNotRequired,
			RollbackClass: executor.RollbackNone, Diagnostics: []executor.RedactedSummary{canary},
			Err: errors.New(canary),
		},
	}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/managed", Name: "managed", Kind: engine.KindFile, Handler: handler,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	drift := eng.CheckAll(t.Context())
	applied := eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if applied.Failed == nil {
		t.Fatal("expected controlled provider failure")
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	logger.Error("pipeline failed", "err", applied.Failed.Err)

	var pending agentsync.Pending
	pending.SetFromPipeline(nil, drift, applied, applied.Failed, "sha256:test")
	wire, err := json.Marshal(pending.Request("", "", "test"))
	if err != nil {
		t.Fatal(err)
	}
	for sink, output := range map[string]string{
		"engine report": fmt.Sprintf("%+v %+v", drift, applied),
		"agent log":     logs.String(),
		"sync payload":  string(wire),
	} {
		if strings.Contains(output, canary) {
			t.Fatalf("%s retained provider canary: %s", sink, output)
		}
	}
}

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

// OS-SRM-006: failed actions retain bounded provider/unit/operation/exit
// diagnostics while raw stderr (which may contain secrets) stays redacted.
func TestEngineReportsServiceActionFailureWithoutLeakingStderr(t *testing.T) {
	const canary = "service-action-secret-canary"
	runner := failedServiceActionRunner{stderr: strings.Repeat("diagnostic ", 100) + canary, exitCode: 7}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "cfg/config", Name: "config", Kind: engine.KindFile,
		Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
			Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
			Activation: []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "telemetry.service"}},
		}},
	}}, runner)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if result.Failed == nil || result.Failed.Address != "activation" {
		t.Fatalf("ApplyAll() = %+v", result)
	}
	safeError := result.Failed.Err
	if safeError.ReasonCode != "activation_failed" || safeError.Operation != "activate" ||
		!strings.Contains(safeError.Details.String(), "provider=systemd") ||
		!strings.Contains(safeError.Details.String(), "unit=telemetry.service") ||
		!strings.Contains(safeError.Details.String(), "operation=restart") ||
		!strings.Contains(safeError.Details.String(), "exitStatus=7") {
		t.Fatalf("safe service action error = %+v", safeError)
	}
	if strings.Contains(safeError.Error(), canary) || len(safeError.Error()) > 512 {
		t.Fatalf("unsafe service diagnostic = %q", safeError.Error())
	}
}

// OS-AEC-066: an invalid structured activation from a provider remains a
// bounded agent-execution failure; it must not panic or reach a host command.
func TestEngineReportsUnknownActivationAsFailure(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "cfg/config", Name: "config", Kind: engine.KindFile,
		Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
			Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
			Activation: []executor.ActivationSignal{
				{Kind: executor.ActivationKind("unknown")},
				{Kind: executor.ActivationLogoutRequired},
			},
		}},
	}}, runner)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if result.Failed == nil || result.Failed.Address != "activation" || result.Failed.Err.ReasonCode != "activation_failed" {
		t.Fatalf("ApplyAll() = %+v", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("unknown activation executed commands: %+v", runner.Calls)
	}
}

// OS-SRM-007: reboot-required is reported as activation evidence; the generic
// activation boundary must not translate it into a reboot command.
func TestEngineReportsRebootRequiredWithoutExecutingReboot(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/packages/kernel", Name: "kernel", Kind: engine.KindPackage,
		Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
			Status: executor.Changed, RebootRequired: executor.RebootRequired, RollbackClass: executor.RollbackNone,
			Activation: []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}},
		}},
	}}, runner)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if result.Failed != nil || len(result.Items) != 1 || result.Items[0].RebootRequired != executor.RebootRequired || len(runner.Calls) != 0 {
		t.Fatalf("ApplyAll() = %+v; commands = %+v", result, runner.Calls)
	}
}

func TestEngineReconcilesPackageRebootRequirementAsObservableActivation(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "base/packages/kernel", Name: "kernel", Kind: engine.KindPackage, Provider: "apt",
		Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
			Status: executor.Changed, RebootRequired: executor.RebootRequired, RollbackClass: executor.RollbackNone,
		}},
	}}, runner)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
	want := []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}
	if result.Failed != nil || !slices.Equal(result.Activations, want) || len(result.Items) != 1 {
		t.Fatalf("ApplyAll() = %+v, want one observable reboot activation", result)
	}
	if result.Items[0].RebootRequired != executor.RebootRequired || !slices.Equal(result.Items[0].Activation, want) {
		t.Fatalf("package Apply item = %+v, want reconciled reboot requirement", result.Items[0])
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("reboot requirement executed implicit process actions: %#v", runner.Calls)
	}
}

// OS-AEC-029: every non-normal risk class is non-enforcing by default and
// can run only with explicit enforcement plus a successful preflight.
func TestEngineAppliesRiskyResourcesOnlyAfterPreflight(t *testing.T) {
	drift := executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}
	enforce := true
	now := time.Date(2035, 2, 3, 4, 5, 6, 0, time.UTC)
	const effectiveHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	leaseOptions := []engine.Option{
		engine.WithClock(func() time.Time { return now }),
		engine.WithExecutionLeases([]changecontrol.ExecutionLease{{
			ID: "lease-risky", ChangeRequestID: "change-risky", EndpointID: "endpoint-risky",
			HashContractVersion: 1,
			ResourceHashes:      map[string]string{"cfg/risky": effectiveHash},
			IssuedAt:            now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
		}}),
	}

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
			ProviderRevision: "command-v1", EffectiveHash: effectiveHash,
			Handler: riskPreflightHandler{executionHandler: executionHandler{check: drift}},
		}}, nil, leaseOptions...)
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
			ProviderRevision: "command-v1", EffectiveHash: effectiveHash,
			Handler: executionHandler{check: drift, applyErr: errors.New("Apply must not run")},
		}}, nil, leaseOptions...)
		if err != nil {
			t.Fatal(err)
		}
		result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
		if result.Failed == nil || result.Failed.Address != "cfg/risky" || len(result.Applied) != 0 {
			t.Fatalf("ApplyAll() = %+v, want preflight failure", result)
		}
	})
}

func TestEngineRejectsEnforcedHighRiskResourceWithoutActiveLease(t *testing.T) {
	enforce := true
	handler := &countingRiskHandler{executionHandler: executionHandler{check: executor.CheckResult{
		Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
	}}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "cfg/sudo", Name: "sudo", Kind: engine.KindSudo,
		ProviderRevision: "sudo-v1",
		EffectiveHash:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Risk:             models.RiskAccess,
		Enforce:          &enforce,
		Handler:          handler,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if result.Failed == nil || result.Failed.Address != "cfg/sudo" || result.Failed.Err.ReasonCode != "authorization_required" {
		t.Fatalf("ApplyAll() = %+v, want authorization-required failure", result)
	}
	if handler.preflights != 0 || handler.applies != 0 {
		t.Fatalf("provider ran without an active lease: preflights=%d applies=%d", handler.preflights, handler.applies)
	}
}

func TestEngineOrdersEveryInfrastructureTierBeforeArbitraryCommands(t *testing.T) {
	kinds := []engine.Kind{
		engine.KindHostname,
		engine.KindAuthorizedKey,
		engine.KindKnownHost,
		engine.KindSudo,
		engine.KindHostsEntry,
		engine.KindDNSResolver,
		engine.KindRoute,
		engine.KindNetworkProfile,
		engine.KindService,
		engine.KindSystemdUnit,
		engine.KindEndpointSchedule,
	}
	resources := make([]engine.ExecutionResource, 0, len(kinds)+1)
	for _, kind := range kinds {
		resources = append(resources, engine.ExecutionResource{
			Address: "cfg/" + string(kind), Name: string(kind), Kind: kind,
			Handler: executionHandler{check: executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}},
		})
	}
	resources = append(resources, engine.ExecutionResource{
		Address: "cfg/command", Name: "command", Kind: engine.KindCommand,
		Handler: executionHandler{check: executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}},
	})

	eng, err := engine.NewForExecution(resources, nil)
	if err != nil {
		t.Fatal(err)
	}
	order := eng.NodeOrder()
	if eng.NodeCount() != len(resources) || len(order) != len(resources) {
		t.Fatalf("engine nodes = %d, order = %v, want every declared resource", eng.NodeCount(), order)
	}
	if order[len(order)-1] != "cfg/command" {
		t.Fatalf("NodeOrder() = %v, want arbitrary command last", order)
	}
}

func TestEngineReportsStableReasonCodesForEveryApplyOutcome(t *testing.T) {
	tests := []struct {
		name   string
		result executor.ApplyResult
		want   executor.ReasonCode
	}{
		{
			name: "already compliant",
			result: executor.ApplyResult{
				Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
			},
			want: "already_compliant",
		},
		{
			name: "provider-specific deferral",
			result: executor.ApplyResult{
				Status: executor.ApplyDeferred, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
				DeferredWork: &executor.DeferredWork{ReasonCode: executor.ReasonDependencyBlocked},
			},
			want: executor.ReasonDependencyBlocked,
		},
		{
			name: "unclassified deferral",
			result: executor.ApplyResult{
				Status: executor.ApplyDeferred, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
			},
			want: executor.ReasonDeferred,
		},
		{
			name: "unknown provider outcome",
			result: executor.ApplyResult{
				Status: executor.ApplyStatus("unexpected"), RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
			},
			want: "apply_unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := engine.NewForExecution([]engine.ExecutionResource{{
				Address: "cfg/resource", Name: "resource", Kind: engine.KindFile,
				Handler: activationHandler{
					executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
					result:           tt.result,
				},
			}}, nil)
			if err != nil {
				t.Fatal(err)
			}

			result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
			if len(result.Items) != 1 || result.Items[0].ReasonCode != tt.want {
				t.Fatalf("ApplyAll() items = %+v, want reason %q", result.Items, tt.want)
			}
		})
	}
}

func TestEngineHonorsProviderNativeLockLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		acquireErr   error
		nilRelease   bool
		wantFailure  bool
		wantApply    int
		wantReleases int
	}{
		{name: "acquisition failure", acquireErr: errors.New("provider lock unavailable"), wantFailure: true},
		{name: "provider has no release work", nilRelease: true, wantApply: 1},
		{name: "provider release runs after apply", wantApply: 1, wantReleases: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &nativeLockHandler{
				executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
				acquireErr:       tt.acquireErr,
				nilRelease:       tt.nilRelease,
			}
			eng, err := engine.NewForExecution([]engine.ExecutionResource{{
				Address: "cfg/package", Name: "package", Kind: engine.KindPackage,
				LockDomains: []string{"package-database"}, Handler: handler,
			}}, nil)
			if err != nil {
				t.Fatal(err)
			}

			result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
			if tt.wantFailure {
				if result.Failed == nil || result.Failed.Err.ReasonCode != "lock_acquisition_failed" {
					t.Fatalf("ApplyAll() = %+v, want lock acquisition failure", result)
				}
			} else if result.Failed != nil {
				t.Fatalf("ApplyAll() = %+v, want success", result)
			}
			if handler.acquisitions != 1 || handler.applies != tt.wantApply || handler.releases != tt.wantReleases {
				t.Fatalf("native lock lifecycle = acquire:%d apply:%d release:%d, want acquire:1 apply:%d release:%d", handler.acquisitions, handler.applies, handler.releases, tt.wantApply, tt.wantReleases)
			}
		})
	}
}

func TestEngineBoundsAndSanitizesPackageLockFailures(t *testing.T) {
	newEngine := func(t *testing.T, coordinator executor.LockCoordinator, handler *nativeLockHandler) *engine.Engine {
		t.Helper()
		eng, err := engine.NewForExecution([]engine.ExecutionResource{{
			Address: "cfg/package", Name: "package", Kind: engine.KindPackage, Provider: "apt", Handler: handler,
		}}, nil, engine.WithLockCoordinator(coordinator))
		if err != nil {
			t.Fatal(err)
		}
		return eng
	}

	t.Run("caller cancellation", func(t *testing.T) {
		handler := &nativeLockHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}}
		coordinator := lockCoordinatorFunc(func(ctx context.Context, _ []string) (func(), error) {
			return nil, ctx.Err()
		})
		eng := newEngine(t, coordinator, handler)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		result := eng.ApplyAll(ctx, engine.PolicyAuto)
		if result.Failed == nil || result.Failed.Err.ReasonCode != executor.ReasonLockCanceled || !result.Failed.Err.Canceled {
			t.Fatalf("ApplyAll() = %+v, want typed canceled lock failure", result)
		}
		if handler.acquisitions != 0 || handler.applies != 0 {
			t.Fatalf("provider ran after cancellation: acquire=%d apply=%d", handler.acquisitions, handler.applies)
		}
	})

	t.Run("bounded coordinator timeout", func(t *testing.T) {
		handler := &nativeLockHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}}
		deadlineObserved := false
		coordinator := lockCoordinatorFunc(func(ctx context.Context, _ []string) (func(), error) {
			_, deadlineObserved = ctx.Deadline()
			if !deadlineObserved {
				return nil, errors.New("lock acquisition context has no deadline")
			}
			return nil, context.DeadlineExceeded
		})
		result := newEngine(t, coordinator, handler).ApplyAll(t.Context(), engine.PolicyAuto)
		if !deadlineObserved || result.Failed == nil || result.Failed.Err.ReasonCode != executor.ReasonLockTimeout || !result.Failed.Err.Canceled {
			t.Fatalf("ApplyAll() = %+v, deadline=%t, want bounded typed timeout", result, deadlineObserved)
		}
		if handler.acquisitions != 0 || handler.applies != 0 {
			t.Fatalf("provider ran after coordinator timeout: acquire=%d apply=%d", handler.acquisitions, handler.applies)
		}
	})

	t.Run("provider native contention", func(t *testing.T) {
		const canary = "native-lock-provider-secret-canary"
		handler := &nativeLockHandler{
			executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
			acquireErr:       fmt.Errorf("%w: %s", executor.ErrNativeLockContended, canary),
		}
		releases := 0
		coordinator := lockCoordinatorFunc(func(ctx context.Context, _ []string) (func(), error) {
			if _, ok := ctx.Deadline(); !ok {
				return nil, errors.New("native lock context has no deadline")
			}
			return func() { releases++ }, nil
		})
		result := newEngine(t, coordinator, handler).ApplyAll(t.Context(), engine.PolicyAuto)
		if result.Failed == nil || result.Failed.Err.ReasonCode != executor.ReasonNativeLockContended || result.Failed.Err.Canceled {
			t.Fatalf("ApplyAll() = %+v, want typed native contention", result)
		}
		if handler.acquisitions != 1 || !handler.deadlineObserved || handler.applies != 0 || releases != 1 {
			t.Fatalf("contention lifecycle = acquire:%d deadline:%t apply:%d domain-releases:%d", handler.acquisitions, handler.deadlineObserved, handler.applies, releases)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), canary) || strings.Contains(fmt.Sprintf("%+v", result), canary) {
			t.Fatalf("safe lock failure retained provider canary: %s", encoded)
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

// OS-PRM-010: native package mutations receive mandatory provider-aware lock
// domains from the engine. Pacman and AUR share one database boundary while
// APT uses its distinct native package-manager boundary.
func TestEngineSerializesPackageMutationsInProviderLockDomain(t *testing.T) {
	tests := []struct {
		name           string
		firstProvider  string
		secondProvider string
		wantDomain     string
	}{
		{name: "APT resources", firstProvider: "apt", secondProvider: "apt", wantDomain: "package-manager:apt"},
		{name: "Pacman and AUR resources", firstProvider: "pacman", secondProvider: "yay", wantDomain: "package-manager:pacman"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coordinator := &observingLockCoordinator{
				delegate: executor.NewLockManager(),
				attempts: make(chan []string, 2),
			}
			firstStarted, firstRelease := make(chan struct{}), make(chan struct{})
			secondStarted, secondRelease := make(chan struct{}), make(chan struct{})
			eng, err := engine.NewForExecution([]engine.ExecutionResource{
				{
					Address: "cfg/package/first", Name: "first", Kind: engine.KindPackage, Provider: tt.firstProvider,
					Handler: selectedBlockingApplyHandler{selection: "first", started: firstStarted, release: firstRelease},
				},
				{
					Address: "cfg/package/second", Name: "second", Kind: engine.KindPackage, Provider: tt.secondProvider,
					Handler: selectedBlockingApplyHandler{selection: "second", started: secondStarted, release: secondRelease},
				},
			}, nil, engine.WithLockCoordinator(coordinator))
			if err != nil {
				t.Fatal(err)
			}

			firstDone := make(chan engine.ApplyResult, 1)
			go func() {
				ctx := context.WithValue(t.Context(), packageRunSelectionKey{}, "first")
				firstDone <- eng.ApplyAll(ctx, engine.PolicyAuto)
			}()
			firstDomains := <-coordinator.attempts
			<-firstStarted

			secondDone := make(chan engine.ApplyResult, 1)
			go func() {
				ctx := context.WithValue(t.Context(), packageRunSelectionKey{}, "second")
				secondDone <- eng.ApplyAll(ctx, engine.PolicyAuto)
			}()
			secondDomains := <-coordinator.attempts

			wantDomains := []string{tt.wantDomain}
			if !slices.Equal(firstDomains, wantDomains) || !slices.Equal(secondDomains, wantDomains) {
				t.Fatalf("package lock domains = first:%v second:%v, want %v", firstDomains, secondDomains, wantDomains)
			}
			select {
			case <-secondStarted:
				t.Fatal("second package mutation started while the shared native lock was held")
			default:
			}

			close(firstRelease)
			first := <-firstDone
			if first.Failed != nil || !slices.Equal(first.Applied, []string{"cfg/package/first"}) {
				t.Fatalf("first ApplyAll() = %+v", first)
			}

			<-secondStarted
			close(secondRelease)
			second := <-secondDone
			if second.Failed != nil || !slices.Equal(second.Applied, []string{"cfg/package/second"}) {
				t.Fatalf("second ApplyAll() = %+v", second)
			}
		})
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
	if len(result.Items) != 2 || result.Items[0].RebootRequired != executor.RebootRequired {
		t.Fatalf("reboot activation was not reconciled into its producing Apply item: %+v", result.Items)
	}
}

// OS-SRM-002, OS-SRM-005: service actions run only after every producing
// resource succeeds, with identical targets coalesced and ordered by action.
func TestEngineRunsCoalescedServiceActionsAfterSuccessfulProducers(t *testing.T) {
	applied := 0
	runner := &serviceActionRunner{applied: &applied, wantApplied: 2}
	changed := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{
			Address: "cfg/first-config", Name: "first-config", Kind: engine.KindFile,
			Handler: countedActivationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, applied: &applied, result: withActivations(changed,
				executor.ActivationSignal{Kind: executor.ActivationRestart, Target: "telemetry.service"},
				executor.ActivationSignal{Kind: executor.ActivationTryRestart, Target: "collector.service"},
				executor.ActivationSignal{Kind: executor.ActivationDaemonReload},
			)},
		},
		{
			Address: "cfg/second-config", Name: "second-config", Kind: engine.KindFile, DependsOn: []string{"cfg/first-config"},
			Handler: countedActivationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, applied: &applied, result: withActivations(changed,
				executor.ActivationSignal{Kind: executor.ActivationRestart, Target: "telemetry.service"},
				executor.ActivationSignal{Kind: executor.ActivationReload, Target: "auditd.service"},
			)},
		},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	want := []executil.MockCall{
		{Name: "systemctl", Args: []string{"daemon-reload"}},
		{Name: "systemctl", Args: []string{"reload", "auditd.service"}},
		{Name: "systemctl", Args: []string{"try-restart", "collector.service"}},
		{Name: "systemctl", Args: []string{"restart", "telemetry.service"}},
	}
	if result.Failed != nil || !slices.EqualFunc(runner.calls, want, func(a, b executil.MockCall) bool { return a.Name == b.Name && slices.Equal(a.Args, b.Args) }) {
		t.Fatalf("ApplyAll() = %+v; service actions = %#v, want %#v", result, runner.calls, want)
	}
}

func TestEngineRunsOneDistroNativeTrustRefreshAfterAllAnchorChanges(t *testing.T) {
	applied := 0
	runner := &serviceActionRunner{applied: &applied, wantApplied: 2}
	changed := executor.ApplyResult{
		Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort,
		Activation: []executor.ActivationSignal{{Kind: executor.ActivationTrustStoreRefresh, Target: "debian"}},
	}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{Address: "security/corporate", Name: "corporate", Kind: engine.KindTrustAnchor, Handler: countedActivationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, applied: &applied, result: changed}},
		{Address: "security/partner", Name: "partner", Kind: engine.KindTrustAnchor, Handler: countedActivationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, applied: &applied, result: changed}},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	want := []executil.MockCall{{Name: "update-ca-certificates"}}
	if result.Failed != nil || !slices.EqualFunc(runner.calls, want, func(a, b executil.MockCall) bool { return a.Name == b.Name && slices.Equal(a.Args, b.Args) }) {
		t.Fatalf("ApplyAll() = %+v; trust refresh calls = %#v, want %#v", result, runner.calls, want)
	}
}

func TestEngineUsesArchNativeTrustRefreshCommand(t *testing.T) {
	applied := 0
	runner := &serviceActionRunner{applied: &applied, wantApplied: 1}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{{
		Address: "security/corporate", Name: "corporate", Kind: engine.KindTrustAnchor,
		Handler: countedActivationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, applied: &applied, result: executor.ApplyResult{
			Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort,
			Activation: []executor.ActivationSignal{{Kind: executor.ActivationTrustStoreRefresh, Target: "arch"}},
		}},
	}}, runner)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	want := []executil.MockCall{{Name: "trust", Args: []string{"extract-compat"}}}
	if result.Failed != nil || !slices.EqualFunc(runner.calls, want, func(a, b executil.MockCall) bool { return a.Name == b.Name && slices.Equal(a.Args, b.Args) }) {
		t.Fatalf("ApplyAll() = %+v; trust refresh calls = %#v, want %#v", result, runner.calls, want)
	}
}

func TestEngineDoesNotRunQueuedServiceActionsAfterProducerFailure(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{"systemctl [restart telemetry.service]": {}}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{
			Address: "cfg/changed", Name: "changed", Kind: engine.KindFile,
			Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
				Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
				Activation: []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "telemetry.service"}},
			}},
		},
		{
			Address: "cfg/failed", Name: "failed", Kind: engine.KindFile, DependsOn: []string{"cfg/changed"},
			Handler: activationHandler{executionHandler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}}, result: executor.ApplyResult{
				Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: errors.New("validation failed"),
			}},
		},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	result := eng.ApplyAll(context.Background(), engine.PolicyAuto)
	if result.Failed == nil || result.Failed.Address != "cfg/failed" || len(runner.Calls) != 0 {
		t.Fatalf("ApplyAll() = %+v; premature actions = %#v", result, runner.Calls)
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

func TestEngineDoesNotRefreshAPTMetadataForOrdinaryDependencies(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{}}
	packageHandler := &cacheRefreshHandler{executionHandler: executionHandler{check: executor.CheckResult{
		Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
	}}}
	eng, err := engine.NewForExecution([]engine.ExecutionResource{
		{
			Address: "base/package-config", Name: "package-config", Kind: engine.KindFile,
			Handler: executionHandler{check: executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}},
		},
		{
			Address: "base/package", Name: "package", Kind: engine.KindPackage,
			DependsOn: []string{"base/package-config"}, Handler: packageHandler,
		},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}

	result := eng.ApplyAll(t.Context(), engine.PolicyAuto)
	if result.Failed != nil || !slices.Equal(result.Applied, []string{"base/package-config", "base/package"}) {
		t.Fatalf("ApplyAll() = %+v", result)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("ordinary dependency refreshed APT metadata: %#v", runner.Calls)
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

type rollbackPreflightHandler struct {
	executionHandler
	err error
}

func (h rollbackPreflightHandler) PreflightRollback(context.Context) error { return h.err }

type blockingApplyHandler struct {
	executionHandler
	started chan<- struct{}
	release <-chan struct{}
}

type packageRunSelectionKey struct{}

type selectedBlockingApplyHandler struct {
	selection string
	started   chan<- struct{}
	release   <-chan struct{}
}

func (h selectedBlockingApplyHandler) Name() string        { return "controlled-package" }
func (h selectedBlockingApplyHandler) Description() string { return "controlled package handler" }
func (h selectedBlockingApplyHandler) State(ctx context.Context) (any, bool) {
	return nil, ctx.Value(packageRunSelectionKey{}) != h.selection
}
func (h selectedBlockingApplyHandler) Check(ctx context.Context) executor.CheckResult {
	if _, compliant := h.State(ctx); compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift}
}
func (h selectedBlockingApplyHandler) Apply(ctx context.Context) error {
	close(h.started)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (selectedBlockingApplyHandler) Revert(context.Context) error { return appErr.ErrNoOp }

type observingLockCoordinator struct {
	delegate *executor.LockManager
	attempts chan []string
}

type lockCoordinatorFunc func(context.Context, []string) (func(), error)

func (f lockCoordinatorFunc) Acquire(ctx context.Context, domains []string) (func(), error) {
	return f(ctx, domains)
}

func (c *observingLockCoordinator) Acquire(ctx context.Context, domains []string) (func(), error) {
	c.attempts <- append([]string(nil), domains...)
	return c.delegate.Acquire(ctx, domains)
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

type nativeLockHandler struct {
	executionHandler
	acquireErr       error
	nilRelease       bool
	acquisitions     int
	applies          int
	releases         int
	deadlineObserved bool
}

func (h *nativeLockHandler) AcquireNativeLocks(ctx context.Context) (func(), error) {
	h.acquisitions++
	_, h.deadlineObserved = ctx.Deadline()
	if h.acquireErr != nil {
		return nil, h.acquireErr
	}
	if h.nilRelease {
		return nil, nil
	}
	return func() { h.releases++ }, nil
}

func (h *nativeLockHandler) Apply(context.Context) error {
	h.applies++
	return nil
}

type activationHandler struct {
	executionHandler
	result executor.ApplyResult
}

func (h activationHandler) ApplyResult(context.Context) executor.ApplyResult { return h.result }

type countedActivationHandler struct {
	executionHandler
	applied *int
	result  executor.ApplyResult
}

func (h countedActivationHandler) ApplyResult(context.Context) executor.ApplyResult {
	*h.applied++
	return h.result
}

func withActivations(result executor.ApplyResult, signals ...executor.ActivationSignal) executor.ApplyResult {
	result.Activation = append([]executor.ActivationSignal(nil), signals...)
	return result
}

type serviceActionRunner struct {
	applied     *int
	wantApplied int
	calls       []executil.MockCall
}

type failedServiceActionRunner struct {
	stderr   string
	exitCode int
}

func (r failedServiceActionRunner) Run(string, ...string) ([]byte, []byte, error) {
	return nil, []byte(r.stderr), codedActionError(r.exitCode)
}

type codedActionError int

func (e codedActionError) Error() string { return "service action failed" }
func (e codedActionError) ExitCode() int { return int(e) }

func (r *serviceActionRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if *r.applied != r.wantApplied {
		return nil, nil, fmt.Errorf("service action ran after %d producers, want %d", *r.applied, r.wantApplied)
	}
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	return nil, nil, nil
}

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
