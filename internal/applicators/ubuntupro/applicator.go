package ubuntupro

import (
	"context"
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
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	attachment := AttachmentUnattached
	if status.Attached {
		attachment = AttachmentAttached
	}
	report := StateReport{Attachment: attachment}
	desiredAttached := applicator.resource.Lifecycle == models.UbuntuProAttached
	if status.Attached == desiredAttached {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("Ubuntu Pro attachment is " + string(attachment)), Actual: report}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary("Ubuntu Pro attachment is " + string(attachment)), Actual: report}
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
