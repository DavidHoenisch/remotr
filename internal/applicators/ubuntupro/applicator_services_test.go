package ubuntupro

import (
	"bytes"
	"context"
	"fmt"
	"slices"
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

func serviceTransitionRebootEnvelope(enabled, disabled []string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":{"enabled":%s,"disabled":%s,"reboot_required":true},"meta":{"environment_vars":[]},"type":"EnableResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, jsonStrings(enabled), jsonStrings(disabled)))
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

// OS-UPM-059: service lifecycle reports a separate maintenance need through
// bounded activation state and never executes package, fix, hardening, or
// reboot actions inside the Ubuntu Pro provider.
func TestApplicatorServiceEnableReportsActivationWithoutExecutingMaintenance(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope("usg"), enabledServicesEnvelope("usg")},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {serviceTransitionRebootEnvelope([]string{"usg"}, nil)},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{Name: "usg", State: models.UbuntuProServiceEnabled}}
	applicator := New(resource, exactUbuntuFacts(), runner, nil)
	result := executor.New().Apply(context.Background(), applicator)
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}
	if result.Status != executor.Changed || result.RebootRequired != executor.RebootRequired || !slices.Equal(result.Activation, wantActivation) || len(result.Diagnostics) != 0 {
		t.Fatalf("Apply() result = %+v", result)
	}
	check := executor.Check(context.Background(), applicator)
	report, ok := check.Actual.(StateReport)
	if check.Status != executor.Compliant || !ok || !slices.Equal(report.Services, []ServiceState{{Name: "usg", Enabled: true}}) {
		t.Fatalf("Check() = %+v", check)
	}
	if !slices.Equal(runner.inputCalls, []string{enableEndpoint}) {
		t.Fatalf("mutation endpoints = %v, want versioned service enable only", runner.inputCalls)
	}
}

