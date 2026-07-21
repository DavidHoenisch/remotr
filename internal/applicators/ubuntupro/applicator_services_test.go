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
		if endpoint == dependenciesEndpoint {
			return []byte(`{"_schema_version":"v1","data":{"attributes":{"services":[]},"meta":{"environment_vars":[]},"type":"DependenciesResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`), nil, nil
		}
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

func enabledVariantEnvelope(name, variant string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"enabled_services":[{"name":%q,"variant_enabled":true,"variant_name":%q}]},"meta":{"environment_vars":[]},"type":"EnabledServicesResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, name, variant))
}

func serviceTransitionEnvelope(enabled, disabled []string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"enabled":%s,"disabled":%s,"reboot_required":false},"meta":{"environment_vars":[]},"type":"EnableResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, jsonStrings(enabled), jsonStrings(disabled)))
}

func dependenciesEnvelope(service string, dependencies, incompatible []string) []byte {
	relations := func(names []string, suffix string) string {
		values := make([]string, 0, len(names))
		for _, name := range names {
			values = append(values, fmt.Sprintf(`{"name":%q,"reason":{"code":%q,"title":"translated"}}`, name, service+suffix+name))
		}
		return strings.Join(values, ",")
	}
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"services":[{"name":%q,"depends_on":[%s],"incompatible_with":[%s]}]},"meta":{"environment_vars":[]},"type":"DependenciesResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, service, relations(dependencies, "-requires-"), relations(incompatible, "-conflicts-")))
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
	if len(runner.inputs) != 1 || !bytes.Equal(runner.inputs[0], []byte(`{"service":"esm-apps","access_only":false}`)) {
		t.Fatalf("enable stdin = %s, want typed full-mode request", runner.inputs)
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

// OS-UPM-017 and OS-UPM-024: every ordinary service row uses the same
// observable enabled-state and post-Check contract, independently by name.
func TestOrdinaryServiceRowsUseObservableSecondCheck(t *testing.T) {
	services := []string{"esm-infra", "esm-apps", "livepatch", "usg", "ros", "ros-updates", "anbox-cloud"}
	for _, service := range services {
		t.Run(service, func(t *testing.T) {
			nativeName := service
			if service == "usg" {
				nativeName = "cis"
			}
			runner := &serviceLifecycleRunner{
				readOutputs: map[string][][]byte{
					isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
					enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope(nativeName)},
				},
				inputOutputs: map[string][][]byte{
					enableEndpoint: {serviceTransitionEnvelope([]string{service}, nil)},
				},
			}
			resource := attachedResource()
			resource.Services = []models.UbuntuProService{{Name: service, State: models.UbuntuProServiceEnabled}}
			if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if fmt.Sprint(runner.inputCalls) != fmt.Sprint([]string{enableEndpoint}) {
				t.Fatalf("mutation endpoints = %v", runner.inputCalls)
			}
		})
	}
}

// OS-UPM-020, OS-UPM-021, and OS-UPM-032: the shared ordinary-service
// adapter fails closed for every authored name and ignores translated text.
func TestOrdinaryServiceRowsFailClosedOnStatusBoundaries(t *testing.T) {
	const messageCanary = "ubuntu-pro-translated-status-message-canary"
	services := []string{"esm-infra", "esm-apps", "livepatch", "usg", "ros", "ros-updates", "anbox-cloud"}
	for _, service := range services {
		t.Run(service, func(t *testing.T) {
			nativeName := service
			if service == "usg" {
				nativeName = "cis"
			}
			valid := string(enabledServicesEnvelope(nativeName))
			translated := strings.Replace(valid, `"enabled_services":`, `"message":"`+messageCanary+`","enabled_services":`, 1)
			cases := []struct {
				name       string
				output     []byte
				wantStatus executor.CheckStatus
				wantReason executor.ReasonCode
			}{
				{name: "unavailable", output: failureEnvelope("service-unavailable", messageCanary), wantStatus: executor.Unsupported, wantReason: "ubuntu_pro_service_unavailable"},
				{name: "unentitled", output: failureEnvelope("service-not-entitled", messageCanary), wantStatus: executor.Unsupported, wantReason: "ubuntu_pro_service_unentitled"},
				{name: "wrong schema", output: []byte(strings.Replace(valid, `"_schema_version":"v1"`, `"_schema_version":"v2"`, 1)), wantStatus: executor.CheckFailed, wantReason: executor.ReasonProbeFailed},
				{name: "malformed result", output: []byte(strings.Replace(valid, `"enabled_services":`, `"missing_services":`, 1)), wantStatus: executor.CheckFailed, wantReason: executor.ReasonProbeFailed},
				{name: "translated unknown member", output: []byte(translated), wantStatus: executor.Compliant, wantReason: executor.ReasonCompliant},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					runner := &providerCheckRunner{outputs: map[string][]byte{
						isAttachedEndpoint: attachmentEnvelope(true), enabledServicesEndpoint: test.output,
					}}
					resource := attachedResource()
					resource.Services = []models.UbuntuProService{{Name: service, State: models.UbuntuProServiceEnabled}}
					result := executor.Check(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
					if err := result.Validate(); err != nil {
						t.Fatalf("Check() returned invalid result: %v", err)
					}
					if result.Status != test.wantStatus || result.ReasonCode != test.wantReason {
						t.Fatalf("Check() = %s/%s (%v), want %s/%s", result.Status, result.ReasonCode, result.Err, test.wantStatus, test.wantReason)
					}
					if strings.Contains(fmt.Sprintf("%#v", result), messageCanary) {
						t.Fatalf("Check() exposed translated message: %#v", result)
					}
				})
			}
		})
	}
}

