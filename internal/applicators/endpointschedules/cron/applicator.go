// Package cron manages endpoint-local schedules through owned /etc/cron.d
// fragments and protected launchers. It is distinct from Remotr server jobs.
package cron

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type LookupUser func(string) (uid, gid int, err error)
type ResolveSecret func(context.Context, string) (string, error)

type Applicator struct {
	Resource         models.EndpointScheduleResource
	CronDir          string
	StateDir         string
	RunDir           string
	BackendAvailable func() error
	LookupUser       LookupUser
	ResolveSecret    ResolveSecret
	mu               sync.Mutex
}

func New(resource models.EndpointScheduleResource) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	return &Applicator{
		Resource: resource, CronDir: "/etc/cron.d", StateDir: "/var/lib/remotr/schedules", RunDir: "/run/remotr/schedules",
		BackendAvailable: defaultBackendAvailable,
		LookupUser:       defaultLookupUser,
		ResolveSecret: func(_ context.Context, reference string) (string, error) {
			return secrets.ReadFileRef(reference)
		},
	}
}

func (a *Applicator) Name() string { return "endpoint-schedule/cron:" + a.Resource.Name }
func (a *Applicator) Description() string {
	return "cron endpoint schedule " + a.Resource.Name
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	result := a.Check(ctx)
	return result.ObservedSummary, result.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("cron endpoint schedule " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return checkFailed(desired, err)
	}
	if err := a.validatePaths(); err != nil {
		return checkFailed(desired, err)
	}
	if a.Resource.Lifecycle != models.LifecycleAbsent {
		if err := a.BackendAvailable(); err != nil {
			return executor.CheckResult{Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable, DesiredSummary: desired, ObservedSummary: "cron backend unavailable", Err: err}
		}
	}
	desiredFiles, err := a.desiredFiles(ctx)
	if err != nil {
		return checkFailed(desired, err)
	}
	for _, file := range desiredFiles {
		matches, err := file.matches()
		if err != nil {
			return checkFailed(desired, err)
		}
		if !matches {
			return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(file.label + " differs")}
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "owned cron schedule matches"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	check := a.Check(ctx)
	switch check.Status {
	case executor.Compliant:
		return appErr.ErrStateAlreadyMet
	case executor.Drifted:
	case executor.Unsupported, executor.CheckFailed, executor.Deferred:
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("endpoint schedule cannot apply: %s", check.ObservedSummary)
	default:
		return fmt.Errorf("endpoint schedule returned unknown check status %q", check.Status)
	}
	files, err := a.desiredFiles(ctx)
	if err != nil {
		return err
	}
	// Support files become durable before the cron fragment becomes active.
	// Removal runs in the opposite order so no new occurrence can begin while
	// protected schedule state is being removed.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := removeIfExists(a.fragmentPath()); err != nil {
			return err
		}
		for _, path := range []string{a.launcherPath(), a.environmentPath(), a.lockPath()} {
			if err := removeIfExists(path); err != nil {
				return err
			}
		}
		return nil
	}
	if err := ensureDirectory(a.StateDir, 0o711); err != nil {
		return fmt.Errorf("prepare cron schedule state directory: %w", err)
	}
	if a.Resource.Overlap == models.ScheduleOverlapForbid {
		if err := ensureDirectory(a.RunDir, 0o711); err != nil {
			return fmt.Errorf("prepare cron schedule run directory: %w", err)
		}
	}
	for _, file := range files {
		if file.path == a.fragmentPath() {
			continue
		}
		if err := file.converge(); err != nil {
			return err
		}
	}
	fragment := files[len(files)-1]
	if fragment.path != a.fragmentPath() {
		return errors.New("cron provider activation plan is invalid")
	}
	return fragment.converge()
}