// OS-UPM-051 and OS-UPM-057: a high-impact service with a cataloged
// no-automatic-rollback contract is observed after later failure, never
// inverted with a generic disable operation.
func TestApplicatorDoesNotInvertNoAutomaticRecoveryService(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope("fips")},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {
				serviceTransitionEnvelope([]string{"fips"}, nil),
				failureEnvelope("service-not-entitled", "localized-later-service-canary"),
			},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{
		{Name: "fips", State: models.UbuntuProServiceEnabled},
		{Name: "ros", State: models.UbuntuProServiceEnabled},
	}
	result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	if result.Status != executor.Failed || result.RollbackClass != executor.RollbackNone || result.Err == nil || !strings.Contains(result.Err.Error(), "service-not-entitled") || !strings.Contains(result.Err.Error(), "rollback incomplete") {
		t.Fatalf("Apply() result = %+v", result)
	}
	if !slices.Equal(runner.inputCalls, []string{enableEndpoint, enableEndpoint}) {
		t.Fatalf("mutation endpoints = %v, want enables only", runner.inputCalls)
	}
	wantReads := []string{isAttachedEndpoint, enabledServicesEndpoint, dependenciesEndpoint, enabledServicesEndpoint}
	if !slices.Equal(runner.readCalls, wantReads) {
		t.Fatalf("read endpoints = %v, want %v", runner.readCalls, wantReads)
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

func TestApplicatorDependencyAndConflictPlanningBoundaries(t *testing.T) {
	t.Run("already satisfied dependency", func(t *testing.T) {
		runner := &serviceLifecycleRunner{
			readOutputs: map[string][][]byte{
				isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
				enabledServicesEndpoint: {enabledServicesEnvelope("esm-apps"), enabledServicesEnvelope("esm-apps", "usg")},
				dependenciesEndpoint:    {dependenciesEnvelope("usg", []string{"esm-apps"}, nil)},
			},
			inputOutputs: map[string][][]byte{enableEndpoint: {serviceTransitionEnvelope([]string{"usg"}, nil)}},
		}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{{Name: "usg", State: models.UbuntuProServiceEnabled}}
		if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if len(runner.inputs) != 1 || !bytes.Contains(runner.inputs[0], []byte(`"service":"usg"`)) {
			t.Fatalf("mutation inputs = %q", runner.inputs)
		}
	})

	t.Run("declared incompatible transition", func(t *testing.T) {
		runner := &serviceLifecycleRunner{
			readOutputs: map[string][][]byte{
				isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
				enabledServicesEndpoint: {enabledServicesEnvelope("fips"), enabledServicesEnvelope("cis")},
				dependenciesEndpoint:    {dependenciesEnvelope("usg", nil, []string{"fips"})},
			},
			inputOutputs: map[string][][]byte{
				disableEndpoint: {serviceTransitionEnvelope(nil, []string{"fips"})},
				enableEndpoint:  {serviceTransitionEnvelope([]string{"usg"}, nil)},
			},
		}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{
			{Name: "usg", State: models.UbuntuProServiceEnabled},
			{Name: "fips", State: models.UbuntuProServiceDisabled},
		}
		if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if fmt.Sprint(runner.inputCalls) != fmt.Sprint([]string{disableEndpoint, enableEndpoint}) {
			t.Fatalf("mutation order = %v", runner.inputCalls)
		}
	})

	t.Run("omitted enabled conflict", func(t *testing.T) {
		runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope("fips")},
			dependenciesEndpoint:    {dependenciesEnvelope("usg", nil, []string{"fips"})},
		}, inputOutputs: map[string][][]byte{}}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{{Name: "usg", State: models.UbuntuProServiceEnabled}}
		err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background())
		if err == nil || !strings.Contains(err.Error(), string(executor.ReasonDependencyBlocked)) || len(runner.inputCalls) != 0 {
			t.Fatalf("Apply() = %v, mutations = %v", err, runner.inputCalls)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		cycle := []byte(`{"_schema_version":"v1","data":{"attributes":{"services":[{"name":"usg","depends_on":[{"name":"esm-apps","reason":{"code":"usg-requires-esm"}}],"incompatible_with":[]},{"name":"esm-apps","depends_on":[{"name":"usg","reason":{"code":"esm-requires-usg"}}],"incompatible_with":[]}]},"meta":{"environment_vars":[]},"type":"DependenciesResult"},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`)
		runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope()},
			dependenciesEndpoint:    {cycle},
		}, inputOutputs: map[string][][]byte{}}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{
			{Name: "usg", State: models.UbuntuProServiceEnabled},
			{Name: "esm-apps", State: models.UbuntuProServiceEnabled},
		}
		err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background())
		if err == nil || !strings.Contains(err.Error(), "cycle") || len(runner.inputCalls) != 0 {
			t.Fatalf("Apply() = %v, mutations = %v", err, runner.inputCalls)
		}
	})

	t.Run("unknown graph member", func(t *testing.T) {
		runner := &serviceLifecycleRunner{readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope()},
			dependenciesEndpoint:    {dependenciesEnvelope("usg", []string{"future-beta"}, nil)},
		}, inputOutputs: map[string][][]byte{}}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{{Name: "usg", State: models.UbuntuProServiceEnabled}}
		if err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background()); err == nil || len(runner.inputCalls) != 0 {
			t.Fatalf("Apply() = %v, mutations = %v", err, runner.inputCalls)
		}
	})

	t.Run("native response mismatch", func(t *testing.T) {
		runner := &serviceLifecycleRunner{
			readOutputs: map[string][][]byte{
				isAttachedEndpoint:      {attachmentEnvelope(true)},
				enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope("usg")},
			},
			inputOutputs: map[string][][]byte{
				enableEndpoint:  {serviceTransitionEnvelope([]string{"esm-apps", "usg"}, nil)},
				disableEndpoint: {serviceTransitionEnvelope(nil, []string{"esm-apps"})},
			},
		}
		resource := attachedResource()
		resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
		err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unexpected effects") || !strings.Contains(err.Error(), "rollback incomplete") || len(runner.inputCalls) != 2 {
			t.Fatalf("Apply() = %v, mutations = %v", err, runner.inputCalls)
		}
	})
}

