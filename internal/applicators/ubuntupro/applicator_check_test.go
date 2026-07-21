package ubuntupro

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

type providerCheckRunner struct {
	outputs map[string][]byte
	errors  map[string]error
	calls   []string
}

type contextAwareCheckRunner struct {
	providerCheckRunner
	contextCalls int
}

type deadlineOnlyContext struct{ context.Context }

func (deadlineOnlyContext) Deadline() (time.Time, bool) {
	return time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC), true
}

func (runner *contextAwareCheckRunner) RunContext(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	runner.contextCalls++
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return runner.providerCheckRunner.Run(name, args...)
}

func (runner *providerCheckRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	if name != proExecutable || len(args) != 2 || args[0] != "api" {
		return nil, nil, fmt.Errorf("unexpected process boundary %s %v", name, args)
	}
	endpoint := args[1]
	runner.calls = append(runner.calls, endpoint)
	if err := runner.errors[endpoint]; err != nil {
		return nil, nil, err
	}
	output, ok := runner.outputs[endpoint]
	if !ok {
		return nil, nil, fmt.Errorf("unexpected endpoint %s", endpoint)
	}
	return append([]byte(nil), output...), nil, nil
}

func failureEnvelope(code, message string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{},"meta":{"environment_vars":[]},"type":"ErrorResult"},"errors":[{"code":%q,"msg":%q,"meta":{}}],"result":"failure","version":"32.3ubuntu0","warnings":[]}`, code, message))
}

func exactUbuntuFacts() facts.Facts {
	return facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", OSID: "ubuntu", OSReleaseSourceCount: 2,
		OSReleaseConsistent: true, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt,
	}
}

func attachmentEnvelope(attached bool) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"is_attached":%t},"meta":{"environment_vars":[]},"type":"IsAttachedResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attached))
}

// OS-UPM-014, OS-UPM-031, and OS-UPM-038: public Check reports only a
// bounded attachment enum and derives compliance from the versioned API.
func TestApplicatorCheckReportsAttachmentState(t *testing.T) {
	for _, test := range []struct {
		name       string
		attached   bool
		wantStatus executor.CheckStatus
		wantReason executor.ReasonCode
	}{
		{name: "attached", attached: true, wantStatus: executor.Compliant, wantReason: executor.ReasonCompliant},
		{name: "unattached", attached: false, wantStatus: executor.Drifted, wantReason: executor.ReasonStateDrift},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &providerCheckRunner{outputs: map[string][]byte{isAttachedEndpoint: attachmentEnvelope(test.attached)}}
			resource := models.UbuntuProResource{
				ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
				Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
			}
			result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
			if err := result.Validate(); err != nil {
				t.Fatalf("Check() returned invalid result: %v", err)
			}
			if result.Status != test.wantStatus || result.ReasonCode != test.wantReason {
				t.Fatalf("Check() = %s/%s, want %s/%s", result.Status, result.ReasonCode, test.wantStatus, test.wantReason)
			}
			report, ok := result.Actual.(StateReport)
			if !ok || report.Attachment != AttachmentState(map[bool]string{true: "attached", false: "unattached"}[test.attached]) {
				t.Fatalf("Check() report = %#v", result.Actual)
			}
			if len(report.Services) != 0 || len(report.WarningCodes) != 0 || report.ContractHealth != "" || report.Entitlement != "" || report.LastOutcome != "" || report.RollbackClass != "" || report.ResidualEffects != "" || report.RebootRequired != "" {
				t.Fatalf("Check() leaked undeclared or unexpected state: %#v", report)
			}
			if len(runner.calls) != 1 || runner.calls[0] != isAttachedEndpoint {
				t.Fatalf("Check() endpoints = %v", runner.calls)
			}
		})
	}
}

// OS-UPM-015, OS-UPM-028, OS-UPM-029, and OS-UPM-032: stable API codes are
// classified without copying localized messages, stderr, or malformed output.
func TestApplicatorCheckClassifiesBoundedProbeFailures(t *testing.T) {
	const diagnosticCanary = "ubuntu-pro-sensitive-localized-diagnostic-canary"
	tests := []struct {
		name       string
		output     []byte
		processErr error
		wantStatus executor.CheckStatus
		wantReason executor.ReasonCode
		wantHealth ContractHealth
	}{
		{name: "invalid contract", output: failureEnvelope("invalid-contract", diagnosticCanary), wantStatus: executor.CheckFailed, wantReason: "ubuntu_pro_contract_invalid", wantHealth: ContractInvalid},
		{name: "expired contract", output: failureEnvelope("expired-contract", diagnosticCanary), wantStatus: executor.CheckFailed, wantReason: "ubuntu_pro_contract_expired", wantHealth: ContractExpired},
		{name: "native lock", output: failureEnvelope("operation-in-progress", diagnosticCanary), wantStatus: executor.Deferred, wantReason: executor.ReasonNativeLockContended},
		{name: "malformed state", output: []byte(`{"raw":"` + diagnosticCanary + `"}`), wantStatus: executor.CheckFailed, wantReason: executor.ReasonProbeFailed},
		{name: "process failure", processErr: errors.New(diagnosticCanary), wantStatus: executor.CheckFailed, wantReason: executor.ReasonProbeFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &providerCheckRunner{
				outputs: map[string][]byte{isAttachedEndpoint: test.output},
				errors:  map[string]error{isAttachedEndpoint: test.processErr},
			}
			resource := models.UbuntuProResource{
				ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
				Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
			}
			result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
			if err := result.Validate(); err != nil {
				t.Fatalf("Check() returned invalid result: %v", err)
			}
			if result.Status != test.wantStatus || result.ReasonCode != test.wantReason {
				t.Fatalf("Check() = %s/%s (%v), want %s/%s", result.Status, result.ReasonCode, result.Err, test.wantStatus, test.wantReason)
			}
			report, hasReport := result.Actual.(StateReport)
			if (test.wantHealth == "" && result.Actual != nil) || (test.wantHealth != "" && (!hasReport || report.ContractHealth != test.wantHealth)) || strings.Contains(fmt.Sprint(result.Err), diagnosticCanary) || strings.Contains(string(result.ObservedSummary), diagnosticCanary) {
				t.Fatalf("Check() exposed probe diagnostic: %#v", result)
			}
			if len(runner.calls) != 1 || runner.calls[0] != isAttachedEndpoint {
				t.Fatalf("Check() endpoints = %v", runner.calls)
			}
		})
	}
}

// OS-UPM-021, OS-UPM-031, and OS-UPM-032: a successful observation with a
// warning is unhealthy, and its localized message is not reportable state.
func TestApplicatorCheckReportsStableWarnings(t *testing.T) {
	const warningCanary = "ubuntu-pro-localized-warning-message-canary"
	output := strings.Replace(
		string(attachmentEnvelope(true)),
		`"warnings":[]`,
		`"warnings":[{"code":"contract-warning","msg":"`+warningCanary+`","meta":{}}]`,
		1,
	)
	runner := &providerCheckRunner{outputs: map[string][]byte{isAttachedEndpoint: []byte(output)}}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
	}
	result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.CheckFailed || result.ReasonCode != "ubuntu_pro_warning" {
		t.Fatalf("Check() = %s/%s (%v), want check_failed/ubuntu_pro_warning", result.Status, result.ReasonCode, result.Err)
	}
	report, ok := result.Actual.(StateReport)
	if !ok || report.Attachment != AttachmentAttached || len(report.WarningCodes) != 1 || report.WarningCodes[0] != "contract-warning" {
		t.Fatalf("Check() report = %#v", result.Actual)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), warningCanary) {
		t.Fatalf("Check() exposed localized warning: %#v", result)
	}
}

func enabledServicesEnvelope(names ...string) []byte {
	services := make([]string, 0, len(names))
	for _, name := range names {
		services = append(services, fmt.Sprintf(`{"name":%q,"variant_enabled":false,"variant_name":null}`, name))
	}
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"enabled_services":[%s]},"meta":{"environment_vars":[]},"type":"EnabledServicesResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, strings.Join(services, ",")))
}

// OS-UPM-031, OS-UPM-032, and OS-UPM-042: Check projects only declared
// service state and normalizes the documented cis status alias to usg.
func TestApplicatorCheckReportsOnlyDeclaredServices(t *testing.T) {
	runner := &providerCheckRunner{outputs: map[string][]byte{
		isAttachedEndpoint:      attachmentEnvelope(true),
		enabledServicesEndpoint: enabledServicesEnvelope("cis", "esm-apps"),
	}}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
		Services: []models.UbuntuProService{
			{Name: "usg", State: models.UbuntuProServiceEnabled},
			{Name: "esm-infra", State: models.UbuntuProServiceDisabled},
		},
	}
	result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.Compliant || result.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("Check() = %s/%s (%v), want compliant", result.Status, result.ReasonCode, result.Err)
	}
	report, ok := result.Actual.(StateReport)
	wantServices := []ServiceState{{Name: "usg", Enabled: true}, {Name: "esm-infra", Enabled: false}}
	if !ok || fmt.Sprint(report.Services) != fmt.Sprint(wantServices) {
		t.Fatalf("Check() report = %#v, want declared services %#v", result.Actual, wantServices)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), "esm-apps") {
		t.Fatalf("Check() exposed undeclared service: %#v", result)
	}
	if fmt.Sprint(runner.calls) != fmt.Sprint([]string{isAttachedEndpoint, enabledServicesEndpoint}) {
		t.Fatalf("Check() endpoints = %v", runner.calls)
	}
}

// OS-UPM-020 and OS-UPM-032: stable service availability and entitlement
// failures are bounded unsupported outcomes rather than compliant state.
func TestApplicatorCheckClassifiesServiceAvailability(t *testing.T) {
	const diagnosticCanary = "ubuntu-pro-service-diagnostic-canary"
	for _, test := range []struct {
		name        string
		code        string
		wantReason  executor.ReasonCode
		wantOutcome EntitlementOutcome
	}{
		{name: "unavailable", code: "service-unavailable", wantReason: "ubuntu_pro_service_unavailable", wantOutcome: EntitlementUnavailable},
		{name: "unentitled", code: "service-not-entitled", wantReason: "ubuntu_pro_service_unentitled", wantOutcome: EntitlementUnentitled},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &providerCheckRunner{outputs: map[string][]byte{
				isAttachedEndpoint:      attachmentEnvelope(true),
				enabledServicesEndpoint: failureEnvelope(test.code, diagnosticCanary),
			}}
			resource := models.UbuntuProResource{
				ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
				Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
				Services: []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}},
			}
			result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
			if err := result.Validate(); err != nil {
				t.Fatalf("Check() returned invalid result: %v", err)
			}
			if result.Status != executor.Unsupported || result.ReasonCode != test.wantReason {
				t.Fatalf("Check() = %s/%s (%v), want unsupported/%s", result.Status, result.ReasonCode, result.Err, test.wantReason)
			}
			report, ok := result.Actual.(StateReport)
			if !ok || report.Attachment != AttachmentAttached || len(report.Services) != 0 || report.Entitlement != test.wantOutcome {
				t.Fatalf("Check() report = %#v", result.Actual)
			}
			if strings.Contains(fmt.Sprintf("%#v", result), diagnosticCanary) {
				t.Fatalf("Check() exposed service diagnostic: %#v", result)
			}
		})
	}
}

// OS-UPM-030: cancellation reaches the process boundary without a wall-clock
// sleep and is reported using a stable, redacted reason.
func TestApplicatorCheckHonorsCancellation(t *testing.T) {
	runner := &contextAwareCheckRunner{}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := executor.Check(ctx, New(resource, exactUbuntuFacts(), runner, nil))
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.CheckFailed || result.ReasonCode != "ubuntu_pro_canceled" || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("Check() = %s/%s (%v), want check_failed/ubuntu_pro_canceled", result.Status, result.ReasonCode, result.Err)
	}
	if runner.contextCalls != 1 || len(runner.calls) != 0 {
		t.Fatalf("process calls = context:%d legacy:%v", runner.contextCalls, runner.calls)
	}
}

// OS-UPM-030: a caller deadline fails closed when the runner cannot enforce
// process cancellation; the legacy process boundary must not be entered.
func TestApplicatorCheckRequiresContextRunnerForDeadline(t *testing.T) {
	runner := &providerCheckRunner{outputs: map[string][]byte{isAttachedEndpoint: attachmentEnvelope(true)}}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
	}
	ctx := deadlineOnlyContext{Context: context.Background()}
	result := executor.Check(ctx, New(resource, exactUbuntuFacts(), runner, nil))
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.CheckFailed || result.ReasonCode != "ubuntu_pro_context_runner_required" {
		t.Fatalf("Check() = %s/%s (%v), want check_failed/ubuntu_pro_context_runner_required", result.Status, result.ReasonCode, result.Err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("legacy process boundary was entered: %v", runner.calls)
	}
}

// OS-UPM-030: the injected timeout boundary makes deadline behavior fully
// deterministic and maps expiry to a stable, redacted Check failure.
func TestApplicatorCheckHonorsInjectedTimeout(t *testing.T) {
	runner := &contextAwareCheckRunner{}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
	}
	applicator := New(resource, exactUbuntuFacts(), runner, nil)
	var requestedTimeout time.Duration
	applicator.api.withTimeout = func(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
		requestedTimeout = timeout
		return context.WithDeadline(parent, time.Unix(0, 0))
	}
	result := executor.Check(context.Background(), applicator)
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.CheckFailed || result.ReasonCode != "ubuntu_pro_timeout" || !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("Check() = %s/%s (%v), want check_failed/ubuntu_pro_timeout", result.Status, result.ReasonCode, result.Err)
	}
	if requestedTimeout != apiProcessTimeout || runner.contextCalls != 1 || len(runner.calls) != 0 {
		t.Fatalf("timeout boundary = %s, context calls = %d, legacy calls = %v", requestedTimeout, runner.contextCalls, runner.calls)
	}
}

// OS-UPM-060 and OS-LPC-022: an invocation option cannot be treated as
// converged when the versioned status API exposes only the enabled name.
func TestApplicatorCheckRejectsUnobservableAccessOnlyMode(t *testing.T) {
	runner := &providerCheckRunner{outputs: map[string][]byte{
		isAttachedEndpoint:      attachmentEnvelope(true),
		enabledServicesEndpoint: enabledServicesEnvelope("esm-apps"),
	}}
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
		Services: []models.UbuntuProService{{
			Name: "esm-apps", State: models.UbuntuProServiceEnabled, EnableMode: models.UbuntuProEnableAccessOnly,
		}},
	}
	result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.Unsupported || result.ReasonCode != "ubuntu_pro_mode_unobservable" {
		t.Fatalf("Check() = %s/%s (%v), want unsupported/ubuntu_pro_mode_unobservable", result.Status, result.ReasonCode, result.Err)
	}
	report, ok := result.Actual.(StateReport)
	if !ok || len(report.Services) != 1 || report.Services[0].Name != "esm-apps" || !report.Services[0].Enabled {
		t.Fatalf("Check() report = %#v", result.Actual)
	}
}

// OS-UPM-043, OS-UPM-045, and OS-UPM-060: explicit full mode is also a mode
// assertion and cannot be inferred from the enabled service name alone.
func TestApplicatorCheckRejectsUnobservableExplicitFullMode(t *testing.T) {
	runner := &providerCheckRunner{outputs: map[string][]byte{
		isAttachedEndpoint:      attachmentEnvelope(true),
		enabledServicesEndpoint: enabledServicesEnvelope("esm-apps"),
	}}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{
		Name: "esm-apps", State: models.UbuntuProServiceEnabled, EnableMode: models.UbuntuProEnableFull,
	}}
	result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	if err := result.Validate(); err != nil {
		t.Fatalf("Check() returned invalid result: %v", err)
	}
	if result.Status != executor.Unsupported || result.ReasonCode != "ubuntu_pro_mode_unobservable" {
		t.Fatalf("Check() = %s/%s (%v), want unsupported/ubuntu_pro_mode_unobservable", result.Status, result.ReasonCode, result.Err)
	}
}
