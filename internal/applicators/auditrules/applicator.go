// Package auditrules manages named Linux audit rule fragments.
package auditrules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource          models.AuditRulesResource
	RulesDir          string
	Runner            executil.Runner
	ObserveImmutable  func(context.Context) (bool, error)
	ObserveLoaded     func(context.Context, []string) (bool, error)
	ValidateEffective func(context.Context, string) error
	LoadEffective     func(context.Context) error
	previous          []byte
	previousExists    bool
	armed             bool
	lastImmutable     bool
}

func New(resource models.AuditRulesResource, runner executil.Runner) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	applicator := &Applicator{Resource: resource, RulesDir: "/etc/audit/rules.d", Runner: runner}
	applicator.ObserveImmutable = applicator.observeImmutable
	applicator.ObserveLoaded = applicator.observeLoaded
	applicator.ValidateEffective = applicator.validateEffective
	applicator.LoadEffective = applicator.loadEffective
	return applicator
}

func (a *Applicator) Name() string { return "audit-rules:" + a.Resource.Name }

func (a *Applicator) Description() string { return "audit rule fragment " + a.Resource.Name }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.RulesDir) {
		return "", fmt.Errorf("audit rules directory must be absolute")
	}
	return filepath.Join(a.RulesDir, "remotr-"+a.Resource.Name+".rules"), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("named audit rule fragment " + a.Resource.Name)
	path, err := a.path()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	persistent := false
	content, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		persistent = os.IsNotExist(err)
		if err != nil && !os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
	} else {
		if err != nil && !os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
		persistent = err == nil && string(content) == a.render()
	}
	loaded := a.Resource.Lifecycle == models.LifecycleAbsent && persistent
	if a.Resource.Lifecycle != models.LifecycleAbsent {
		loaded, err = a.ObserveLoaded(ctx, a.Resource.Rules)
		if err != nil {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
	}
	immutable, err := a.ObserveImmutable(ctx)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	observed := executor.RedactedSummary(fmt.Sprintf("persistent=%t loaded=%t immutable=%t", persistent, loaded, immutable))
	if persistent && loaded {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: observed}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: observed}
}

func (a *Applicator) Apply(ctx context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if err := os.MkdirAll(a.RulesDir, 0o750); err != nil {
		return err
	}
	staged, err := stage(path, []byte(a.render()))
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	if err := a.ValidateEffective(ctx, staged); err != nil {
		return fmt.Errorf("validate effective audit rules: %w", err)
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	immutable, err := a.ObserveImmutable(ctx)
	if err != nil {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := os.Rename(staged, path); err != nil {
		return err
	}
	if !immutable {
		if err := a.LoadEffective(ctx); err != nil {
			_ = restore(path, previous, previousExists)
			return fmt.Errorf("load effective audit rules: %w", err)
		}
	}
	a.previous, a.previousExists, a.armed, a.lastImmutable = previous, previousExists, true, immutable
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
	case a.lastImmutable:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootRequired, RollbackClass: executor.RollbackBestEffort, Activation: []executor.ActivationSignal{{Kind: executor.ActivationRebootRequired}}}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	}
}

func (a *Applicator) Revert(ctx context.Context) error {
	if !a.armed {
		return appErr.ErrNoOp
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if err := restore(path, a.previous, a.previousExists); err != nil {
		return err
	}
	if !a.lastImmutable {
		if err := a.LoadEffective(ctx); err != nil {
			return err
		}
	}
	a.previous = nil
	a.armed = false
	return nil
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	return strings.Join(a.Resource.Rules, "\n") + "\n"
}

func (a *Applicator) observeImmutable(_ context.Context) (bool, error) {
	stdout, stderr, err := a.Runner.Run("auditctl", "-s")
	if err != nil {
		return false, redactedCommandError("auditctl status", stderr, err)
	}
	for _, line := range strings.Split(string(stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "enabled" {
			return fields[1] == "2", nil
		}
	}
	return false, errors.New("auditctl status omitted enabled state")
}

func (a *Applicator) observeLoaded(_ context.Context, rules []string) (bool, error) {
	stdout, stderr, err := a.Runner.Run("auditctl", "-l")
	if err != nil {
		return false, redactedCommandError("auditctl list", stderr, err)
	}
	loaded := make(map[string]struct{})
	for _, line := range strings.Split(string(stdout), "\n") {
		loaded[strings.TrimSpace(line)] = struct{}{}
	}
	for _, rule := range rules {
		if _, ok := loaded[rule]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func (a *Applicator) validateEffective(_ context.Context, _ string) error {
	_, stderr, err := a.Runner.Run("augenrules", "--check")
	return redactedCommandError("augenrules check", stderr, err)
}

func (a *Applicator) loadEffective(_ context.Context) error {
	_, stderr, err := a.Runner.Run("augenrules", "--load")
	return redactedCommandError("augenrules load", stderr, err)
}

func redactedCommandError(operation string, stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	if len(stderr) > 0 {
		return fmt.Errorf("%s diagnostic was redacted: %w", operation, err)
	}
	return err
}

func stage(path string, content []byte) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-audit-rules-")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Chmod(0o640); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func restore(path string, previous []byte, exists bool) error {
	if !exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	staged, err := stage(path, previous)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	return os.Rename(staged, path)
}
