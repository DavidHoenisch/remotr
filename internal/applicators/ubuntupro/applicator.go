package ubuntupro

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

type TokenResolver func(context.Context, string) ([]byte, error)

type AttachmentState string

const (
	AttachmentAttached   AttachmentState = "attached"
	AttachmentUnattached AttachmentState = "unattached"
)

type ServiceState struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Variant string `json:"variant,omitempty"`
}

// StateReport is the bounded, safe projection returned through public Check.
// It intentionally contains no account, contract, token, or raw API data.
type StateReport struct {
	Attachment   AttachmentState `json:"attachment"`
	Services     []ServiceState  `json:"services,omitempty"`
	WarningCodes []string        `json:"warningCodes,omitempty"`
}

type Applicator struct {
	resource models.UbuntuProResource
	facts    facts.Facts
	api      *APIClient
	resolve  TokenResolver
}

func New(resource models.UbuntuProResource, endpoint facts.Facts, runner executil.Runner, resolver TokenResolver) *Applicator {
	return &Applicator{resource: resource, facts: endpoint.Normalized(), api: NewAPIClient(runner), resolve: resolver}
}

func (applicator *Applicator) Name() string { return "ubuntu-pro:" + applicator.resource.Name }

func (applicator *Applicator) Description() string {
	return "Ubuntu Pro subscription attachment"
}

func (applicator *Applicator) State(ctx context.Context) (any, bool) {
	check := applicator.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (applicator *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("Ubuntu Pro attachment is " + string(applicator.resource.Lifecycle))
	if err := applicator.preflight(); err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonPreflightFailed, DesiredSummary: desired, Err: err}
	}
	api := applicator.api.WithContext(ctx)
	status, err := api.IsAttached()
	if err != nil {
		return classifyCheckError(desired, err)
	}
	attachment := AttachmentUnattached
	if status.Attached {
		attachment = AttachmentAttached
	}
	report := StateReport{Attachment: attachment, WarningCodes: slices.Clone(status.WarningCodes)}
	if len(status.WarningCodes) != 0 {
		return warningCheckResult(desired, report)
	}
	desiredAttached := applicator.resource.Lifecycle == models.UbuntuProAttached
	if status.Attached != desiredAttached {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("Ubuntu Pro attachment is " + string(attachment)), Actual: report}
	}
	if !status.Attached || len(applicator.resource.Services) == 0 {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("Ubuntu Pro attachment is " + string(attachment)), Actual: report}
	}

	enabled, err := api.EnabledServices()
	if err != nil {
		return classifyCheckError(desired, err, report)
	}
	report.WarningCodes = slices.Clone(enabled.WarningCodes)
	enabledByName := make(map[string]EnabledService, len(enabled.Services))
	for _, service := range enabled.Services {
		enabledByName[service.Name] = service
	}
	compliant := true
	unobservableMode := false
	for _, declared := range applicator.resource.Services {
		observed, isEnabled := enabledByName[declared.Name]
		report.Services = append(report.Services, ServiceState{Name: declared.Name, Enabled: isEnabled, Variant: observed.Variant})
		wantEnabled := declared.State == models.UbuntuProServiceEnabled
		if isEnabled && declared.EnableMode != "" {
			unobservableMode = true
		}
		if wantEnabled != isEnabled || (isEnabled && declared.Variant != observed.Variant) {
			compliant = false
		}
	}
	if len(report.WarningCodes) != 0 {
		return warningCheckResult(desired, report)
	}
	if unobservableMode {
		return executor.CheckResult{
			Status: executor.Unsupported, ReasonCode: "ubuntu_pro_mode_unobservable", DesiredSummary: desired,
			ObservedSummary: "requested Ubuntu Pro enable mode is not durably observable", Actual: report,
		}
	}
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "declared Ubuntu Pro state matches", Actual: report}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "declared Ubuntu Pro state differs", Actual: report}
}

func warningCheckResult(desired executor.RedactedSummary, report StateReport) executor.CheckResult {
	return executor.CheckResult{
		Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_warning", DesiredSummary: desired,
		ObservedSummary: "Ubuntu Pro API reported a stable warning", Actual: report,
		Err: errors.New("Ubuntu Pro API reported a stable warning"),
	}
}

