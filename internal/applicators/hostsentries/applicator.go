package hostsentries

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const defaultHostsPath = "/etc/hosts"

type Applicator struct {
	Resource       models.HostsEntryResource
	Path           string
	SyncURL        string
	previous       []byte
	previousExists bool
	rollbackArmed  bool
	rollback       *filetx.Handle
}

func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := filetx.New(store, address, artifactDigest, false)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
}

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	return a.rollback.Preflight(ctx, a.Path)
}

func New(resource models.HostsEntryResource) *Applicator {
	return &Applicator{Resource: resource, Path: defaultHostsPath}
}

func (a *Applicator) Name() string { return "hosts-entry:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return fmt.Sprintf("hosts entry %s", a.Resource.Name)
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (a *Applicator) Check(context.Context) executor.CheckResult {
	raw, err := os.ReadFile(a.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && a.Resource.Lifecycle == models.LifecycleAbsent {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}
		}
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, Err: err}
	}
	managed := managedLines(raw, a.marker())
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if len(managed) == 0 {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, ObservedSummary: "owned hosts entry is present", DesiredSummary: "owned hosts entry absent", Actual: managed}
	}
	want := a.desiredLine()
	if len(managed) == 1 && managed[0] == want {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, Actual: managed}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: executor.RedactedSummary(want), ObservedSummary: "owned hosts entry differs", Actual: managed}
}

func (a *Applicator) Preflight(context.Context) error {
	if a.SyncURL == "" {
		return nil
	}
	u, err := url.Parse(a.SyncURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("hostsEntry %q cannot identify active Remotr host", a.Resource.Name)
	}
	active := strings.ToLower(u.Hostname())
	if strings.EqualFold(a.Resource.CanonicalHost, active) {
		return fmt.Errorf("hostsEntry %q would change active Remotr destination %q without a guarded network transaction", a.Resource.Name, active)
	}
	for _, alias := range a.Resource.Aliases {
		if strings.EqualFold(alias, active) {
			return fmt.Errorf("hostsEntry %q would change active Remotr destination %q without a guarded network transaction", a.Resource.Name, active)
		}
	}
	return nil
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status == executor.CheckFailed {
		return check.Err
	}
	raw, err := os.ReadFile(a.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if a.rollback != nil {
		if err := a.rollback.Arm(ctx, a.Path); err != nil {
			return err
		}
	} else {
		a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), raw...), err == nil, true
	}
	next := withoutManagedLines(raw, a.marker())
	if a.Resource.Lifecycle != models.LifecycleAbsent {
		if len(next) > 0 && next[len(next)-1] != '\n' {
			next = append(next, '\n')
		}
		next = append(next, a.desiredLine()...)
		next = append(next, '\n')
	}
	return writeHostsAtomic(a.Path, next)
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
}

func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback != nil {
		err := a.rollback.Rollback(ctx)
		if errors.Is(err, os.ErrNotExist) {
			return appErr.ErrNoOp
		}
		return err
	}
	if !a.rollbackArmed {
		return appErr.ErrNoOp
	}
	if !a.previousExists {
		return os.Remove(a.Path)
	}
	return writeHostsAtomic(a.Path, a.previous)
}

func (a *Applicator) marker() string { return "remotr:" + a.Resource.Name }

func (a *Applicator) desiredLine() string {
	fields := []string{a.Resource.Address, a.Resource.CanonicalHost}
	fields = append(fields, a.Resource.Aliases...)
	return strings.Join(fields, " ") + " # " + a.marker()
}

func managedLines(raw []byte, marker string) []string {
	var found []string
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.SplitN(line, "#", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == marker {
			found = append(found, strings.TrimSpace(line))
		}
	}
	return found
}

func withoutManagedLines(raw []byte, marker string) []byte {
	lines := strings.SplitAfter(string(raw), "\n")
	var out strings.Builder
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSuffix(line, "\n"), "#", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == marker {
			continue
		}
		out.WriteString(line)
	}
	return []byte(out.String())
}

func writeHostsAtomic(path string, data []byte) error {
	info, err := os.Lstat(path)
	mode := os.FileMode(0o644)
	uid, gid := -1, -1
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("hosts path %q must be a regular non-symlink file", path)
		}
		mode = info.Mode().Perm()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(stat.Uid), int(stat.Gid)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".remotr-hosts-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if uid >= 0 {
		if err := tmp.Chown(uid, gid); err != nil {
			tmp.Close()
			return err
		}
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	dirHandle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer dirHandle.Close()
	return dirHandle.Sync()
}
