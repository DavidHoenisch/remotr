package ubuntupro

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
			if len(report.Services) != 0 || len(report.WarningCodes) != 0 {
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
	}{
		{name: "invalid contract", output: failureEnvelope("invalid-contract", diagnosticCanary), wantStatus: executor.CheckFailed, wantReason: "ubuntu_pro_contract_invalid"},
		{name: "expired contract", output: failureEnvelope("expired-contract", diagnosticCanary), wantStatus: executor.CheckFailed, wantReason: "ubuntu_pro_contract_expired"},
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
			if result.Actual != nil || strings.Contains(fmt.Sprint(result.Err), diagnosticCanary) || strings.Contains(string(result.ObservedSummary), diagnosticCanary) {
				t.Fatalf("Check() exposed probe diagnostic: %#v", result)
			}
			if len(runner.calls) != 1 || runner.calls[0] != isAttachedEndpoint {
				t.Fatalf("Check() endpoints = %v", runner.calls)
			}
		})
	}
}
