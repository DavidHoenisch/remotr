package ubuntupro

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type apiBoundaryRunner struct {
	name           string
	args           []string
	input          []byte
	inputCalls     int
	runCalls       int
	stdout         []byte
	ordinaryStdout []byte
}

func (r *apiBoundaryRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.runCalls++
	r.name = name
	r.args = append([]string(nil), args...)
	if r.ordinaryStdout == nil {
		return nil, nil, fmt.Errorf("ordinary Run was not configured: %s %v", name, args)
	}
	return append([]byte(nil), r.ordinaryStdout...), nil, nil
}

func (r *apiBoundaryRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	r.inputCalls++
	r.name = name
	r.args = append([]string(nil), args...)
	r.input = append([]byte(nil), input...)
	return append([]byte(nil), r.stdout...), nil, nil
}

// OS-UPM-038 and OS-LPC-020: read-only attachment observation uses only the
// literal versioned endpoint and the documented common envelope.
func TestAPIClientIsAttachedUsesExactReadOnlyEndpoint(t *testing.T) {
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"is_attached":true},"meta":{"environment_vars":[]},"type":"IsAttachedResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).IsAttached()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.status.is_attached.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	if !result.Attached || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("attachment result = %#v", result)
	}
	argv := runner.name + " " + strings.Join(runner.args, " ")
	for _, forbidden := range []string{" status", "--format", "--args", "sh -c"} {
		if strings.Contains(argv, forbidden) {
			t.Fatalf("read-only process boundary contains %q: %s", forbidden, argv)
		}
	}
}

// OS-UPM-038 and OS-LPC-020: runtime client eligibility comes from the
// literal version endpoint rather than a human-oriented command.
func TestAPIClientVersionUsesExactReadOnlyEndpoint(t *testing.T) {
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"installed_version":"32.3ubuntu0"},"meta":{"environment_vars":[]},"type":"VersionResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).Version()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.version.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	if result.InstalledVersion != "32.3ubuntu0" || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("version result = %#v", result)
	}
}

// OS-UPM-032, OS-UPM-038, and OS-LPC-020: enabled services are typed from the
// versioned endpoint, including the documented cis representation of usg.
func TestAPIClientEnabledServicesUsesExactReadOnlyEndpoint(t *testing.T) {
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"enabled_services":[
    {"name":"cis","variant_enabled":false,"variant_name":null},
    {"name":"realtime-kernel","variant_enabled":true,"variant_name":"raspi"}
  ]},"meta":{"environment_vars":[]},"type":"EnabledServicesResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).EnabledServices()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.status.enabled_services.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	want := []EnabledService{{Name: "usg"}, {Name: "realtime-kernel", Variant: "raspi"}}
	if !slices.Equal(result.Services, want) || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("enabled-services result = %#v, want %#v", result, want)
	}
}

// OS-UPM-038, OS-UPM-046, and OS-LPC-020: dependency control flow retains
// stable codes and never localized reason titles.
func TestAPIClientDependenciesUsesExactReadOnlyEndpoint(t *testing.T) {
	const localizedTitleCanary = "localized-dependency-title-canary"
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"services":[
    {"name":"usg","depends_on":[{"name":"esm-apps","reason":{"code":"usg-requires-esm","title":"` + localizedTitleCanary + `"}}],"incompatible_with":[{"name":"fips","reason":{"code":"usg-conflicts-fips","title":"translated"}}]}
  ]},"meta":{"environment_vars":[]},"type":"DependenciesResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).Dependencies()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.services.dependencies.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	want := []ServiceDependencies{{
		Name:             "usg",
		DependsOn:        []ServiceRelation{{Name: "esm-apps", Code: "usg-requires-esm"}},
		IncompatibleWith: []ServiceRelation{{Name: "fips", Code: "usg-conflicts-fips"}},
	}}
	if !slices.EqualFunc(result.Services, want, func(left, right ServiceDependencies) bool {
		return left.Name == right.Name && slices.Equal(left.DependsOn, right.DependsOn) && slices.Equal(left.IncompatibleWith, right.IncompatibleWith)
	}) {
		t.Fatalf("dependency result = %#v, want %#v", result.Services, want)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), localizedTitleCanary) {
		t.Fatalf("localized title entered typed result: %#v", result)
	}
}

