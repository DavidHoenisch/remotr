package ubuntupro

import (
	"context"
	"errors"
	"fmt"
	"slices"

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

func (applicator *Applicator) Check(context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("Ubuntu Pro attachment is " + string(applicator.resource.Lifecycle))
	if err := applicator.preflight(); err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonPreflightFailed, DesiredSummary: desired, Err: err}
	}
	status, err := applicator.api.IsAttached()
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

	enabled, err := applicator.api.EnabledServices()
	if err != nil {
		return classifyCheckError(desired, err, report)
	}
	report.WarningCodes = slices.Clone(enabled.WarningCodes)
	enabledByName := make(map[string]EnabledService, len(enabled.Services))
	for _, service := range enabled.Services {
		enabledByName[service.Name] = service
	}
	compliant := true
	for _, declared := range applicator.resource.Services {
		observed, isEnabled := enabledByName[declared.Name]
		report.Services = append(report.Services, ServiceState{Name: declared.Name, Enabled: isEnabled, Variant: observed.Variant})
		wantEnabled := declared.State == models.UbuntuProServiceEnabled
		if wantEnabled != isEnabled || (isEnabled && declared.Variant != observed.Variant) {
			compliant = false
		}
	}
	if len(report.WarningCodes) != 0 {
		return warningCheckResult(desired, report)
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
	status, err := applicator.api.IsAttached()
	if err != nil {
		return err
	}
	if status.Attached {
		return nil
	}
	if applicator.resolve == nil {
		return fmt.Errorf("Ubuntu Pro token resolver is unavailable")
	}
	token, err := applicator.resolve(ctx, applicator.resource.TokenRef)
	if err != nil {
		return err
	}
	defer clear(token)
	result, err := applicator.api.FullTokenAttach(token)
	if err != nil {
		return err
	}
	if len(result.Enabled) != 0 {
		return fmt.Errorf("Ubuntu Pro attachment enabled unexpected services")
	}
	observed, err := applicator.api.IsAttached()
	if err != nil {
		return err
	}
	if !observed.Attached {
		return fmt.Errorf("Ubuntu Pro attachment post-check is ambiguous")
	}
	return nil
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