// OS-UPM-023 and OS-UPM-024: a later operation failure restores earlier
// managed service changes in reverse order and verifies the prior state.
func TestApplicatorRestoresEarlierServiceAfterLaterFailure(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope()},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint: {
				serviceTransitionEnvelope([]string{"esm-apps"}, nil),
				failureEnvelope("service-unavailable", "localized-later-failure-canary"),
			},
			disableEndpoint: {serviceTransitionEnvelope(nil, []string{"esm-apps"})},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{
		{Name: "esm-apps", State: models.UbuntuProServiceEnabled},
		{Name: "usg", State: models.UbuntuProServiceEnabled},
	}
	err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "service-unavailable") || !strings.Contains(err.Error(), "rollback restored") {
		t.Fatalf("Apply() error = %v, want original failure plus restored rollback", err)
	}
	wantCalls := []string{enableEndpoint, enableEndpoint, disableEndpoint}
	if fmt.Sprint(runner.inputCalls) != fmt.Sprint(wantCalls) {
		t.Fatalf("mutation order = %v, want %v", runner.inputCalls, wantCalls)
	}
}

// OS-UPM-023 and OS-UPM-024: state drift detected by the second Check restores
// the declared change and verifies the prior managed state.
func TestApplicatorRestoresServiceAfterPostCheckDrift(t *testing.T) {
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint: {attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {
				enabledServicesEnvelope(), enabledServicesEnvelope(), enabledServicesEnvelope(),
			},
		},
		inputOutputs: map[string][][]byte{
			enableEndpoint:  {serviceTransitionEnvelope([]string{"esm-apps"}, nil)},
			disableEndpoint: {serviceTransitionEnvelope(nil, []string{"esm-apps"})},
		},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
	err := New(resource, exactUbuntuFacts(), runner, nil).Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), string(executor.ReasonStateDrift)) || !strings.Contains(err.Error(), "rollback restored") {
		t.Fatalf("Apply() error = %v, want post-check drift plus restored rollback", err)
	}
	if fmt.Sprint(runner.inputCalls) != fmt.Sprint([]string{enableEndpoint, disableEndpoint}) {
		t.Fatalf("mutation order = %v", runner.inputCalls)
	}
}

// OS-UPM-033 and OS-UPM-051: native reboot need becomes the standard
// structured activation; the process boundary contains no reboot command.
func TestApplicatorApplyResultSignalsRebootRequired(t *testing.T) {
	rebootTransition := strings.Replace(
		string(serviceTransitionEnvelope([]string{"esm-apps"}, nil)),
		`"reboot_required":false`, `"reboot_required":true`, 1,
	)
	runner := &serviceLifecycleRunner{
		readOutputs: map[string][][]byte{
			isAttachedEndpoint:      {attachmentEnvelope(true), attachmentEnvelope(true)},
			enabledServicesEndpoint: {enabledServicesEnvelope(), enabledServicesEnvelope("esm-apps")},
		},
		inputOutputs: map[string][][]byte{enableEndpoint: {[]byte(rebootTransition)}},
	}
	resource := attachedResource()
	resource.Services = []models.UbuntuProService{{Name: "esm-apps", State: models.UbuntuProServiceEnabled}}
	result := executor.New().Apply(context.Background(), New(resource, exactUbuntuFacts(), runner, nil))
	wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}
	if result.Status != executor.Changed || result.RebootRequired != executor.RebootRequired || !slices.Equal(result.Activation, wantActivation) {
		t.Fatalf("Apply() result = %+v, want changed/reboot-required", result)
	}
	for _, endpoint := range append(append([]string(nil), runner.readCalls...), runner.inputCalls...) {
		if strings.Contains(endpoint, "reboot") {
			t.Fatalf("provider invoked a reboot boundary: %v %v", runner.readCalls, runner.inputCalls)
		}
	}
}