// OS-LSM-079: newer Ubuntu Pro clients may advertise independent historical
// or preview services that Remotr does not manage. Those informational nodes
// do not invalidate the dependency graph for cataloged services.
func TestAPIClientDependenciesIgnoresUnmanagedInformationalServices(t *testing.T) {
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"services":[
    {"name":"cc-eal","depends_on":[],"incompatible_with":[]},
    {"name":"esm-apps","depends_on":[],"incompatible_with":[]},
    {"name":"esm-apps-legacy","depends_on":[],"incompatible_with":[]},
    {"name":"fips-preview","depends_on":[],"incompatible_with":[]}
  ]},"meta":{"environment_vars":[]},"type":"DependenciesResult"},
  "errors":[],"result":"success","version":"37.2ubuntu0.1","warnings":[]
}`)}

	result, err := NewAPIClient(runner).Dependencies()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.services.dependencies.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	want := []ServiceDependencies{{Name: "esm-apps"}}
	if !slices.EqualFunc(result.Services, want, func(left, right ServiceDependencies) bool {
		return left.Name == right.Name && slices.Equal(left.DependsOn, right.DependsOn) && slices.Equal(left.IncompatibleWith, right.IncompatibleWith)
	}) {
		t.Fatalf("dependency result = %#v, want %#v", result.Services, want)
	}
	if result.ClientVersion != "37.2ubuntu0.1" {
		t.Fatalf("client version = %q, want 37.2ubuntu0.1", result.ClientVersion)
	}
}

// OS-UPM-033 and OS-UPM-038: reboot state comes from the literal versioned
// endpoint and its closed documented enum.
func TestAPIClientRebootRequiredUsesExactReadOnlyEndpoint(t *testing.T) {
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"reboot_required":"yes-kernel-livepatches-applied"},"meta":{"environment_vars":[]},"type":"RebootRequiredResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).RebootRequired()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.security.status.reboot_required.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	if !result.Required || !result.LivepatchesApplied || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("reboot-required result = %#v", result)
	}
}

// OS-UPM-037, OS-UPM-044, and OS-LPC-020: service enablement uses typed JSON
// stdin and excludes localized messages from its control result.
func TestAPIClientEnableUsesExactProtectedProcessBoundary(t *testing.T) {
	const localizedMessageCanary = "localized-enable-message-canary"
	runner := &apiBoundaryRunner{stdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"enabled":["realtime-kernel"],"disabled":["livepatch"],"messages":["` + localizedMessageCanary + `"],"reboot_required":true},"meta":{"environment_vars":[]},"type":"EnableResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).Enable("realtime-kernel", "raspi", true)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.services.enable.v1", "--data", "-"}
	if runner.inputCalls != 1 || runner.runCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	wantInput := []byte(`{"service":"realtime-kernel","variant":"raspi","access_only":true}`)
	if !bytes.Equal(runner.input, wantInput) {
		t.Fatalf("protected stdin = %s, want %s", runner.input, wantInput)
	}
	if !slices.Equal(result.Enabled, []string{"realtime-kernel"}) || !slices.Equal(result.Disabled, []string{"livepatch"}) || !result.RebootRequired {
		t.Fatalf("enable result = %#v", result)
	}
	if strings.Contains(fmt.Sprintf("%#v", result), localizedMessageCanary) {
		t.Fatalf("localized message entered typed result: %#v", result)
	}
}

// OS-UPM-037 and OS-UPM-043: service disablement and purge intent use only
// typed JSON protected stdin.
func TestAPIClientDisableUsesExactProtectedProcessBoundary(t *testing.T) {
	runner := &apiBoundaryRunner{stdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"disabled":["fips"]},"meta":{"environment_vars":[]},"type":"DisableResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).Disable("fips", true)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.services.disable.v1", "--data", "-"}
	if runner.inputCalls != 1 || runner.runCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	wantInput := []byte(`{"service":"fips","purge":true}`)
	if !bytes.Equal(runner.input, wantInput) {
		t.Fatalf("protected stdin = %s, want %s", runner.input, wantInput)
	}
	if !slices.Equal(result.Disabled, []string{"fips"}) || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("disable result = %#v", result)
	}
}

// OS-UPM-025, OS-UPM-033, and OS-LPC-020: explicit detachment uses only the
// literal versioned endpoint and its typed disabled/reboot result.
func TestAPIClientDetachUsesExactEndpoint(t *testing.T) {
	runner := &apiBoundaryRunner{ordinaryStdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"disabled":["esm-apps","esm-infra"],"reboot_required":true},"meta":{"environment_vars":[]},"type":"DetachResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	result, err := NewAPIClient(runner).Detach()
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "u.pro.detach.v1"}
	if runner.runCalls != 1 || runner.inputCalls != 0 || runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q (Run=%d, RunInput=%d), want /usr/bin/pro %q", runner.name, runner.args, runner.runCalls, runner.inputCalls, wantArgs)
	}
	if !slices.Equal(result.Disabled, []string{"esm-apps", "esm-infra"}) || !result.RebootRequired || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("detach result = %#v", result)
	}
}

// OS-UPM-010, OS-UPM-037, OS-UPM-039, OS-LPC-019, and OS-LPC-020: Canonical's
// v32 full-token endpoint receives a typed JSON object through protected stdin
// and no token-bearing or legacy command-line representation exists.
func TestAPIClientFullTokenAttachUsesExactProtectedProcessBoundary(t *testing.T) {
	const tokenCanary = "ubuntu-pro-process-boundary-token-canary"
	runner := &apiBoundaryRunner{stdout: []byte(`{
  "_schema_version":"v1",
  "data":{"attributes":{"enabled":[],"reboot_required":false},"meta":{"environment_vars":[]},"type":"FullTokenAttachResult"},
  "errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]
}`)}
	token := []byte(tokenCanary)
	result, err := NewAPIClient(runner).FullTokenAttach(token)
	if err != nil {
		t.Fatal(err)
	}
	if runner.inputCalls != 1 || runner.runCalls != 0 {
		t.Fatalf("RunInput calls = %d, ordinary Run calls = %d", runner.inputCalls, runner.runCalls)
	}
	wantArgs := []string{"api", "u.pro.attach.token.full_token_attach.v1", "--data", "-"}
	if runner.name != "/usr/bin/pro" || !slices.Equal(runner.args, wantArgs) {
		t.Fatalf("process boundary = %q %q, want /usr/bin/pro %q", runner.name, runner.args, wantArgs)
	}
	wantInput := []byte(`{"token":"` + tokenCanary + `","auto_enable_services":false}`)
	if !bytes.Equal(runner.input, wantInput) {
		t.Fatalf("protected stdin = %s, want exact typed request %s", runner.input, wantInput)
	}
	argv := runner.name + " " + strings.Join(runner.args, " ")
	for _, forbidden := range []string{tokenCanary, "--args", " attach ", " enable ", " disable ", " status ", "sh -c"} {
		if strings.Contains(argv, forbidden) {
			t.Fatalf("unsafe process boundary contains %q: %s", forbidden, argv)
		}
	}
	for index, value := range token {
		if value != 0 {
			t.Fatalf("caller token byte %d was not cleared", index)
		}
	}
	if len(result.Enabled) != 0 || result.RebootRequired || result.ClientVersion != "32.3ubuntu0" {
		t.Fatalf("attach result = %#v", result)
	}
}
