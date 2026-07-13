// Package systemdunits manages one named systemd unit file or drop-in with
// staged verification and atomic replacement.
package systemdunits

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type LookupOwner func(owner, group string) (uid, gid int, err error)
type ValidateUnit func(context.Context, string, string, string) error

type Applicator struct {
	Resource     models.SystemdUnitResource
	Runner       executil.Runner
	UnitDir      string
	LookupOwner  LookupOwner
	ValidateUnit ValidateUnit
	mu           sync.Mutex
}

func New(resource models.SystemdUnitResource, runner executil.Runner) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Resource: resource, Runner: runner, UnitDir: "/etc/systemd/system", LookupOwner: defaultLookupOwner}
}

func (a *Applicator) Name() string { return "systemd-unit:" + a.Resource.Name }
func (a *Applicator) Description() string {
	if a.Resource.DropIn != "" {
		return "systemd drop-in " + a.Resource.Unit + "/" + a.Resource.DropIn
	}
	return "systemd unit " + a.Resource.Unit
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	result := a.Check(ctx)
	return result.ObservedSummary, result.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary(a.Description())
	if err := ctx.Err(); err != nil {
		return failedCheck(desired, err)
	}
	path, err := a.path()
	if err != nil {
		return failedCheck(desired, err)
	}
	info, err := os.Lstat(path)
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "managed systemd unit artifact is absent"}
		}
		if err != nil {
			return failedCheck(desired, err)
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "managed systemd unit artifact exists"}
	}
	if err != nil {
		if os.IsNotExist(err) {
			return driftedCheck(desired, "managed systemd unit artifact is missing")
		}
		return failedCheck(desired, err)
	}
	if !info.Mode().IsRegular() {
		return driftedCheck(desired, "managed systemd unit artifact is not a regular file")
	}
	uid, gid, err := a.owner()
	if err != nil {
		return failedCheck(desired, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid || int(stat.Gid) != gid || info.Mode().Perm() != a.mode() {
		return driftedCheck(desired, "managed systemd unit metadata differs")
	}
	content, err := os.ReadFile(path) // #nosec G304 -- resource identities produce a validated provider path.
	if err != nil {
		return failedCheck(desired, err)
	}
	if string(content) != a.Resource.Content {
		return driftedCheck(desired, "managed systemd unit content differs")
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "managed systemd unit artifact matches"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status != executor.Drifted {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("systemd unit cannot apply: %s", check.ObservedSummary)
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to remove non-regular systemd unit artifact %s", path)
		}
		return os.Remove(path)
	}
	uid, gid, err := a.owner()
	if err != nil {
		return err
	}
	stageRoot, stagedPath, err := a.stage()
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	if err := os.Chmod(stagedPath, a.mode()); err != nil {
		return err
	}
	if err := os.Chown(stagedPath, uid, gid); err != nil {
		return err
	}
	if err := a.validateStaged(ctx, stageRoot, stagedPath); err != nil {
		return err
	}
	return writeAtomic(path, []byte(a.Resource.Content), a.mode(), uid, gid)
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	return executor.ApplyResult{
		Status: executor.Changed, Activation: []executor.ActivationSignal{{Kind: executor.ActivationDaemonReload}},
		RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone,
	}
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.UnitDir) || filepath.Clean(a.UnitDir) != a.UnitDir {
		return "", fmt.Errorf("systemd unit directory %q must be a clean absolute path", a.UnitDir)
	}
	if a.Runner == nil || a.LookupOwner == nil {
		return "", errors.New("systemd unit provider boundaries are required")
	}
	if a.Resource.DropIn != "" {
		return filepath.Join(a.UnitDir, a.Resource.Unit+".d", a.Resource.DropIn), nil
	}
	return filepath.Join(a.UnitDir, a.Resource.Unit), nil
}

func (a *Applicator) mode() os.FileMode {
	if len(a.Resource.Mode) == 1 {
		return os.FileMode(a.Resource.Mode[0] & 0o777)
	}
	return 0o644
}

func (a *Applicator) owner() (int, int, error) {
	owner, group := a.Resource.Owner, a.Resource.Group
	if owner == "" {
		owner = "root"
	}
	if group == "" {
		group = "root"
	}
	return a.LookupOwner(owner, group)
}

func (a *Applicator) stage() (string, string, error) {
	if err := os.MkdirAll(a.UnitDir, 0o755); err != nil {
		return "", "", err
	}
	root, err := os.MkdirTemp(a.UnitDir, ".remotr-systemd-unit-")
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(root, a.Resource.Unit)
	if a.Resource.DropIn != "" {
		path = filepath.Join(root, a.Resource.Unit+".d", a.Resource.DropIn)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		os.RemoveAll(root)
		return "", "", err
	}
	if err := os.WriteFile(path, []byte(a.Resource.Content), a.mode()); err != nil {
		os.RemoveAll(root)
		return "", "", err
	}
	return root, path, nil
}

func (a *Applicator) validateStaged(ctx context.Context, stageRoot, stagedPath string) error {
	if a.ValidateUnit != nil {
		return a.ValidateUnit(ctx, stageRoot, stagedPath, a.Resource.Unit)
	}
	unitPath := strings.Join([]string{stageRoot, a.UnitDir, "/usr/lib/systemd/system", "/lib/systemd/system"}, ":")
	_, stderr, err := a.Runner.Run("env", "SYSTEMD_UNIT_PATH="+unitPath, "systemd-analyze", "verify", a.Resource.Unit)
	if err != nil {
		return fmt.Errorf("systemd-analyze rejected staged unit %s: %s: %w", a.Resource.Unit, bounded(stderr), err)
	}
	return nil
}

func defaultLookupOwner(owner, group string) (int, int, error) {
	u, err := user.Lookup(owner)
	if err != nil {
		return 0, 0, fmt.Errorf("systemd unit owner %q: %w", owner, err)
	}
	g, err := user.LookupGroup(group)
	if err != nil {
		return 0, 0, fmt.Errorf("systemd unit group %q: %w", group, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func writeAtomic(path string, content []byte, mode os.FileMode, uid, gid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-systemd-unit-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chown(uid, gid); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
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

func driftedCheck(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}

func failedCheck(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, ObservedSummary: "systemd unit check failed", Err: err}
}

func bounded(value []byte) string {
	const limit = 512
	trimmed := strings.TrimSpace(string(value))
	if len(trimmed) > limit {
		trimmed = trimmed[:limit]
	}
	return trimmed
}
