// Package timesync implements the advertised systemd-timesyncd provider for
// the provider-neutral time-synchronization resource contract.
package timesync

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
	Resource              models.TimeSyncResource
	Runner                executil.Runner
	ConfigDir             string
	SupportsCustomServers func() bool
}

func New(resource models.TimeSyncResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{
		Resource: resource, Runner: runner, ConfigDir: "/etc/systemd/timesyncd.conf.d",
		SupportsCustomServers: func() bool { return true },
	}
}

func (a *Applicator) Name() string        { return "time-sync:" + a.Resource.Name }
func (a *Applicator) Description() string { return "time synchronization " + a.Resource.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("time synchronization " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return failed(desired, err)
	}
	if err := a.Resource.Validate(); err != nil {
		return failed(desired, err)
	}
	available, err := a.backendAvailable()
	if err != nil {
		return failed(desired, err)
	}
	if !available {
		return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "time_sync_provider_unsupported", DesiredSummary: desired, ObservedSummary: "systemd-timesyncd.service is unavailable"}
	}
	if a.managesServers() && !a.SupportsCustomServers() {
		return executor.CheckResult{Status: executor.Unsupported, ReasonCode: "time_sync_servers_unsupported", DesiredSummary: desired, ObservedSummary: "active provider cannot manage custom time servers"}
	}
	if a.Resource.Enabled != nil {
		state, err := a.enablement()
		if err != nil {
			return failed(desired, err)
		}
		if state.configured != *a.Resource.Enabled {
			return drifted(desired, "time synchronization configured enablement differs")
		}
		if state.effective() != *a.Resource.Enabled {
			return drifted(desired, "time synchronization effective enablement differs")
		}
	}
	if a.managesServers() {
		path, err := a.fragmentPath()
		if err != nil {
			return failed(desired, err)
		}
		contents, err := os.ReadFile(path) // #nosec G304 -- path is derived from validated resource identity.
		if os.IsNotExist(err) || string(contents) != a.fragmentContents() {
			return drifted(desired, "time-server fragment differs")
		}
		if err != nil {
			return failed(desired, err)
		}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func (a *Applicator) Apply(ctx context.Context) error {
	_, err := a.apply(ctx)
	return err
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	changed, err := a.apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	if changed.fragment {
		result.Activation = []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "systemd-timesyncd.service"}}
	}
	return result
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

type changes struct{ enabled, fragment bool }

func (a *Applicator) apply(ctx context.Context) (changes, error) {
	if err := ctx.Err(); err != nil {
		return changes{}, err
	}
	if err := a.Resource.Validate(); err != nil {
		return changes{}, err
	}
	available, err := a.backendAvailable()
	if err != nil {
		return changes{}, err
	}
	if !available {
		return changes{}, errors.New("systemd-timesyncd.service is unavailable")
	}
	if a.managesServers() && !a.SupportsCustomServers() {
		return changes{}, fmt.Errorf("time synchronization server configuration is unsupported by %s", a.Resource.Provider)
	}
	changed := changes{}
	var rollbackFragment func() error
	var observedEnablement *enablementState
	if a.Resource.Enabled != nil {
		state, err := a.enablement()
		if err != nil {
			return changes{}, err
		}
		observedEnablement = &state
	}
	if a.managesServers() {
		path, err := a.fragmentPath()
		if err != nil {
			return changes{}, err
		}
		contents, err := os.ReadFile(path) // #nosec G304 -- path is derived from validated resource identity.
		existed := err == nil
		if err != nil && !os.IsNotExist(err) {
			return changes{}, err
		}
		mode := os.FileMode(0o644)
		if existed {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return changes{}, statErr
			}
			mode = info.Mode().Perm()
		}
		if !existed || string(contents) != a.fragmentContents() {
			if err := writeAtomic(path, []byte(a.fragmentContents()), 0o644); err != nil {
				return changes{}, err
			}
			previous := append([]byte(nil), contents...)
			rollbackFragment = func() error {
				if !existed {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						return fmt.Errorf("remove staged time-server fragment: %w", err)
					}
					return nil
				}
				if err := writeAtomic(path, previous, mode); err != nil {
					return fmt.Errorf("restore time-server fragment: %w", err)
				}
				return nil
			}
			changed.fragment = true
		}
	}
	if observedEnablement != nil && (observedEnablement.configured != *a.Resource.Enabled || observedEnablement.effective() != *a.Resource.Enabled) {
		if err := a.setEnabled(*a.Resource.Enabled); err != nil {
			if rollbackFragment != nil {
				return changes{}, errors.Join(err, rollbackFragment())
			}
			return changes{}, err
		}
		changed.enabled = true
	}
	if !changed.enabled && !changed.fragment {
		return changes{}, appErr.ErrStateAlreadyMet
	}
	return changed, nil
}

