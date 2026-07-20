package swaps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"golang.org/x/sys/unix"
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
		active, priority, err := a.active()
		if err != nil {
			return failed(d, err)
		}
		priorityDrift := active && *a.Resource.Active && a.Resource.Priority != 0 && priority != a.Resource.Priority
		if active != *a.Resource.Active || priorityDrift {
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
	var restoreFstab func() error
	if want, managed := a.Resource.DesiredPersistent(); managed {
		body, err := os.ReadFile(a.FstabPath)
		existed := err == nil
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
			mode := os.FileMode(0o644)
			if existed {
				info, err := os.Stat(a.FstabPath)
				if err != nil {
					return err
				}
				mode = info.Mode().Perm()
			}
			previous := append([]byte(nil), body...)
			if err := writeAtomic(a.FstabPath, []byte(next), mode); err != nil {
				return err
			}
			restoreFstab = func() error {
				if existed {
					return writeAtomic(a.FstabPath, previous, mode)
				}
				err := os.Remove(a.FstabPath)
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			changed = true
		}
	}
	if a.Resource.Active != nil {
		active, priority, err := a.active()
		if err != nil {
			return restoreAfterFailure(err, restoreFstab)
		}
		priorityDrift := active && *a.Resource.Active && a.Resource.Priority != 0 && priority != a.Resource.Priority
		if active != *a.Resource.Active || priorityDrift {
			if *a.Resource.Active {
				if priorityDrift {
					if _, stderr, err := a.Runner.Run("swapoff", a.Resource.Path); err != nil {
						err = fmt.Errorf("swapoff for priority change: %s: %w", strings.TrimSpace(string(stderr)), err)
						return restoreAfterFailure(err, restoreFstab)
					}
				}
				if err := a.createAndActivate(); err != nil {
					if priorityDrift {
						err = errors.Join(err, a.activate(priority))
					}
					return restoreAfterFailure(err, restoreFstab)
				}
			} else if _, stderr, err := a.Runner.Run("swapoff", a.Resource.Path); err != nil {
				err = fmt.Errorf("swapoff: %s: %w", strings.TrimSpace(string(stderr)), err)
				return restoreAfterFailure(err, restoreFstab)
			}
			changed = true
		}
	}
	if !changed {
		return appErr.ErrStateAlreadyMet
	}
	return nil
}

func restoreAfterFailure(operationErr error, restore func() error) error {
	if restore == nil {
		return operationErr
	}
	return errors.Join(operationErr, restore())
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
			return ensureSwapFileCapacity(a.Resource.Path, a.Resource.SizeBytes)
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
		if info.Size() != a.Resource.SizeBytes {
			return fmt.Errorf("swap file %q does not match declared size", a.Resource.Path)
		}
		if info.Mode().Perm() != 0o600 {
			return fmt.Errorf("swap file %q does not have protected mode 0600", a.Resource.Path)
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

func ensureSwapFileCapacity(path string, requiredBytes int64) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(filepath.Dir(path), &stat); err != nil {
		return fmt.Errorf("inspect swap-file capacity for %q: %w", path, err)
	}
	blockSize := uint64(stat.Bsize)
	if blockSize == 0 {
		return fmt.Errorf("swap-file capacity for %q reported a zero block size", path)
	}
	requiredBlocks := (uint64(requiredBytes) + blockSize - 1) / blockSize
	if requiredBlocks > uint64(stat.Bavail) {
		return fmt.Errorf("insufficient filesystem capacity for swap file %q", path)
	}
	return nil
}

func (a *Applicator) active() (bool, int, error) {
	body, err := os.ReadFile(a.SwapsPath)
	if err != nil {
		return false, 0, err
	}
	for _, l := range strings.Split(string(body), "\n") {
		fields := strings.Fields(l)
		if len(fields) > 0 && fields[0] == a.Resource.Path {
			if len(fields) < 5 {
				return false, 0, fmt.Errorf("swap state for %q is malformed", a.Resource.Path)
			}
			priority, err := strconv.Atoi(fields[4])
			if err != nil {
				return false, 0, fmt.Errorf("swap priority for %q is invalid: %w", a.Resource.Path, err)
			}
			return true, priority, nil
		}
	}
	return false, 0, nil
}
func (a *Applicator) createAndActivate() error {
	created := false
	if a.Resource.Type == "file" {
		if _, err := os.Lstat(a.Resource.Path); os.IsNotExist(err) {
			created = true
			count := (a.Resource.SizeBytes + (1 << 20) - 1) / (1 << 20)
			if _, stderr, err := a.Runner.Run("dd", "if=/dev/zero", "of="+a.Resource.Path, "bs=1M", fmt.Sprintf("count=%d", count), "conv=fsync"); err != nil {
				return cleanupCreatedSwapFile(a.Resource.Path, fmt.Errorf("create swap file: %s: %w", strings.TrimSpace(string(stderr)), err))
			}
			if err := os.Truncate(a.Resource.Path, a.Resource.SizeBytes); err != nil {
				return cleanupCreatedSwapFile(a.Resource.Path, err)
			}
			if err := os.Chmod(a.Resource.Path, 0o600); err != nil {
				return cleanupCreatedSwapFile(a.Resource.Path, err)
			}
			if _, stderr, err := a.Runner.Run("mkswap", a.Resource.Path); err != nil {
				return cleanupCreatedSwapFile(a.Resource.Path, fmt.Errorf("format swap: %s: %w", strings.TrimSpace(string(stderr)), err))
			}
		}
	}
	err := a.activate(a.Resource.Priority)
	if err != nil {
		if created {
			return cleanupCreatedSwapFile(a.Resource.Path, err)
		}
		return err
	}
	return nil
}

func (a *Applicator) activate(priority int) error {
	args := []string{}
	if priority != 0 {
		args = []string{"--priority", fmt.Sprint(priority)}
	}
	args = append(args, a.Resource.Path)
	_, stderr, err := a.Runner.Run("swapon", args...)
	if err != nil {
		return fmt.Errorf("swapon: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func cleanupCreatedSwapFile(path string, operationErr error) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		err = nil
	}
	return errors.Join(operationErr, err)
}

func (a *Applicator) marker() string { return "# remotr:" + a.Resource.Name }
func (a *Applicator) entry() string {
	opt := "defaults"
	if a.Resource.Priority != 0 {
		opt = "pri=" + fmt.Sprint(a.Resource.Priority)
	}
	return a.Resource.Path + " none swap " + opt + " 0 0 " + a.marker() + "\n"
}
func writeAtomic(path string, b []byte, mode os.FileMode) error {
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
	if err = f.Chmod(mode); err != nil {
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