func classifyCheckError(desired executor.RedactedSummary, err error, reports ...StateReport) executor.CheckResult {
	var actual any
	if len(reports) != 0 {
		actual = reports[0]
	}
	if errors.Is(err, context.Canceled) {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_canceled", DesiredSummary: desired, Actual: actual, Err: context.Canceled}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_timeout", DesiredSummary: desired, Actual: actual, Err: context.DeadlineExceeded}
	}
	if errors.Is(err, errContextRunnerRequired) {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_context_runner_required", DesiredSummary: desired, Actual: actual, Err: errContextRunnerRequired}
	}
	var apiError APIError
	if errors.As(err, &apiError) {
		switch apiError.Code {
		case "invalid-contract":
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_contract_invalid", DesiredSummary: desired, Actual: actual, Err: apiError}
		case "expired-contract":
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: "ubuntu_pro_contract_expired", DesiredSummary: desired, Actual: actual, Err: apiError}
		case "operation-in-progress":
			return executor.CheckResult{Status: executor.Deferred, ReasonCode: executor.ReasonNativeLockContended, DesiredSummary: desired, Actual: actual, Err: executor.ErrNativeLockContended}
		case "service-unavailable":
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "ubuntu_pro_service_unavailable", DesiredSummary: desired, Actual: actual, Err: apiError}
		case "service-not-entitled":
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "ubuntu_pro_service_unentitled", DesiredSummary: desired, Actual: actual, Err: apiError}
		}
	}
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Actual: actual, Err: fmt.Errorf("Ubuntu Pro versioned API probe failed")}
}

func (applicator *Applicator) Apply(ctx context.Context) error {
	if err := applicator.preflight(); err != nil {
		return err
	}
	if applicator.resource.Lifecycle != models.UbuntuProAttached {
		return fmt.Errorf("Ubuntu Pro detachment is not implemented")
	}
	api := applicator.api.WithContext(ctx)
	status, err := api.IsAttached()
	if err != nil {
		return err
	}
	if len(status.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro attachment probe reported a stable warning")
	}
	if !status.Attached {
		if applicator.resolve == nil {
			return fmt.Errorf("Ubuntu Pro token resolver is unavailable")
		}
		token, err := applicator.resolve(ctx, applicator.resource.TokenRef)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return errors.New("Ubuntu Pro token resolution failed")
		}
		defer clear(token)
		result, err := api.FullTokenAttach(token)
		if err != nil {
			var apiError APIError
			if errors.As(err, &apiError) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			recovery := applicator.Check(ctx)
			return fmt.Errorf("Ubuntu Pro attachment outcome is ambiguous after API failure; recovery check: %s", recovery.ReasonCode)
		}
		if len(result.Enabled) != 0 {
			return fmt.Errorf("Ubuntu Pro attachment enabled unexpected services")
		}
		observed, err := api.IsAttached()
		if err != nil {
			return err
		}
		if !observed.Attached || len(observed.WarningCodes) != 0 {
			reason := executor.ReasonStateDrift
			if len(observed.WarningCodes) != 0 {
				reason = "ubuntu_pro_warning"
			}
			return fmt.Errorf("Ubuntu Pro attachment post-check failed: %s", reason)
		}
	}
	return applicator.convergeServices(ctx, api)
}

func (applicator *Applicator) convergeServices(ctx context.Context, api *APIClient) error {
	if len(applicator.resource.Services) == 0 {
		return nil
	}
	for _, declared := range applicator.resource.Services {
		if declared.EnableMode != "" {
			return errors.New("Ubuntu Pro selected enable mode is not durably observable")
		}
	}
	enabled, err := api.EnabledServices()
	if err != nil {
		return err
	}
	if len(enabled.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro enabled-services probe reported a stable warning")
	}
	enabledByName := make(map[string]EnabledService, len(enabled.Services))
	for _, service := range enabled.Services {
		enabledByName[service.Name] = service
	}
	graph, err := api.Dependencies()
	if err != nil {
		return err
	}
	if len(graph.WarningCodes) != 0 {
		return errors.New("Ubuntu Pro dependency probe reported a stable warning")
	}
	relationsByName := make(map[string]ServiceDependencies, len(graph.Services))
	for _, service := range graph.Services {
		relationsByName[service.Name] = service
	}
	declaredByName := make(map[string]models.UbuntuProService, len(applicator.resource.Services))
	for _, service := range applicator.resource.Services {
		declaredByName[service.Name] = service
	}
	for _, declared := range applicator.resource.Services {
		if declared.State != models.UbuntuProServiceEnabled {
			continue
		}
		relations := relationsByName[declared.Name]
		for _, dependency := range relations.DependsOn {
			owned, declaredDependency := declaredByName[dependency.Name]
			if _, alreadyEnabled := enabledByName[dependency.Name]; !alreadyEnabled &&
				(!declaredDependency || owned.State != models.UbuntuProServiceEnabled) {
				return fmt.Errorf("%s: service %s requires explicitly enabled %s", executor.ReasonDependencyBlocked, declared.Name, dependency.Name)
			}
		}
		for _, incompatible := range relations.IncompatibleWith {
			owned, declaredIncompatible := declaredByName[incompatible.Name]
			if _, alreadyEnabled := enabledByName[incompatible.Name]; alreadyEnabled &&
				(!declaredIncompatible || owned.State != models.UbuntuProServiceDisabled) {
				return fmt.Errorf("%s: service %s requires explicitly disabled %s", executor.ReasonDependencyBlocked, declared.Name, incompatible.Name)
			}
		}
	}
	operationOrder, err := serviceOperationOrder(applicator.resource.Services, enabledByName, relationsByName)
	if err != nil {
		return err
	}
	changed := false
	for _, declared := range operationOrder {
		observed, isEnabled := enabledByName[declared.Name]
		wantEnabled := declared.State == models.UbuntuProServiceEnabled
		if wantEnabled && (!isEnabled || observed.Variant != declared.Variant) {
			transition, err := api.Enable(declared.Name, declared.Variant, false)
			if err != nil {
				return err
			}
			if len(transition.WarningCodes) != 0 || !slices.Equal(transition.Enabled, []string{declared.Name}) || len(transition.Disabled) != 0 {
				return errors.New("Ubuntu Pro enable operation returned unexpected effects")
			}
			changed = true
		} else if !wantEnabled && isEnabled {
			purge := declared.DisableMode == models.UbuntuProPurgePackages
			transition, err := api.Disable(declared.Name, purge)
			if err != nil {
				return err
			}
			if len(transition.WarningCodes) != 0 || !slices.Equal(transition.Disabled, []string{declared.Name}) || len(transition.Enabled) != 0 {
				return errors.New("Ubuntu Pro disable operation returned unexpected effects")
			}
			changed = true
		}
	}
	if !changed {
		return nil
	}
	check := applicator.Check(ctx)
	if check.Status != executor.Compliant {
		return fmt.Errorf("Ubuntu Pro service post-check failed: %s", check.ReasonCode)
	}
	return nil
}