func ensureDirectory(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
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

type desiredFile struct {
	path       string
	contents   []byte
	mode       os.FileMode
	uid, gid   int
	present    bool
	checkOwner bool
	label      string
}

func (f desiredFile) matches() (bool, error) {
	info, err := os.Lstat(f.path)
	if !f.present {
		if os.IsNotExist(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != f.mode.Perm() {
		return false, nil
	}
	if f.checkOwner {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != f.uid || int(stat.Gid) != f.gid {
			return false, nil
		}
	}
	contents, err := os.ReadFile(f.path) // #nosec G304 -- provider-owned validated path
	if err != nil {
		return false, err
	}
	return string(contents) == string(f.contents), nil
}

func (f desiredFile) converge() error {
	if !f.present {
		return removeIfExists(f.path)
	}
	return writeAtomic(f.path, f.contents, f.mode, f.uid, f.gid, f.checkOwner)
}

func (a *Applicator) desiredFiles(ctx context.Context) ([]desiredFile, error) {
	fragment := desiredFile{path: a.fragmentPath(), mode: 0o644, uid: -1, gid: -1, label: "owned cron fragment"}
	launcher := desiredFile{path: a.launcherPath(), mode: 0o700, uid: -1, gid: -1, label: "protected launcher"}
	environment := desiredFile{path: a.environmentPath(), mode: 0o600, uid: -1, gid: -1, label: "protected environment"}
	lock := desiredFile{path: a.lockPath(), mode: 0o600, uid: -1, gid: -1, label: "protected overlap lock"}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return []desiredFile{environment, launcher, lock, fragment}, nil
	}
	uid, gid, err := a.LookupUser(a.Resource.User)
	if err != nil {
		return nil, fmt.Errorf("endpoint schedule user %q: %w", a.Resource.User, err)
	}
	launcher.uid, launcher.gid, launcher.checkOwner = uid, gid, true
	environment.uid, environment.gid, environment.checkOwner = uid, gid, true
	lock.uid, lock.gid, lock.checkOwner = uid, gid, true
	launcher.present = true
	lock.present = a.Resource.Overlap == models.ScheduleOverlapForbid
	environment.contents, err = a.environment(ctx)
	if err != nil {
		return nil, err
	}
	environment.present = len(environment.contents) > 0
	launcher.contents = a.launcher(environment.present)
	fragment.present = a.Resource.Lifecycle == models.LifecyclePresent
	if fragment.present {
		fragment.contents = []byte("# Managed by Remotr: endpointSchedule/" + a.Resource.Name + "\n" + a.Resource.Schedule + " " + a.Resource.User + " " + cronPath(a.launcherPath()) + "\n")
	}
	return []desiredFile{environment, launcher, lock, fragment}, nil
}

func (a *Applicator) environment(ctx context.Context) ([]byte, error) {
	if len(a.Resource.Environment) == 0 {
		return nil, nil
	}
	var body strings.Builder
	for _, variable := range a.Resource.Environment {
		value := variable.Value
		if variable.SecretRef != "" {
			var err error
			value, err = a.ResolveSecret(ctx, variable.SecretRef)
			if err != nil {
				return nil, fmt.Errorf("resolve schedule environment reference %q: %w", variable.SecretRef, err)
			}
		}
		body.WriteString(variable.Name)
		body.WriteByte('=')
		body.WriteString(shellQuote(value))
		body.WriteByte('\n')
	}
	return []byte(body.String()), nil
}

func (a *Applicator) launcher(hasEnvironment bool) []byte {
	lines := []string{"#!/bin/sh", "set -eu"}
	if hasEnvironment {
		lines = append(lines, "set -a", ". "+shellQuote(a.environmentPath()), "set +a")
	}
	if a.Resource.WorkingDirectory != "" {
		lines = append(lines, "cd -- "+shellQuote(a.Resource.WorkingDirectory))
	}
	command := make([]string, 0, len(a.Resource.Argv)+10)
	if a.Resource.Overlap == models.ScheduleOverlapForbid {
		command = append(command, "/usr/bin/flock", "--nonblock", filepath.Join(a.RunDir, a.Resource.Name+".lock"))
	}
	if a.Resource.Timeout != "" {
		command = append(command, "/usr/bin/timeout", "--signal=TERM", a.Resource.Timeout)
	}
	if len(a.Resource.Argv) > 0 {
		command = append(command, a.Resource.Argv...)
	} else {
		command = append(command, "/bin/sh", "-c", "--", a.Resource.Shell)
	}
	quoted := make([]string, len(command))
	for i, arg := range command {
		quoted[i] = shellQuote(arg)
	}
	lines = append(lines, "exec "+strings.Join(quoted, " "))
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (a *Applicator) validatePaths() error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	for name, path := range map[string]string{"cron directory": a.CronDir, "state directory": a.StateDir, "run directory": a.RunDir} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("%s %q must be a clean absolute path", name, path)
		}
	}
	if a.BackendAvailable == nil || a.LookupUser == nil || a.ResolveSecret == nil {
		return errors.New("cron provider boundaries are required")
	}
	return nil
}

func (a *Applicator) fragmentPath() string {
	return filepath.Join(a.CronDir, "remotr-"+a.Resource.Name)
}
func (a *Applicator) launcherPath() string { return filepath.Join(a.StateDir, a.Resource.Name+".sh") }
func (a *Applicator) environmentPath() string {
	return filepath.Join(a.StateDir, a.Resource.Name+".env")
}
func (a *Applicator) lockPath() string { return filepath.Join(a.RunDir, a.Resource.Name+".lock") }

func defaultBackendAvailable() error {
	if _, err := exec.LookPath("cron"); err == nil {
		return nil
	}
	if _, err := exec.LookPath("crond"); err == nil {
		return nil
	}
	return errors.New("neither cron nor crond is installed")
}

func defaultLookupUser(name string) (int, int, error) {
	entry, err := user.Lookup(name)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(entry.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(entry.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func cronPath(path string) string {
	if strings.IndexFunc(path, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) == -1 {
		return path
	}
	return shellQuote(path)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func writeAtomic(path string, contents []byte, mode os.FileMode, uid, gid int, owner bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-schedule-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if owner {
		if err := temporary.Chown(uid, gid); err != nil {
			temporary.Close()
			return err
		}
	}
	if _, err := temporary.Write(contents); err != nil {
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

func checkFailed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, ObservedSummary: "cron schedule check failed", Err: err}
}
