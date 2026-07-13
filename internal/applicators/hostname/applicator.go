package hostname

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource models.HostnameResource
	Runner   executil.Runner
}

func New(resource models.HostnameResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner}
}

func (a *Applicator) Name() string        { return "hostname:" + a.Resource.Name }
func (a *Applicator) Description() string { return "hostname " + a.Resource.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("managed hostname " + a.Resource.Name)
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	for _, spec := range []struct {
		flag string
		want *string
	}{{"--static", a.Resource.Static}, {"--transient", a.Resource.Transient}} {
		if spec.want == nil {
			continue
		}
		out, stderr, err := a.Runner.Run("hostnamectl", spec.flag)
		if err != nil {
			return failed(desired, fmt.Errorf("read %s hostname: %s: %w", spec.flag, strings.TrimSpace(string(stderr)), err))
		}
		if strings.TrimSpace(string(out)) != *spec.want {
			return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(spec.flag + " hostname differs")}
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	changed := false
	for _, spec := range []struct {
		flag string
		want *string
	}{{"--static", a.Resource.Static}, {"--transient", a.Resource.Transient}} {
		if spec.want == nil {
			continue
		}
		out, stderr, err := a.Runner.Run("hostnamectl", spec.flag)
		if err != nil {
			return fmt.Errorf("read %s hostname: %s: %w", spec.flag, strings.TrimSpace(string(stderr)), err)
		}
		if strings.TrimSpace(string(out)) == *spec.want {
			continue
		}
		if _, stderr, err := a.Runner.Run("hostnamectl", "set-hostname", spec.flag, *spec.want); err != nil {
			return fmt.Errorf("set %s hostname: %s: %w", spec.flag, strings.TrimSpace(string(stderr)), err)
		}
		changed = true
	}
	if !changed {
		return appErr.ErrStateAlreadyMet
	}
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
}
func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }
func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}
