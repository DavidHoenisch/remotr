// Package systemdtimer manages persistent endpoint schedules as paired
// systemd service and timer units.
package systemdtimer

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
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type LookupUser func(string) (uid, gid int, err error)
type ResolveSecret func(context.Context, string) (string, error)
type ValidateUnits func(context.Context, string, string) error

type Applicator struct {
	Resource       models.EndpointScheduleResource
	Runner         executil.Runner
	UnitDir        string
	EnvironmentDir string
	LookupUser     LookupUser
	ResolveSecret  ResolveSecret
	ValidateUnits  ValidateUnits
	mu             sync.Mutex
}

func New(resource models.EndpointScheduleResource, runner executil.Runner) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{
		Resource: resource, Runner: runner, UnitDir: "/etc/systemd/system", EnvironmentDir: "/var/lib/remotr/schedules",
		LookupUser: defaultLookupUser,
		ResolveSecret: func(_ context.Context, reference string) (string, error) {
			return secrets.ReadFileRef(reference)
		},
	}
}

func (a *Applicator) Name() string { return "endpoint-schedule/systemd-timer:" + a.Resource.Name }
func (a *Applicator) Description() string {
	return "systemd timer endpoint schedule " + a.Resource.Name
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	result := a.Check(ctx)
	return result.ObservedSummary, result.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("systemd timer endpoint schedule " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return failedCheck(desired, err)
	}
	if err := a.validate(); err != nil {
		return failedCheck(desired, err)
	}
	state, err := a.timerState()
	if err != nil {
		return failedCheck(desired, err)
	}
	files, err := a.desiredFiles(ctx)
	if err != nil {
		return failedCheck(desired, err)
	}
	drift := ""
	for _, file := range files {
		matches, err := file.matches()
		if err != nil {
			return failedCheck(desired, err)
		}
		if !matches && drift == "" {
			drift = file.label + " differs"
		}
	}
	wantEnabled, wantActive := false, false
	if a.Resource.Lifecycle == models.LifecyclePresent {
		wantEnabled, wantActive = true, true
	}
	if state.enabled != wantEnabled && drift == "" {
		drift = "timer enablement differs"
	}
	if state.active != wantActive && drift == "" {
		drift = "timer active state differs"
	}
	if drift != "" {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(drift), Actual: state}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "paired systemd timer units match", Actual: state}
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
		return fmt.Errorf("systemd timer cannot apply: %s", check.ObservedSummary)
	}
	state, ok := check.Actual.(timerState)
	if !ok {
		return errors.New("systemd timer check omitted observed timer state")
	}
	files, err := a.desiredFiles(ctx)
	if err != nil {
		return err
	}
	if a.Resource.Lifecycle != models.LifecycleAbsent {
		if err := a.validateStagedUnits(ctx, files); err != nil {
			return err
		}
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if state.active {
			if err := a.systemctl("stop", a.timerUnit()); err != nil {
				return err
			}
		}
		if state.enabled {
			if err := a.systemctl("disable", a.timerUnit()); err != nil {
				return err
			}
		}
	}
	filesChanged := false
	for _, file := range files {
		matches, err := file.matches()
		if err != nil {
			return err
		}
		if matches {
			continue
		}
		if err := file.converge(); err != nil {
			return err
		}
		filesChanged = true
	}
	if filesChanged {
		if err := a.systemctl("daemon-reload"); err != nil {
			return err
		}
	}
	switch a.Resource.Lifecycle {
	case models.LifecyclePresent:
		if !state.enabled {
			if err := a.systemctl("enable", a.timerUnit()); err != nil {
				return err
			}
		}
		if !state.active {
			if err := a.systemctl("start", a.timerUnit()); err != nil {
				return err
			}
		}
	case models.LifecycleDisabled:
		if state.active {
			if err := a.systemctl("stop", a.timerUnit()); err != nil {
				return err
			}
		}
		if state.enabled {
			if err := a.systemctl("disable", a.timerUnit()); err != nil {
				return err
			}
		}
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

type timerState struct{ enabled, active bool }

func (a *Applicator) timerState() (timerState, error) {
	enabled, err := a.systemctlState("is-enabled", "enabled", "enabled-runtime")
	if err != nil {
		return timerState{}, err
	}
	active, err := a.systemctlState("is-active", "active")
	if err != nil {
		return timerState{}, err
	}
	return timerState{enabled: enabled, active: active}, nil
}

func (a *Applicator) systemctlState(operation string, trueValues ...string) (bool, error) {
	stdout, stderr, err := a.Runner.Run("systemctl", operation, a.timerUnit())
	value := strings.TrimSpace(string(stdout))
	for _, expected := range trueValues {
		if value == expected {
			return true, nil
		}
	}
	switch value {
	case "disabled", "indirect", "static", "inactive", "failed", "unknown", "not-found":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl %s %s: %s: %w", operation, a.timerUnit(), bounded(stderr), err)
	}
	return false, nil
}

func (a *Applicator) systemctl(args ...string) error {
	_, stderr, err := a.Runner.Run("systemctl", args...)
	if err != nil {
		return fmt.Errorf("systemctl %s: %s: %w", strings.Join(args, " "), bounded(stderr), err)
	}
	return nil
}

type desiredFile struct {
	path       string
	contents   []byte
	mode       os.FileMode
	uid, gid   int
	checkOwner bool
	present    bool
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
		err := os.Remove(f.path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return writeAtomic(f.path, f.contents, f.mode, f.uid, f.gid, f.checkOwner)
}

func (a *Applicator) desiredFiles(ctx context.Context) ([]desiredFile, error) {
	environment := desiredFile{path: a.environmentPath(), mode: 0o600, uid: -1, gid: -1, label: "protected environment"}
	service := desiredFile{path: a.servicePath(), mode: 0o644, uid: -1, gid: -1, label: "paired service unit"}
	timer := desiredFile{path: a.timerPath(), mode: 0o644, uid: -1, gid: -1, label: "timer unit"}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return []desiredFile{environment, service, timer}, nil
	}
	uid, gid, err := a.LookupUser(a.Resource.User)
	if err != nil {
		return nil, fmt.Errorf("endpoint schedule user %q: %w", a.Resource.User, err)
	}
	environment.uid, environment.gid, environment.checkOwner = uid, gid, true
	environment.contents, err = a.environment(ctx)
	if err != nil {
		return nil, err
	}
	environment.present = len(environment.contents) > 0
	service.present, timer.present = true, true
	service.contents = a.serviceUnit(environment.present)
	timer.contents = a.timerUnitContents()
	return []desiredFile{environment, service, timer}, nil
}

func (a *Applicator) serviceUnit(hasEnvironment bool) []byte {
	lines := []string{"[Unit]", "Description=Remotr endpoint schedule " + a.Resource.Name, "", "[Service]", "Type=oneshot", "User=" + a.Resource.User}
	if a.Resource.WorkingDirectory != "" {
		lines = append(lines, "WorkingDirectory="+execArgument(a.Resource.WorkingDirectory))
	}
	if hasEnvironment {
		lines = append(lines, "EnvironmentFile="+execArgument(a.environmentPath()))
	}
	if a.Resource.Timeout != "" {
		lines = append(lines, "TimeoutStartSec="+a.Resource.Timeout)
	}
	command := a.Resource.Argv
	if len(command) == 0 {
		command = []string{"/bin/sh", "-c", a.Resource.Shell}
	}
	quoted := make([]string, len(command))
	for i, arg := range command {
		quoted[i] = execArgument(arg)
	}
	lines = append(lines, "ExecStart="+strings.Join(quoted, " "))
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (a *Applicator) timerUnitContents() []byte {
	persistent := false
	if a.Resource.Persistent != nil {
		persistent = *a.Resource.Persistent
	}
	return []byte(strings.Join([]string{
		"[Unit]",
		"Description=Remotr endpoint schedule " + a.Resource.Name,
		"",
		"[Timer]",
		"OnCalendar=" + a.Resource.Schedule,
		fmt.Sprintf("Persistent=%t", persistent),
		"Unit=" + a.serviceUnitName(),
		"",
		"[Install]",
		"WantedBy=timers.target",
	}, "\n") + "\n")
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
		body.WriteString(unitValue(value))
		body.WriteByte('\n')
	}
	return []byte(body.String()), nil
}

func (a *Applicator) validateStagedUnits(ctx context.Context, files []desiredFile) error {
	if err := os.MkdirAll(a.UnitDir, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(a.UnitDir, ".remotr-systemd-timer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var service, timer string
	for _, file := range files {
		switch file.path {
		case a.servicePath():
			service = filepath.Join(stage, filepath.Base(file.path))
			if err := os.WriteFile(service, file.contents, 0o644); err != nil {
				return err
			}
		case a.timerPath():
			timer = filepath.Join(stage, filepath.Base(file.path))
			if err := os.WriteFile(timer, file.contents, 0o644); err != nil {
				return err
			}
		}
	}
	if a.ValidateUnits != nil {
		return a.ValidateUnits(ctx, service, timer)
	}
	_, stderr, err := a.Runner.Run("systemd-analyze", "verify", service, timer)
	if err != nil {
		return fmt.Errorf("systemd-analyze rejected staged schedule units: %s: %w", bounded(stderr), err)
	}
	return nil
}

func (a *Applicator) validate() error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if a.Resource.Backend != models.ScheduleBackendSystemdTimer {
		return fmt.Errorf("systemd timer provider cannot manage backend %q", a.Resource.Backend)
	}
	for label, path := range map[string]string{"unit directory": a.UnitDir, "environment directory": a.EnvironmentDir} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("%s %q must be a clean absolute path", label, path)
		}
	}
	if a.Runner == nil || a.LookupUser == nil || a.ResolveSecret == nil {
		return errors.New("systemd timer provider boundaries are required")
	}
	return nil
}

func (a *Applicator) serviceUnitName() string {
	return "remotr-schedule-" + a.Resource.Name + ".service"
}
func (a *Applicator) timerUnit() string   { return "remotr-schedule-" + a.Resource.Name + ".timer" }
func (a *Applicator) servicePath() string { return filepath.Join(a.UnitDir, a.serviceUnitName()) }
func (a *Applicator) timerPath() string   { return filepath.Join(a.UnitDir, a.timerUnit()) }
func (a *Applicator) environmentPath() string {
	return filepath.Join(a.EnvironmentDir, a.Resource.Name+".env")
}

func execArgument(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) == -1 && !strings.Contains(value, "%") {
		return value
	}
	return unitValue(value)
}

func unitValue(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return "\"" + value + "\""
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

func writeAtomic(path string, contents []byte, mode os.FileMode, uid, gid int, owner bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-systemd-")
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

func bounded(value []byte) string {
	const limit = 512
	value = []byte(strings.TrimSpace(string(value)))
	if len(value) > limit {
		value = value[:limit]
	}
	return string(value)
}

func failedCheck(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, ObservedSummary: "systemd timer check failed", Err: err}
}
