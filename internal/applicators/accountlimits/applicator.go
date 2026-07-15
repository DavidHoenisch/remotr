// Package accountlimits manages named pam_limits fragments.
package accountlimits

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource       models.AccountLimitResource
	LimitsDir      string
	previous       []byte
	previousExists bool
	armed          bool
}

func New(resource models.AccountLimitResource) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	return &Applicator{Resource: resource, LimitsDir: "/etc/security/limits.d"}
}

func (a *Applicator) Name() string { return "account-limits:" + a.Resource.Name }

func (a *Applicator) Description() string { return "account limit fragment " + a.Resource.Name }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.LimitsDir) {
		return "", fmt.Errorf("account limits directory must be absolute")
	}
	return filepath.Join(a.LimitsDir, "90-remotr-"+a.Resource.Name+".conf"), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(_ context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("named account limit fragment " + a.Resource.Name)
	path, err := a.path()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named fragment is absent"}
		}
		if err != nil {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named fragment exists"}
	}
	if os.IsNotExist(err) {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named fragment is absent"}
	}
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	if string(content) == a.render() {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named fragment matches"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named fragment differs"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if err := atomicWrite(path, []byte(a.render())); err != nil {
		return err
	}
	a.previous, a.previousExists, a.armed = previous, previousExists, true
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Activation: []executor.ActivationSignal{{Kind: executor.ActivationLogoutRequired}}}
	}
}

func (a *Applicator) Revert(context.Context) error {
	if !a.armed {
		return appErr.ErrNoOp
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if !a.previousExists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := atomicWrite(path, a.previous); err != nil {
		return err
	}
	a.previous = nil
	a.armed = false
	return nil
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	var out strings.Builder
	for _, entry := range a.Resource.Entries {
		fmt.Fprintf(&out, "%s %s %s %s\n", entry.Domain, entry.Type, entry.Item, entry.Value)
	}
	return out.String()
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-account-limits-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
