package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/ubuntupro"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/internal/ubuntuproqualification"
)

const ubuntuProAcceptanceToken = "ubuntu-pro-acceptance-synthetic-canary"

func TestUbuntuProManagementWorkflow(t *testing.T) {
	state := &ubuntuProAcceptanceState{}
	RunFeatureFiles(t, []string{filepath.Join("features", "ubuntu_pro_management.feature")}, func(steps *ScenarioSteps) {
		steps.Step(`^an authenticated Ubuntu Pro workflow with an active synthetic token$`, state.startUnattached)
		steps.Step(`^an authenticated pre-attached Ubuntu Pro workflow$`, state.startAttached)
		steps.Step(`^an authenticated pre-attached Ubuntu Pro workflow with FIPS enabled$`, state.startAttachedWithFIPS)
		steps.Step(`^the exact Ubuntu endpoint Syncs and applies attachment$`, state.syncAndAttach)
		steps.Step(`^fleet state reports attached without exposing the token$`, state.assertAttachedRedacted)
		steps.Step(`^the operator adds the ordinary service "([^"]*)"$`, state.addOrdinaryService)
		steps.Step(`^the service converges and an idempotent resync makes no change$`, state.assertServiceAndResync)
		steps.Step(`^the operator explicitly detaches Ubuntu Pro$`, state.detach)
		steps.Step(`^fleet state reports detached$`, state.assertDetached)
		steps.Step(`^the operator replaces FIPS with Livepatch$`, state.replaceFIPSWithLivepatch)
		steps.Step(`^the owned conflict is disabled before Livepatch is enabled$`, state.assertConflictOrder)
		steps.Step(`^the operator requests access-only ESM Apps$`, state.requestAccessOnly)
		steps.Step(`^authenticated Sync blocks the unadvertised specialized capability$`, state.assertCapabilityBlocked)
		steps.Step(`^the operator enables FIPS through the mock API$`, state.enableFIPS)
		steps.Step(`^fleet state reports reboot required without rebooting$`, state.assertRebootReported)
	})
}

type ubuntuProAcceptanceState struct {
	runner        *ubuntuProAcceptanceRunner
	resource      models.UbuntuProResource
	lastApply     executor.ApplyResult
	current       *ubuntupro.Applicator
	authenticated bool
	tokenActive   bool
	blocked       bool
}

func (s *ubuntuProAcceptanceState) reset(attached bool, services map[string]string) {
	*s = ubuntuProAcceptanceState{
		runner:        &ubuntuProAcceptanceRunner{attached: attached, enabled: services},
		authenticated: true,
		tokenActive:   true,
	}
}

func (s *ubuntuProAcceptanceState) startUnattached() error {
	s.reset(false, map[string]string{})
	return nil
}

func (s *ubuntuProAcceptanceState) startAttached() error {
	s.reset(true, map[string]string{})
	return nil
}

func (s *ubuntuProAcceptanceState) startAttachedWithFIPS() error {
	s.reset(true, map[string]string{"fips": ""})
	s.runner.livepatchConflictsWithFIPS = true
	return nil
}

func (s *ubuntuProAcceptanceState) syncAndAttach() error {
	resource, err := parseUbuntuProAcceptanceResource("attached", "")
	if err != nil {
		return err
	}
	return s.applyAuthenticated(resource, executor.Changed)
}

func (s *ubuntuProAcceptanceState) assertAttachedRedacted() error {
	check := s.check()
	report, ok := check.Actual.(ubuntupro.StateReport)
	if check.Status != executor.Compliant || !ok || report.Attachment != ubuntupro.AttachmentAttached {
		return fmt.Errorf("fleet Check = %+v", check)
	}
	if strings.Contains(fmt.Sprintf("%+v", []any{s.lastApply, check}), ubuntuProAcceptanceToken) {
		return fmt.Errorf("fleet-visible result exposed the synthetic token")
	}
	if !s.runner.attachSawSyntheticToken {
		return fmt.Errorf("protected attachment boundary did not receive the synthetic token")
	}
	return nil
}

func (s *ubuntuProAcceptanceState) addOrdinaryService(service string) error {
	resource, err := parseUbuntuProAcceptanceResource("attached", fmt.Sprintf("\n        services:\n          - name: %s\n            state: enabled", service))
	if err != nil {
		return err
	}
	return s.applyAuthenticated(resource, executor.Changed)
}

func (s *ubuntuProAcceptanceState) assertServiceAndResync() error {
	check := s.check()
	if check.Status != executor.Compliant {
		return fmt.Errorf("service Check = %+v", check)
	}
	if err := s.applyAuthenticated(s.resource, executor.NoChange); err != nil {
		return fmt.Errorf("idempotent resync: %w", err)
	}
	return nil
}

func (s *ubuntuProAcceptanceState) detach() error {
	resource, err := parseUbuntuProAcceptanceResource("detached", "")
	if err != nil {
		return err
	}
	return s.applyAuthenticated(resource, executor.Changed)
}