// OS-UPM-044 and OS-UPM-045: a named variant is sent only as a typed field and
// must be durably re-observed in the versioned enabled-services result.
func TestApplicatorConvergesObservableVariantWithoutDowngrade(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledVariantEnvelope("realtime-kernel", "raspi")},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {serviceTransitionEnvelope([]string{"realtime-kernel"}, nil)},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{
		Name: "realtime-kernel", State: models.UbuntuProServiceEnabled, Variant: "raspi",
	}}
	if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(runner.inputs) != 1 || !bytes.Equal(runner.inputs[0], []byte(`{"service":"realtime-kernel","variant":"raspi","access_only":false}`)) {
		t.Fatalf("variant stdin = %s", runner.inputs)
	}
}

// OS-UPM-043 and OS-UPM-045: explicit purge is never downgraded to the safe
// retain default; its typed flag is exact and disabled state is re-observed.
func TestApplicatorSendsExplicitPurgeWithoutDowngrade(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope("esm-apps"), enabledServicesEnvelope()},
		},
		inputOutputs: map[string][][]byte{
			disableEndpoint: {serviceTransitionEnvelope(nil, []string{"esm-apps"})},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{
		Name: "esm-apps", State: models.UbuntuProServiceDisabled, DisableMode: models.UbuntuProPurgePackages,
	}}
	if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(runner.inputs) != 1 || !bytes.Equal(runner.inputs[0], []byte(`{"service":"esm-apps","purge":true}`)) {
		t.Fatalf("purge stdin = %s", runner.inputs)
	}
}

// OS-UPM-046: a disabled dependency must be already satisfied or explicitly
// owned as enabled; otherwise planning stops before the mutation boundary.
func TestApplicatorBlocksOmittedDisabledDependency(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope()},
			dependenciesEndpoint:    {dependenciesEnvelope("usg", []string{"esm-apps"}, nil)},
		},
		inputOutputs: map[string][][]byte{},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{Name: "usg", State: models.UbuntuProServiceEnabled}}
	err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), string(executor.ReasonDependencyBlocked)) {
		t.Fatalf("Apply() error = %v, want dependency_blocked", err)
	}
	if len(runner.inputCalls) != 0 {
		t.Fatalf("dependency failure crossed mutation boundary: %v", runner.inputCalls)
	}
	if fmt.Sprint(runner.readCalls) != fmt.Sprint([]string{isAttachedEndpoint, enabledServicesEndpoint, dependenciesEndpoint}) {
		t.Fatalf("read endpoints = %v", runner.readCalls)
	}
}

// OS-UPM-049: declared dependencies are enabled before their targets even
// when authored in the opposite order.
func TestApplicatorOrdersDeclaredDependenciesBeforeTargets(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope("esm-apps", "usg")},
			dependenciesEndpoint:    {dependenciesEnvelope("usg", []string{"esm-apps"}, nil)},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {
				serviceTransitionEnvelope([]string{"esm-apps"}, nil),
				serviceTransitionEnvelope([]string{"usg"}, nil),
			},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{
		{Name: "usg", State: models.UbuntuProServiceEnabled},
		{Name: "esm-apps", State: models.UbuntuProServiceEnabled},
	}
	if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	wantInputs := [][]byte{
		[]byte(`{"service":"esm-apps","access_only":false}`),
		[]byte(`{"service":"usg","access_only":false}`),
	}
	if len(runner.inputs) != len(wantInputs) {
		t.Fatalf("enable inputs = %q", runner.inputs)
	}
	for index := range wantInputs {
		if !bytes.Equal(runner.inputs[index], wantInputs[index]) {
			t.Fatalf("enable input %d = %s, want %s", index, runner.inputs[index], wantInputs[index])
		}
	}
}
