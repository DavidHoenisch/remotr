package swaps

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
	Resource             models.SwapResource
	Runner               executil.Runner
	FstabPath, SwapsPath string
}

func New(resource models.SwapResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner, FstabPath: "/etc/fstab", SwapsPath: "/proc/swaps"}
}
func (a *Applicator) Name() string        { return "swap:" + a.Resource.Name }
func (a *Applicator) Description() string { return "swap " + a.Resource.Path }
func (a *Applicator) State(ctx context.Context) (any, bool) {
	c := a.Check(ctx)
	return c.ObservedSummary, c.Status == executor.Compliant
}
func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	d := executor.RedactedSummary("swap " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return failed(d, err)
	}
	if err := a.preflight(); err != nil {
		return failed(d, err)
	}
	if a.Resource.Active != nil {
		active, err := a.active()
		if err != nil {
			return failed(d, err)
		}
		if active != *a.Resource.Active {
			return drifted(d, "active swap state differs")
		}
	}
	if want, managed := a.Resource.DesiredPersistent(); managed {
		body, err := os.ReadFile(a.FstabPath)
		if err != nil && !os.IsNotExist(err) {
			return failed(d, err)
		}
		if (strings.Contains(string(body), a.marker())) != want {
			return drifted(d, "owned swap declaration differs")
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: d}
}
func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.preflight(); err != nil {
		return err
	}
	changed := false
	if a.Resource.Active != nil {
		active, err := a.active()
		if err != nil {
			return err
		}
		if active != *a.Resource.Active {
			if *a.Resource.Active {
				if err := a.createAndActivate(); err != nil {
					return err
				}
			} else if _, stderr, err := a.Runner.Run("swapoff", a.Resource.Path); err != nil {
				return fmt.Errorf("swapoff: %s: %w", strings.TrimSpace(string(stderr)), err)
			}
			changed = true
		}
	}
	if want, managed := a.Resource.DesiredPersistent(); managed {
		body, err := os.ReadFile(a.FstabPath)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		lines := []string{}
		for _, line := range strings.SplitAfter(string(body), "\n") {
			if strings.Contains(line, a.marker()) {
				continue
			}
			lines = append(lines, line)
		}
		next := strings.Join(lines, "")
		if want {
			next += a.entry()
		}
		if next != string(body) {
			if err := writeAtomic(a.FstabPath, []byte(next)); err != nil {
				return err
			}
			changed = true
		}
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

func (a *Applicator) preflight() error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	wantActive := a.Resource.Active != nil && *a.Resource.Active
	wantPersistent, managesPersistent := a.Resource.DesiredPersistent()
	if !wantActive && !(managesPersistent && wantPersistent) {
		return nil
	}
	if a.Resource.Type == "file" {
		info, err := os.Lstat(a.Resource.Path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect swap file %q: %w", a.Resource.Path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("swap file %q is a symbolic link", a.Resource.Path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("swap file %q is not a regular file", a.Resource.Path)
		}
		return nil
	}
	info, err := os.Stat(a.Resource.Path)
	if err != nil {
		return fmt.Errorf("inspect swap block device %q: %w", a.Resource.Path, err)
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return fmt.Errorf("swap device %q is not a block device", a.Resource.Path)
	}
	return nil
}

func (a *Applicator) active() (bool, error) {
	body, err := os.ReadFile(a.SwapsPath)
	if err != nil {
		return false, err
	}
	for _, l := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(l, a.Resource.Path+" ") {
			return true, nil
		}
	}
	return false, nil
}
func (a *Applicator) createAndActivate() error {
	if a.Resource.Type == "file" {
		if _, err := os.Stat(a.Resource.Path); os.IsNotExist(err) {
			count := (a.Resource.SizeBytes + (1 << 20) - 1) / (1 << 20)
			if _, stderr, err := a.Runner.Run("dd", "if=/dev/zero", "of="+a.Resource.Path, "bs=1M", fmt.Sprintf("count=%d", count), "conv=fsync"); err != nil {
				return fmt.Errorf("create swap file: %s: %w", strings.TrimSpace(string(stderr)), err)
			}
			if err := os.Chmod(a.Resource.Path, 0o600); err != nil {
				return err
			}
			if _, stderr, err := a.Runner.Run("mkswap", a.Resource.Path); err != nil {
				return fmt.Errorf("format swap: %s: %w", strings.TrimSpace(string(stderr)), err)
			}
		}
	}
	args := []string{}
	if a.Resource.Priority != 0 {
		args = []string{"--priority", fmt.Sprint(a.Resource.Priority)}
	}
	args = append(args, a.Resource.Path)
	_, stderr, err := a.Runner.Run("swapon", args...)
	if err != nil {
		return fmt.Errorf("swapon: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}
func (a *Applicator) marker() string { return "# remotr:" + a.Resource.Name }
func (a *Applicator) entry() string {
	opt := "defaults"
	if a.Resource.Priority != 0 {
		opt = "pri=" + fmt.Sprint(a.Resource.Priority)
	}
	return a.Resource.Path + " none swap " + opt + " 0 0 " + a.marker() + "\n"
}
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".remotr-swap-")
	if err != nil {
		return err
	}
	n := f.Name()
	defer os.Remove(n)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(n, path)
}
func drifted(d executor.RedactedSummary, o string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: d, ObservedSummary: executor.RedactedSummary(o)}
}
func failed(d executor.RedactedSummary, e error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: d, Err: e}
}