func (s *ubuntuProAcceptanceState) assertDetached() error {
	check := s.check()
	report, ok := check.Actual.(ubuntupro.StateReport)
	if check.Status != executor.Compliant || !ok || report.Attachment != ubuntupro.AttachmentUnattached {
		return fmt.Errorf("detached fleet Check = %+v", check)
	}
	return nil
}

func (s *ubuntuProAcceptanceState) replaceFIPSWithLivepatch() error {
	resource, err := parseUbuntuProAcceptanceResource("attached", "\n        services:\n          - name: livepatch\n            state: enabled\n          - name: fips\n            state: disabled")
	if err != nil {
		return err
	}
	return s.applyAuthenticated(resource, executor.Changed)
}

func (s *ubuntuProAcceptanceState) assertConflictOrder() error {
	want := []string{"disable:fips", "enable:livepatch"}
	if !slices.Equal(s.runner.mutations, want) {
		return fmt.Errorf("mutation order = %v, want %v", s.runner.mutations, want)
	}
	if check := s.check(); check.Status != executor.Compliant {
		return fmt.Errorf("post-conflict Check = %+v", check)
	}
	return nil
}

func (s *ubuntuProAcceptanceState) requestAccessOnly() error {
	resource, err := parseUbuntuProAcceptanceResource("attached", "\n        services:\n          - name: esm-apps\n            state: enabled\n            enableMode: access-only")
	if err != nil {
		return err
	}
	s.resource = resource
	qualification, err := ubuntuproqualification.Load(filepath.Join(repositoryRoot(), "test", "qualification", "ubuntu-pro.yaml"))
	if err != nil {
		return err
	}
	capabilities := qualification.AdvertisedCapabilities(ubuntuproqualification.Target{
		Distribution: "ubuntu", Release: "24.04", Architecture: "amd64", APIRevision: "ubuntu-pro-api-v32",
	})
	s.blocked = !slices.Contains(capabilities, "provider:ubuntu-pro-option/esm-apps/access-only")
	return nil
}

func (s *ubuntuProAcceptanceState) assertCapabilityBlocked() error {
	if !s.blocked || len(s.runner.mutations) != 0 {
		return fmt.Errorf("access-only capability blocked=%t mutations=%v", s.blocked, s.runner.mutations)
	}
	return nil
}

func (s *ubuntuProAcceptanceState) enableFIPS() error {
	s.runner.rebootOnEnable = true
	resource, err := parseUbuntuProAcceptanceResource("attached", "\n        services:\n          - name: fips\n            state: enabled")
	if err != nil {
		return err
	}
	return s.applyAuthenticated(resource, executor.Changed)
}

func (s *ubuntuProAcceptanceState) assertRebootReported() error {
	if s.lastApply.RebootRequired != executor.RebootRequired {
		return fmt.Errorf("Apply reboot requirement = %s", s.lastApply.RebootRequired)
	}
	check := s.check()
	report, ok := check.Actual.(ubuntupro.StateReport)
	if check.Status != executor.Compliant || !ok || report.RebootRequired != executor.RebootRequired {
		return fmt.Errorf("fleet reboot report = %+v", check)
	}
	for _, call := range append(append([]string{}, s.runner.reads...), s.runner.mutations...) {
		if strings.Contains(call, "reboot") {
			return fmt.Errorf("provider crossed a reboot execution boundary: %v", call)
		}
	}
	return nil
}

func (s *ubuntuProAcceptanceState) applyAuthenticated(resource models.UbuntuProResource, want executor.ApplyStatus) error {
	if !s.authenticated {
		return fmt.Errorf("Sync is not authenticated")
	}
	s.resource = resource
	s.current = s.applicator()
	s.lastApply = executor.New().Apply(context.Background(), s.current)
	if s.lastApply.Status != want {
		return fmt.Errorf("Apply = %+v, want %s", s.lastApply, want)
	}
	return nil
}

func (s *ubuntuProAcceptanceState) check() executor.CheckResult {
	if s.current == nil {
		s.current = s.applicator()
	}
	return executor.Check(context.Background(), s.current)
}

func (s *ubuntuProAcceptanceState) applicator() *ubuntupro.Applicator {
	return ubuntupro.New(s.resource, facts.Facts{
		Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86, Package: types.Apt,
		OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
	}, s.runner, func(context.Context, string) ([]byte, error) {
		if !s.tokenActive {
			return nil, fmt.Errorf("synthetic token is inactive")
		}
		return []byte(ubuntuProAcceptanceToken), nil
	})
}

func parseUbuntuProAcceptanceResource(lifecycle, serviceBlock string) (models.UbuntuProResource, error) {
	token := "\n        tokenRef: remotr:ubuntu-pro/acceptance@active"
	if lifecycle == "detached" {
		token = ""
	}
	document := fmt.Sprintf("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: ubuntuPro\n        name: subscription\n        lifecycle: %s%s%s\n", lifecycle, token, serviceBlock)
	state, err := models.ParseState(strings.NewReader(document))
	if err != nil {
		return models.UbuntuProResource{}, fmt.Errorf("public authoring rejected Ubuntu Pro workflow: %w", err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].UbuntuPro) != 1 {
		return models.UbuntuProResource{}, fmt.Errorf("public authoring omitted Ubuntu Pro resource")
	}
	return state.Configurations[0].UbuntuPro[0], nil
}

