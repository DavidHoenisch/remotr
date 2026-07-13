// Package sysctl manages one runtime and/or persistent Linux sysctl value.
package sysctl

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

// Applicator owns only the resource's named sysctl.d fragment.
type Applicator struct {
	Resource  models.SysctlResource
	ProcRoot  string
	DropInDir string
	Runner    executil.Runner
}

func New(resource models.SysctlResource, runner executil.Runner) *Applicator {
	if resource.Activation == "" {
		resource.Activation = models.SysctlSingleKey
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, ProcRoot: "/proc/sys", DropInDir: "/etc/sysctl.d", Runner: runner}
}

func (a *Applicator) Name() string { return "sysctl:" + a.Resource.Name }

func (a *Applicator) Description() string { return "sysctl " + a.Resource.Key }

func (a *Applicator) runtimePath() (string, error) {
	if !filepath.IsAbs(a.ProcRoot) {
		return "", errors.New("sysctl proc root must be absolute")
	}
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(a.ProcRoot, strings.ReplaceAll(a.Resource.Key, ".", "/")), nil
}

func (a *Applicator) dropInPath() (string, error) {
	if !filepath.IsAbs(a.DropInDir) {
		return "", errors.New("sysctl drop-in directory must be absolute")
	}
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(a.DropInDir, "99-remotr-"+a.Resource.Name+".conf"), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("sysctl " + a.Resource.Key)
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	if a.Resource.Runtime {
		path, err := a.runtimePath()
		if err != nil {
			return failed(desired, err)
		}
		value, err := os.ReadFile(path) // #nosec G304 -- path is derived from validated sysctl key.
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "sysctl_key_unsupported", DesiredSummary: desired, ObservedSummary: "kernel key is absent"}
		}
		if err != nil {
			return failed(desired, err)
		}
		if strings.TrimSpace(string(value)) != a.Resource.Value {
			return drift(desired, "runtime value differs")
		}
	}
	if a.Resource.Persistent {
		path, err := a.dropInPath()
		if err != nil {
			return failed(desired, err)
		}
		value, err := os.ReadFile(path) // #nosec G304 -- provider constructs an owned fragment path.
		if os.IsNotExist(err) || string(value) != a.dropInContent() {
			return drift(desired, "persistent drop-in differs")
		}
		if err != nil {
			return failed(desired, err)
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status == executor.Unsupported || check.Status == executor.CheckFailed {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("cannot apply %s: %s", a.Resource.Key, check.ReasonCode)
	}
	var dropIn string
	var err error
	if a.Resource.Persistent {
		dropIn, err = a.dropInPath()
		if err != nil {
			return err
		}
		if err := atomicWrite(dropIn, []byte(a.dropInContent()), 0o644); err != nil {
			return err
		}
	}
	if a.Resource.Activation == models.SysctlNextBoot {
		return nil
	}
	if a.Resource.Activation == models.SysctlReload && a.Resource.Persistent {
		_, stderr, err := a.Runner.Run("sysctl", "--load", dropIn)
		if err != nil {
			return fmt.Errorf("reload Remotr sysctl drop-in: %s: %w", strings.TrimSpace(string(stderr)), err)
		}
		return nil
	}
	if a.Resource.Runtime {
		path, err := a.runtimePath()
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(a.Resource.Value+"\n"), 0o644) // #nosec G306 -- kernel pseudo-file mode is ignored.
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
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	if a.Resource.Activation == models.SysctlNextBoot {
		result.Activation = []executor.ActivationSignal{{Kind: executor.ActivationNextBoot}}
	}
	return result
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) dropInContent() string { return a.Resource.Key + " = " + a.Resource.Value + "\n" }

func atomicWrite(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-sysctl-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func drift(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}

func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}
