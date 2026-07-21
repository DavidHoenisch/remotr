//go:build vmsafety

package ubuntupro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

const ubuntuProSyntheticCanary = "remotr-synthetic-ubuntu-pro-token-canary"

type ubuntuProVMAPIDouble struct {
	attached  bool
	readCalls []string
	input     []byte
}

func (runner *ubuntuProVMAPIDouble) Run(name string, args ...string) ([]byte, []byte, error) {
	return runner.RunContext(context.Background(), name, args...)
}

func (runner *ubuntuProVMAPIDouble) RunContext(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if name != proExecutable || len(args) != 2 || args[0] != "api" {
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro read boundary: %s %v", name, args)
	}
	runner.readCalls = append(runner.readCalls, args[1])
	switch args[1] {
	case isAttachedEndpoint:
		return ubuntuProVMAttachmentEnvelope(runner.attached), nil, nil
	case detachEndpoint:
		runner.attached = false
		return ubuntuProVMDetachEnvelope(), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro API endpoint: %s", args[1])
	}
}

func (runner *ubuntuProVMAPIDouble) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.RunInputContext(context.Background(), name, input, args...)
}

func (runner *ubuntuProVMAPIDouble) RunInputContext(ctx context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	want := []string{"api", fullTokenAttachEndpoint, "--data", "-"}
	if name != proExecutable || !slices.Equal(args, want) {
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro mutation boundary: %s %v", name, args)
	}
	var request struct {
		Token              string `json:"token"`
		AutoEnableServices bool   `json:"auto_enable_services"`
	}
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, nil, fmt.Errorf("decode protected attach request: %w", err)
	}
	if request.Token != ubuntuProSyntheticCanary || request.AutoEnableServices {
		return nil, nil, fmt.Errorf("unexpected protected attach request")
	}
	runner.input = append([]byte(nil), input...)
	runner.attached = true
	return ubuntuProVMAttachEnvelope(), nil, nil
}

func ubuntuProVMAttachmentEnvelope(attached bool) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"is_attached":%t},"meta":{"environment_vars":[]},"type":"IsAttachedResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attached))
}

