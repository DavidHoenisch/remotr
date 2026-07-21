package ubuntupro

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type attachmentLifecycleRunner struct {
	statusOutputs [][]byte
	attachOutput  []byte
	attachErr     error
	readCalls     int
	inputCalls    int
}

func (runner *attachmentLifecycleRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return runner.run(name, args...)
}

func (runner *attachmentLifecycleRunner) RunContext(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	return runner.run(name, args...)
}

func (runner *attachmentLifecycleRunner) run(name string, args ...string) ([]byte, []byte, error) {
	runner.readCalls++
	if name != proExecutable || len(args) != 2 || args[0] != "api" || args[1] != isAttachedEndpoint || len(runner.statusOutputs) == 0 {
		return nil, nil, fmt.Errorf("unexpected read process %s %v", name, args)
	}
	output := runner.statusOutputs[0]
	runner.statusOutputs = runner.statusOutputs[1:]
	return append([]byte(nil), output...), nil, nil
}

func (runner *attachmentLifecycleRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.runInput(name, input, args...)
}

func (runner *attachmentLifecycleRunner) RunInputContext(_ context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.runInput(name, input, args...)
}

func (runner *attachmentLifecycleRunner) runInput(name string, _ []byte, args ...string) ([]byte, []byte, error) {
	runner.inputCalls++
	if name != proExecutable || len(args) != 4 || args[0] != "api" || args[1] != fullTokenAttachEndpoint || args[2] != "--data" || args[3] != "-" {
		return nil, nil, fmt.Errorf("unexpected input process %s %v", name, args)
	}
	return append([]byte(nil), runner.attachOutput...), nil, runner.attachErr
}

func attachedResource() models.UbuntuProResource {
	return models.UbuntuProResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
		Name:         "primary-subscription", TokenRef: "remotr:ubuntu-pro/production@active",
	}
}