func serviceOperationOrder(
	declared []models.UbuntuProService,
	enabled map[string]EnabledService,
	graph map[string]ServiceDependencies,
) ([]models.UbuntuProService, error) {
	byName := make(map[string]models.UbuntuProService, len(declared))
	for _, service := range declared {
		byName[service.Name] = service
	}
	preDisable := make(map[string]bool)
	for _, service := range declared {
		if service.State != models.UbuntuProServiceEnabled {
			continue
		}
		for _, incompatible := range graph[service.Name].IncompatibleWith {
			owned, ok := byName[incompatible.Name]
			if _, active := enabled[incompatible.Name]; active && ok && owned.State == models.UbuntuProServiceDisabled {
				preDisable[incompatible.Name] = true
			}
		}
	}
	order := make([]models.UbuntuProService, 0, len(declared))
	preNames := make([]string, 0, len(preDisable))
	for name := range preDisable {
		preNames = append(preNames, name)
	}
	sort.Strings(preNames)
	for _, name := range preNames {
		order = append(order, byName[name])
	}

	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 2 {
			return nil
		}
		if state[name] == 1 {
			return fmt.Errorf("%s: Ubuntu Pro dependency graph contains a cycle", executor.ReasonDependencyBlocked)
		}
		service, ok := byName[name]
		if !ok || service.State != models.UbuntuProServiceEnabled {
			return nil
		}
		state[name] = 1
		dependencies := make([]string, 0, len(graph[name].DependsOn))
		for _, dependency := range graph[name].DependsOn {
			owned, declaredDependency := byName[dependency.Name]
			if declaredDependency && owned.State == models.UbuntuProServiceEnabled {
				dependencies = append(dependencies, dependency.Name)
			}
		}
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[name] = 2
		order = append(order, service)
		return nil
	}
	enableNames := make([]string, 0, len(declared))
	for _, service := range declared {
		if service.State == models.UbuntuProServiceEnabled {
			enableNames = append(enableNames, service.Name)
		}
	}
	sort.Strings(enableNames)
	for _, name := range enableNames {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	disableNames := make([]string, 0, len(declared))
	for _, service := range declared {
		if service.State == models.UbuntuProServiceDisabled && !preDisable[service.Name] {
			disableNames = append(disableNames, service.Name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(disableNames)))
	for _, name := range disableNames {
		order = append(order, byName[name])
	}
	return order, nil
}

func (applicator *Applicator) Revert(context.Context) error { return nil }

func (applicator *Applicator) preflight() error {
	if err := applicator.resource.Validate(); err != nil {
		return err
	}
	if applicator.resource.Landscape != nil {
		return fmt.Errorf("Ubuntu Pro Landscape provider is unsupported without a protected native secret transport")
	}
	if !applicator.facts.ExactUbuntu() || applicator.facts.Arch != types.X86 || applicator.facts.Package != types.Apt ||
		!slices.Contains([]string{"20.04", "22.04", "24.04", "26.04"}, applicator.facts.DistroVersion) {
		return fmt.Errorf("Ubuntu Pro provider is unsupported on this endpoint")
	}
	return nil
}