func ubuntuProVMAttachEnvelope() []byte {
	return []byte(`{"_schema_version":"v1","data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
}

func ubuntuProVMDetachEnvelope() []byte {
	return []byte(`{"_schema_version":"v1","data":{"attributes":{"disabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"DetachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
}

func ubuntuProVMFacts(t *testing.T) facts.Facts {
	t.Helper()
	release := os.Getenv("REMOTR_UBUNTU_PRO_VM_RELEASE")
	if release == "" {
		t.Fatal("REMOTR_UBUNTU_PRO_VM_RELEASE is required")
	}
	endpoint, err := facts.Read()
	if err != nil {
		t.Fatalf("read guest facts: %v", err)
	}
	if !endpoint.ExactUbuntu() || endpoint.DistroVersion != release || endpoint.Arch != types.X86 || endpoint.Package != types.Apt {
		t.Fatalf("guest facts = %+v, want exact Ubuntu %s amd64 with APT", endpoint, release)
	}
	return endpoint
}

func ubuntuProVMResolver(t *testing.T, calls *int) TokenResolver {
	t.Helper()
	path := os.Getenv("REMOTR_UBUNTU_PRO_TOKEN_FILE")
	return func(ctx context.Context, reference string) ([]byte, error) {
		*calls++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if reference != "remotr:ubuntu-pro/vm@active" {
			return nil, fmt.Errorf("unexpected token reference")
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat synthetic token: %w", err)
		}
		if info.Mode().Perm() != 0o600 {
			return nil, fmt.Errorf("synthetic token mode = %o, want 600", info.Mode().Perm())
		}
		material, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read synthetic token: %w", err)
		}
		return bytes.TrimSpace(material), nil
	}
}

// OS-UPM-010, OS-UPM-011, OS-UPM-014, OS-UPM-024, OS-UPM-025, and
// OS-UPM-027: a real pinned Ubuntu guest exercises the public provider while a
// deterministic API double prevents any Canonical subscription consumption.
func TestUbuntuProProviderContractVM(t *testing.T) {
	endpoint := ubuntuProVMFacts(t)
	runner := &ubuntuProVMAPIDouble{}
	resolverCalls := 0
	resource := models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "vm-subscription", TokenRef: "remotr:ubuntu-pro/vm@active",
	}
	applicator := New(resource, endpoint, runner, ubuntuProVMResolver(t, &resolverCalls))
	first := executor.New().Apply(context.Background(), applicator)
	if first.Status != executor.Changed || resolverCalls != 1 || !runner.attached {
		t.Fatalf("first Apply() = %+v, resolver calls = %d, attached = %t", first, resolverCalls, runner.attached)
	}
	check := executor.Check(context.Background(), applicator)
	if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("Check() = %+v", check)
	}
	second := executor.New().Apply(context.Background(), applicator)
	if second.Status != executor.NoChange || resolverCalls != 1 {
		t.Fatalf("second Apply() = %+v, resolver calls = %d", second, resolverCalls)
	}
	detached := resource
	detached.Lifecycle = models.UbuntuProDetached
	detached.TokenRef = ""
	detach := executor.New().Apply(context.Background(), New(detached, endpoint, runner, nil))
	if detach.Status != executor.Changed || runner.attached {
		t.Fatalf("detach Apply() = %+v, attached = %t", detach, runner.attached)
	}
	if !bytes.Contains(runner.input, []byte(ubuntuProSyntheticCanary)) {
		t.Fatal("protected stdin did not carry the synthetic canary")
	}
	for _, result := range []any{first, check, second, detach} {
		if strings.Contains(fmt.Sprintf("%+v", result), ubuntuProSyntheticCanary) {
			t.Fatalf("public result exposed synthetic token: %+v", result)
		}
	}
}

type ubuntuProVMServiceCase struct {
	name    string
	service string
	variant string
	risk    models.RiskClass
}

func ubuntuProVMServiceCases() []ubuntuProVMServiceCase {
	var cases []ubuntuProVMServiceCase
	for _, contract := range models.UbuntuProServiceCatalog() {
		cases = append(cases, ubuntuProVMServiceCase{name: contract.Name, service: contract.Name, risk: contract.EnableRisk})
		for _, variant := range contract.Variants {
			cases = append(cases, ubuntuProVMServiceCase{
				name: contract.Name + "-variant-" + variant, service: contract.Name, variant: variant, risk: contract.EnableRisk,
			})
		}
	}
	return cases
}

func ubuntuProVMSelectsService(selector string, test ubuntuProVMServiceCase) bool {
	if selector == "" || strings.Contains(selector, ".") {
		return true
	}
	if strings.HasPrefix(selector, "service-") {
		return strings.HasPrefix(selector, "service-"+test.service+"-") && test.variant == ""
	}
	if strings.HasPrefix(selector, "variant-") {
		return strings.HasPrefix(selector, "variant-"+test.service+"-"+test.variant+"-") && test.variant != ""
	}
	if strings.HasPrefix(selector, "enable-mode-") {
		return strings.HasPrefix(selector, "enable-mode-"+test.service+"-full-") && test.variant == ""
	}
	return false
}

// OS-UPM-017 through OS-UPM-024 and OS-UPM-042 through OS-UPM-053: every
// catalog service and variant converges independently through the public
// provider seam on a real pinned Ubuntu guest. The native effects remain an
// API double; this test qualifies provider control flow, not entitlement.
func TestUbuntuProServiceMatrixVM(t *testing.T) {
	endpoint := ubuntuProVMFacts(t)
	selector := os.Getenv("REMOTR_UBUNTU_PRO_SELECTOR")
	run := 0
	for _, test := range ubuntuProVMServiceCases() {
		if !ubuntuProVMSelectsService(selector, test) {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			run++
			observed := enabledServicesEnvelope(test.service)
			if test.variant != "" {
				observed = enabledVariantEnvelope(test.service, test.variant)
			}
			transition := serviceTransitionEnvelope([]string{test.service}, nil)
			if test.risk == models.RiskBoot {
				transition = serviceTransitionRebootEnvelope([]string{test.service}, nil)
			}
			runner := &serviceLifecycleRunner{
				readOutputs: map[string][][]byte{
					isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true), attachmentEnvelope(true), attachmentEnvelope(true)},
					enabledServicesEndpoint: {enabledServicesEnvelope(), observed, observed, observed},
				},
				inputOutputs: map[string][][]byte{enableEndpoint: {transition}},
			}
			resource := models.UbuntuProResource{
				ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
				Name:         "vm-service-" + test.name,
				TokenRef:     "remotr:ubuntu-pro/vm@active",
				Services: []models.UbuntuProService{{
					Name: test.service, State: models.UbuntuProServiceEnabled, Variant: test.variant,
				}},
			}
			applicator := New(resource, endpoint, runner, nil)
			first := executor.New().Apply(context.Background(), applicator)
			if first.Status != executor.Changed {
				t.Fatalf("first Apply() = %+v", first)
			}
			if test.risk == models.RiskBoot && first.RebootRequired != executor.RebootRequired {
				t.Fatalf("boot-impact Apply() = %+v, want reboot signal", first)
			}
			check := executor.Check(context.Background(), applicator)
			if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
				t.Fatalf("Check() = %+v", check)
			}
			second := executor.New().Apply(context.Background(), applicator)
			if second.Status != executor.NoChange {
				t.Fatalf("second Apply() = %+v", second)
			}
			if !slices.Equal(runner.inputCalls, []string{enableEndpoint}) || len(runner.inputs) != 1 {
				t.Fatalf("mutation endpoints = %v, inputs = %d", runner.inputCalls, len(runner.inputs))
			}
			var request struct {
				Service    string `json:"service"`
				Variant    string `json:"variant"`
				AccessOnly bool   `json:"access_only"`
			}
			if err := json.Unmarshal(runner.inputs[0], &request); err != nil || request.Service != test.service || request.Variant != test.variant || request.AccessOnly {
				t.Fatalf("protected enable request = %s (%+v, %v)", runner.inputs[0], request, err)
			}
		})
	}
	if run == 0 {
		t.Fatalf("selector %q matched no Ubuntu Pro service fixture", selector)
	}
}