type ubuntuProAcceptanceRunner struct {
	attached                   bool
	enabled                    map[string]string
	livepatchConflictsWithFIPS bool
	rebootOnEnable             bool
	attachSawSyntheticToken    bool
	reads                      []string
	mutations                  []string
}

func (r *ubuntuProAcceptanceRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return r.RunContext(context.Background(), name, args...)
}

func (r *ubuntuProAcceptanceRunner) RunContext(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if name != "/usr/bin/pro" || len(args) != 2 || args[0] != "api" {
		return nil, nil, fmt.Errorf("unexpected read boundary: %s %v", name, args)
	}
	endpoint := args[1]
	r.reads = append(r.reads, endpoint)
	switch endpoint {
	case "u.pro.status.is_attached.v1":
		return ubuntuProAcceptanceEnvelope("IsAttachedResult", fmt.Sprintf(`{"is_attached":%t}`, r.attached)), nil, nil
	case "u.pro.status.enabled_services.v1":
		names := make([]string, 0, len(r.enabled))
		for service := range r.enabled {
			names = append(names, service)
		}
		sort.Strings(names)
		items := make([]string, 0, len(names))
		for _, service := range names {
			variant := r.enabled[service]
			if variant == "" {
				items = append(items, fmt.Sprintf(`{"name":%q,"variant_enabled":false,"variant_name":null}`, service))
			} else {
				items = append(items, fmt.Sprintf(`{"name":%q,"variant_enabled":true,"variant_name":%q}`, service, variant))
			}
		}
		return ubuntuProAcceptanceEnvelope("EnabledServicesResult", `{"enabled_services":[`+strings.Join(items, ",")+`]}`), nil, nil
	case "u.pro.services.dependencies.v1":
		services := "[]"
		if r.livepatchConflictsWithFIPS {
			services = `[{"name":"livepatch","depends_on":[],"incompatible_with":[{"name":"fips","reason":{"code":"livepatch-conflicts-fips"}}]}]`
		}
		return ubuntuProAcceptanceEnvelope("DependenciesResult", `{"services":`+services+`}`), nil, nil
	case "u.pro.detach.v1":
		r.attached = false
		r.enabled = map[string]string{}
		r.mutations = append(r.mutations, "detach")
		return ubuntuProAcceptanceEnvelope("DetachResult", `{"disabled":[],"reboot_required":false}`), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected read endpoint %s", endpoint)
	}
}

func (r *ubuntuProAcceptanceRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return r.RunInputContext(context.Background(), name, input, args...)
}

func (r *ubuntuProAcceptanceRunner) RunInputContext(ctx context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if name != "/usr/bin/pro" || len(args) != 4 || args[0] != "api" || args[2] != "--data" || args[3] != "-" {
		return nil, nil, fmt.Errorf("unexpected mutation boundary: %s %v", name, args)
	}
	switch args[1] {
	case "u.pro.attach.token.full_token_attach.v1":
		var request struct {
			Token              string `json:"token"`
			AutoEnableServices bool   `json:"auto_enable_services"`
		}
		if err := json.Unmarshal(input, &request); err != nil || request.AutoEnableServices {
			return nil, nil, fmt.Errorf("invalid protected attachment request")
		}
		r.attachSawSyntheticToken = request.Token == ubuntuProAcceptanceToken
		r.attached = true
		r.mutations = append(r.mutations, "attach")
		return ubuntuProAcceptanceEnvelope("FullTokenAttachResult", `{"enabled":[],"reboot_required":false}`), nil, nil
	case "u.pro.services.enable.v1":
		var request struct {
			Service string `json:"service"`
			Variant string `json:"variant"`
		}
		if err := json.Unmarshal(input, &request); err != nil || request.Service == "" {
			return nil, nil, fmt.Errorf("invalid protected enable request")
		}
		r.enabled[request.Service] = request.Variant
		r.mutations = append(r.mutations, "enable:"+request.Service)
		reboot := r.rebootOnEnable && request.Service == "fips"
		return ubuntuProAcceptanceEnvelope("ServiceTransitionResult", fmt.Sprintf(`{"enabled":[%q],"disabled":[],"reboot_required":%t}`, request.Service, reboot)), nil, nil
	case "u.pro.services.disable.v1":
		var request struct {
			Service string `json:"service"`
		}
		if err := json.Unmarshal(input, &request); err != nil || request.Service == "" {
			return nil, nil, fmt.Errorf("invalid protected disable request")
		}
		delete(r.enabled, request.Service)
		r.mutations = append(r.mutations, "disable:"+request.Service)
		return ubuntuProAcceptanceEnvelope("ServiceTransitionResult", fmt.Sprintf(`{"enabled":[],"disabled":[%q],"reboot_required":false}`, request.Service)), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected mutation endpoint %s", args[1])
	}
}

func ubuntuProAcceptanceEnvelope(resultType, attributes string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":%s,"meta":{"environment_vars":[]},"type":%q},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attributes, resultType))
}