func attachSuccessEnvelope() []byte {
	return []byte(`{"_schema_version":"v1","data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
}

// OS-UPM-014 and OS-UPM-024: successful mutation is not convergence until a
// health-aware public second Check passes.
func TestApplicatorApplyRejectsPostAttachWarning(t *testing.T) {
	const warningCanary = "ubuntu-pro-post-attach-localized-warning-canary"
	warningStatus := strings.Replace(
		string(attachmentEnvelope(true)),
		`"warnings":[]`,
		`"warnings":[{"code":"contract-warning","msg":"`+warningCanary+`","meta":{}}]`,
		1,
	)
	runner := &attachmentLifecycleRunner{
		statusOutputs: [][]byte{attachmentEnvelope(false), []byte(warningStatus)},
		attachOutput:  attachSuccessEnvelope(),
	}
	resolverCalls := 0
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		resolverCalls++
		return []byte("post-attach-warning-token-canary"), nil
	})
	err := applicator.Apply(context.Background())
	if err == nil {
		t.Fatal("Apply() accepted an unhealthy post-attach Check")
	}
	if strings.Contains(err.Error(), warningCanary) {
		t.Fatalf("Apply() exposed localized warning: %v", err)
	}
	if runner.readCalls != 2 || runner.inputCalls != 1 || resolverCalls != 1 {
		t.Fatalf("Apply() calls = read:%d input:%d resolve:%d", runner.readCalls, runner.inputCalls, resolverCalls)
	}
}

// OS-UPM-010, OS-UPM-011, OS-UPM-014, and OS-UPM-024: attachment converges
// through public Apply/Check and a second Apply performs no secret or mutation.
func TestApplicatorAttachmentConvergesIdempotently(t *testing.T) {
	runner := &attachmentLifecycleRunner{
		statusOutputs: [][]byte{
			attachmentEnvelope(false), attachmentEnvelope(true),
			attachmentEnvelope(true), attachmentEnvelope(true),
		},
		attachOutput: attachSuccessEnvelope(),
	}
	resolverCalls := 0
	material := []byte("attachment-idempotence-token-canary")
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		resolverCalls++
		return material, nil
	})
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	check := executor.Check(context.Background(), applicator)
	if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("post-Apply Check() = %s/%s (%v)", check.Status, check.ReasonCode, check.Err)
	}
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	if runner.readCalls != 4 || runner.inputCalls != 1 || resolverCalls != 1 {
		t.Fatalf("lifecycle calls = read:%d input:%d resolve:%d", runner.readCalls, runner.inputCalls, resolverCalls)
	}
	for index, value := range material {
		if value != 0 {
			t.Fatalf("resolved token byte %d was not cleared", index)
		}
	}
}

// OS-UPM-010 and OS-UPM-015: an invalid token leaves the observable endpoint
// unattached and does not trigger a retry or contract replacement.
func TestApplicatorInvalidTokenLeavesEndpointUnattached(t *testing.T) {
	runner := &attachmentLifecycleRunner{
		statusOutputs: [][]byte{attachmentEnvelope(false), attachmentEnvelope(false)},
		attachOutput:  failureEnvelope("invalid-token", "localized-invalid-token-canary"),
	}
	resolverCalls := 0
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		resolverCalls++
		return []byte("invalid-token-material-canary"), nil
	})
	err := applicator.Apply(context.Background())
	var apiError APIError
	if !errors.As(err, &apiError) || apiError.Code != "invalid-token" {
		t.Fatalf("Apply() error = %T %v, want stable invalid-token APIError", err, err)
	}
	check := executor.Check(context.Background(), applicator)
	if check.Status != executor.Drifted || check.ReasonCode != executor.ReasonStateDrift {
		t.Fatalf("Check() = %s/%s (%v), want unattached drift", check.Status, check.ReasonCode, check.Err)
	}
	if runner.readCalls != 2 || runner.inputCalls != 1 || resolverCalls != 1 {
		t.Fatalf("invalid-token calls = read:%d input:%d resolve:%d", runner.readCalls, runner.inputCalls, resolverCalls)
	}
}

// OS-UPM-024 and OS-UPM-032: a lost response after possible native success
// triggers one read-only recovery Check and never retries attachment.
func TestApplicatorLostAttachResponseChecksWithoutRetry(t *testing.T) {
	const processCanary = "ubuntu-pro-lost-response-process-canary"
	runner := &attachmentLifecycleRunner{
		statusOutputs: [][]byte{attachmentEnvelope(false), attachmentEnvelope(true)},
		attachErr:     errors.New(processCanary),
	}
	resolverCalls := 0
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		resolverCalls++
		return []byte("lost-response-token-canary"), nil
	})
	err := applicator.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "attachment outcome is ambiguous") {
		t.Fatalf("Apply() error = %v, want bounded ambiguous outcome", err)
	}
	if strings.Contains(err.Error(), processCanary) {
		t.Fatalf("Apply() exposed process diagnostic: %v", err)
	}
	if runner.readCalls != 2 || runner.inputCalls != 1 || resolverCalls != 1 {
		t.Fatalf("lost-response calls = read:%d input:%d resolve:%d", runner.readCalls, runner.inputCalls, resolverCalls)
	}
}

// OS-UPM-012 and OS-UPM-032: resolver denial stops before mutation and its
// potentially secret diagnostic is not copied into the provider error.
func TestApplicatorResolverDenialIsRedacted(t *testing.T) {
	const resolverCanary = "ubuntu-pro-resolver-denial-secret-canary"
	runner := &attachmentLifecycleRunner{statusOutputs: [][]byte{attachmentEnvelope(false)}}
	resolverCalls := 0
	applicator := New(attachedResource(), exactUbuntuFacts(), runner, func(context.Context, string) ([]byte, error) {
		resolverCalls++
		return nil, errors.New(resolverCanary)
	})
	err := applicator.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "token resolution failed") {
		t.Fatalf("Apply() error = %v, want bounded token-resolution failure", err)
	}
	if strings.Contains(err.Error(), resolverCanary) {
		t.Fatalf("Apply() exposed resolver diagnostic: %v", err)
	}
	if runner.readCalls != 1 || runner.inputCalls != 0 || resolverCalls != 1 {
		t.Fatalf("resolver-denial calls = read:%d input:%d resolve:%d", runner.readCalls, runner.inputCalls, resolverCalls)
	}
}