func (a *Applicator) managesServers() bool {
	return a.Resource.Servers != nil || a.Resource.Pools != nil
}

func (a *Applicator) backendAvailable() (bool, error) {
	stdout, stderr, err := a.Runner.Run("systemctl", "show", "systemd-timesyncd.service", "--property=LoadState", "--value")
	if err != nil {
		return false, fmt.Errorf("inspect systemd-timesyncd provider: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	switch strings.TrimSpace(string(stdout)) {
	case "loaded":
		return true, nil
	case "not-found", "masked":
		return false, nil
	default:
		return false, fmt.Errorf("systemd-timesyncd provider returned unsupported load state %q", strings.TrimSpace(string(stdout)))
	}
}

type enablementState struct {
	configured bool
	active     bool
	ntp        bool
}

func (s enablementState) effective() bool { return s.active && s.ntp }

func (a *Applicator) enablement() (enablementState, error) {
	unitFileState, err := a.systemdState("UnitFileState")
	if err != nil {
		return enablementState{}, err
	}
	configured := false
	switch unitFileState {
	case "enabled", "enabled-runtime":
		configured = true
	case "disabled", "static", "indirect", "generated", "transient":
	default:
		return enablementState{}, fmt.Errorf("systemd-timesyncd configured state is unsupported: %q", unitFileState)
	}
	activeState, err := a.systemdState("ActiveState")
	if err != nil {
		return enablementState{}, err
	}
	active := false
	switch activeState {
	case "active":
		active = true
	case "inactive", "failed", "activating", "deactivating", "reloading":
	default:
		return enablementState{}, fmt.Errorf("systemd-timesyncd effective state is unsupported: %q", activeState)
	}
	stdout, stderr, err := a.Runner.Run("timedatectl", "show", "--property=NTP", "--value")
	if err != nil {
		return enablementState{}, fmt.Errorf("read time synchronization enablement: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	ntp := false
	switch strings.TrimSpace(string(stdout)) {
	case "yes":
		ntp = true
	case "no":
	default:
		return enablementState{}, fmt.Errorf("time synchronization enablement returned unsupported value %q", strings.TrimSpace(string(stdout)))
	}
	return enablementState{configured: configured, active: active, ntp: ntp}, nil
}

func (a *Applicator) systemdState(property string) (string, error) {
	stdout, stderr, err := a.Runner.Run("systemctl", "show", "systemd-timesyncd.service", "--property="+property, "--value")
	if err != nil {
		return "", fmt.Errorf("read systemd-timesyncd %s: %s: %w", property, strings.TrimSpace(string(stderr)), err)
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (a *Applicator) setEnabled(enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	if _, stderr, err := a.Runner.Run("timedatectl", "set-ntp", value); err != nil {
		return fmt.Errorf("set time synchronization enablement: %s: %w", strings.TrimSpace(string(stderr)), err)
	}
	return nil
}

func (a *Applicator) fragmentPath() (string, error) {
	if !filepath.IsAbs(a.ConfigDir) {
		return "", fmt.Errorf("time sync config directory must be absolute")
	}
	return filepath.Join(a.ConfigDir, "99-remotr-"+a.Resource.Name+".conf"), nil
}

func (a *Applicator) fragmentContents() string {
	lines := []string{"[Time]"}
	if a.Resource.Servers != nil {
		lines = append(lines, "NTP="+strings.Join(a.Resource.Servers, " "))
	}
	if a.Resource.Pools != nil {
		lines = append(lines, "FallbackNTP="+strings.Join(a.Resource.Pools, " "))
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeAtomic(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-timesync-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func drifted(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}

func failed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}
