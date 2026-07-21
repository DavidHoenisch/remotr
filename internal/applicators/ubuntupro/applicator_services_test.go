package ubuntupro

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type serviceLifecycleRunner struct {
	readOutputs  map[string][][]byte
	inputOutputs map[string][][]byte
	readCalls    []string
	inputCalls   []string
	inputs       [][]byte
}

func (runner *serviceLifecycleRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return runner.run(name, args...)
}

func (runner *serviceLifecycleRunner) RunContext(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	return runner.run(name, args...)
}

func (runner *serviceLifecycleRunner) run(name string, args ...string) ([]byte, []byte, error) {
	if name != proExecutable || len(args) != 2 || args[0] != "api" {
		return nil, nil, fmt.Errorf("unexpected read process %s %v", name, args)
	}
	endpoint := args[1]
	runner.readCalls = append(runner.readCalls, endpoint)
	outputs := runner.readOutputs[endpoint]
	if len(outputs) == 0 {
		return nil, nil, fmt.Errorf("unexpected read endpoint %s", endpoint)
	}
	runner.readOutputs[endpoint] = outputs[1:]
	return append([]byte(nil), outputs[0]...), nil, nil
}

func (runner *serviceLifecycleRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.runInput(name, input, args...)
}

func (runner *serviceLifecycleRunner) RunInputContext(_ context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	return runner.runInput(name, input, args...)
}

func (runner *serviceLifecycleRunner) runInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	if name != proExecutable || len(args) != 4 || args[0] != "api" || args[2] != "--data" || args[3] != "-" {
		return nil, nil, fmt.Errorf("unexpected input process %s %v", name, args)
	}
	endpoint := args[1]
	runner.inputCalls = append(runner.inputCalls, endpoint)
	runner.inputs = append(runner.inputs, append([]byte(nil), input...))
	outputs := runner.inputOutputs[endpoint]
	if len(outputs) == 0 {
		return nil, nil, fmt.Errorf("unexpected input endpoint %s", endpoint)
	}
	runner.inputOutputs[endpoint] = outputs[1:]
	return append([]byte(nil), outputs[0]...), nil, nil
}

func serviceTransitionEnvelope(enabled, disabled []string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"enabled":%s,"disabled":%s,"reboot_required":false},"meta":{"environment_vars":[]},"type":"EnableResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, jsonStrings(enabled), jsonStrings(disabled)))
}

func jsonStrings(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	result := "["
	for index, value := range values {
		if index != 0 {
			result += ","
		}
		result += fmt.Sprintf("%q", value)
	}
	return result + "]"
}

// OS-UPM-017 and OS-UPM-024: one declared ordinary service converges through
// the protected versioned enable endpoint and a public second Check.
func TestApplicatorEnablesDeclaredService(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope("esm-apps"), enabledServicesEnvelope("esm-apps")},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {serviceTransitionEnvelope([]string{"esm-apps"}, nil)},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
	applicator := New(resource, exactUbuntuFacts(), runner, nil)
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	check := executor.Check(context.Background(), applicator)
	if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("second Check() = %s/%s (%v), want compliant", check.Status, check.ReasonCode, check.Err)
	}
	if fmt.Sprint(runner.inputCalls) != fmt.Sprint([]string{enableEndpoint}) {
		t.Fatalf("mutation endpoints = %v, want enable only", runner.inputCalls)
	}
}

// OS-UPM-019 and OS-UPM-024: explicit disable retains packages by default,
// leaves omitted services untouched, and passes a public second Check.
func TestApplicatorDisablesOnlyDeclaredServiceWithoutPurge(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint: {
				attachmentEnvelope(true), attachmentEnvelope(true), attachmentEnvelope(true),
			},
			enabledServicesEndpoint: {
				enabledServicesEnvelope("esm-apps", "usg"), enabledServicesEnvelope("usg"), enabledServicesEnvelope("usg"),
			},
		},
		inputOutputs: map[string][][]byte{
			disableEndpoint: {serviceTransitionEnvelope(nil, []string{"esm-apps"})},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceDisabled}}
	applicator := New(resource, exactUbuntuFacts(), runner, nil)
	if err := applicator.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	check := executor.Check(context.Background(), applicator)
	if check.Status != executor.Compliant || check.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("second Check() = %s/%s (%v), want compliant", check.Status, check.ReasonCode, check.Err)
	}
	if fmt.Sprint(runner.inputCalls) != fmt.Sprint([]string{disableEndpoint}) {
		t.Fatalf("mutation endpoints = %v, want disable only", runner.inputCalls)
	}
	if len(runner.inputs) != 1 || !bytes.Equal(runner.inputs[0], []byte(`{"service":"esm-apps","purge":false}`)) {
		t.Fatalf("disable stdin = %s, want typed retain-packages request", runner.inputs)
	}
	if strings.Contains(fmt.Sprint(check.Actual), "usg") {
		t.Fatalf("Check() exposed omitted service: %#v", check.Actual)
	}
}