// OS-UPM-043 through OS-UPM-053 and OS-UPM-057: after the pinned guest proves
// exact Ubuntu identity, run the reviewed public-provider high-risk fixtures
// inside that guest. Native effects stay behind deterministic API doubles.
func TestUbuntuProHighRiskMatrixVM(t *testing.T) {
	_ = ubuntuProVMFacts(t)
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "specialized-service-reporting", run: TestApplicatorServiceEnableReportsActivationWithoutExecutingMaintenance},
		{name: "fips-no-automatic-inverse", run: TestApplicatorDoesNotInvertNoAutomaticRecoveryService},
		{name: "realtime-kernel-variant", run: TestApplicatorConvergesObservableVariantWithoutDowngrade},
		{name: "explicit-purge-request", run: TestApplicatorSendsExplicitPurgeWithoutDowngrade},
		{name: "livepatch-conflict-planning", run: TestApplicatorDependencyAndConflictPlanningBoundaries},
		{name: "ordinary-specialized-services", run: TestOrdinaryServiceRowsUseObservableSecondCheck},
		{name: "best-effort-recovery", run: TestApplicatorRestoresEarlierServiceAfterLaterFailure},
		{name: "post-check-drift-recovery", run: TestApplicatorRestoresServiceAfterPostCheckDrift},
		{name: "reboot-signal", run: TestApplicatorApplyResultSignalsRebootRequired},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

// OS-UPM-015, OS-UPM-020 through OS-UPM-024, and OS-UPM-028 through
// OS-UPM-032: deterministic fault cases execute inside each pinned guest only
// after exact identity succeeds. This qualifies provider recovery/control
// flow, not a live Canonical subscription or entitled native effects.
func TestUbuntuProFaultMatrixVM(t *testing.T) {
	_ = ubuntuProVMFacts(t)
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "invalid-token", run: TestApplicatorInvalidTokenLeavesEndpointUnattached},
		{name: "attachment-faults", run: TestApplicatorAttachmentNegativeBoundaries},
		{name: "bounded-probe-faults", run: TestApplicatorCheckClassifiesBoundedProbeFailures},
		{name: "unentitled-service", run: TestApplicatorCheckClassifiesServiceAvailability},
		{name: "cancellation", run: TestApplicatorCheckHonorsCancellation},
		{name: "timeout", run: TestApplicatorCheckHonorsInjectedTimeout},
		{name: "malformed-and-oversized-envelope", run: TestDecodeEnvelopeBoundariesAndStableErrors},
		{name: "graph-drift-and-side-effects", run: TestApplicatorDependencyAndConflictPlanningBoundaries},
		{name: "partial-failure", run: TestApplicatorServiceFailureRollsBackOnlyNewAttachment},
		{name: "rollback-failure", run: TestApplicatorNewAttachmentRollbackFailureIsBoundedAndNotRetried},
		{name: "service-restoration", run: TestApplicatorRestoresEarlierServiceAfterLaterFailure},
		{name: "residual-effects", run: TestApplicatorDoesNotInvertNoAutomaticRecoveryService},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

// OS-UPM-003 and OS-UPM-004: Ubuntu-derived, ambiguous, interim, and future
// identities fail before API or secret boundaries, even inside an Ubuntu VM.
func TestUbuntuProNegativeIdentitiesVM(t *testing.T) {
	ubuntuProVMFacts(t)
	tests := []struct {
		name  string
		facts facts.Facts
	}{
		{name: "pop-os", facts: facts.Facts{Distro: types.Debian, DistroVersion: "22.04", OSID: "pop", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "System76", Arch: types.X86, Package: types.Apt}},
		{name: "linux-mint", facts: facts.Facts{Distro: types.Debian, DistroVersion: "22", OSID: "linuxmint", OSIDLike: []string{"ubuntu", "debian"}, OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Linux Mint", Arch: types.X86, Package: types.Apt}},
		{name: "conflicting-os-release", facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "24.04", OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: false, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt}},
		{name: "interim-ubuntu", facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "25.10", OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt}},
		{name: "future-ubuntu", facts: facts.Facts{Distro: types.Ubuntu, DistroVersion: "28.04", OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu", Arch: types.X86, Package: types.Apt}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &ubuntuProVMAPIDouble{}
			resolverCalls := 0
			result := executor.New().Apply(context.Background(), New(models.UbuntuProResource{
				ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
				Name:         "negative-identity", TokenRef: "remotr:ubuntu-pro/vm@active",
			}, test.facts, runner, ubuntuProVMResolver(t, &resolverCalls)))
			if result.Status != executor.Failed || result.Err == nil || len(runner.readCalls) != 0 || len(runner.input) != 0 || resolverCalls != 0 {
				t.Fatalf("Apply() = %+v, reads = %v, input = %q, resolver calls = %d", result, runner.readCalls, runner.input, resolverCalls)
			}
		})
	}
}
