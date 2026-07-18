// Package journald manages validated, named systemd-journald drop-ins.
package journald

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

type Applicator struct {
	Resource          models.JournaldResource
	ConfigDir         string
	MainConfig        string
	Runner            executil.Runner
	ValidateEffective func(context.Context, string, string, string) error
	previous          []byte
	previousExists    bool
	rollbackArmed     bool
	rollback          *filetx.Handle
}

func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := filetx.New(store, address, artifactDigest, true)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
}

func New(resource models.JournaldResource, runners ...executil.Runner) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	runner := executil.Runner(executil.SanitizedOSRunner{})
	if len(runners) > 0 && runners[0] != nil {
		runner = runners[0]
	}
	return &Applicator{
		Resource: resource, ConfigDir: "/etc/systemd/journald.conf.d", MainConfig: "/etc/systemd/journald.conf", Runner: runner,
	}
}

func (a *Applicator) Name() string { return "journald:" + a.Resource.Name }

func (a *Applicator) Description() string { return "journald policy " + a.Resource.Name }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.ConfigDir) || !filepath.IsAbs(a.MainConfig) {
		return "", fmt.Errorf("journald configuration paths must be absolute")
	}
	return filepath.Join(a.ConfigDir, "90-remotr-"+a.Resource.Name+".conf"), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(_ context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("named journald policy " + a.Resource.Name)
	path, err := a.path()
	if err != nil {
		return failedCheck(desired, err)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated named drop-in path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named journald drop-in is absent"}
		}
		if err != nil {
			return failedCheck(desired, err)
		}
		return driftedCheck(desired, "named journald drop-in exists")
	}
	if os.IsNotExist(err) {
		return driftedCheck(desired, "named journald drop-in is absent")
	}
	if err != nil {
		return failedCheck(desired, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return failedCheck(desired, err)
	}
	if string(content) == a.render() && info.Mode().Perm() == 0o644 {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named journald drop-in matches"}
	}
	return driftedCheck(desired, "named journald drop-in differs")
}

func (a *Applicator) Apply(ctx context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated named drop-in path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := a.validateCandidate(ctx); err != nil {
		return err
	}
	if a.rollback != nil {
		if err := a.rollback.Arm(ctx, path); err != nil {
			return err
		}
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if err := atomicWrite(path, []byte(a.render())); err != nil {
		return err
	}
	if a.rollback == nil {
		a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), previous...), previousExists, true
	}
	return nil
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
		return executor.ApplyResult{
			Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass,
			Activation: []executor.ActivationSignal{{Kind: executor.ActivationRestart, Target: "systemd-journald.service"}},
		}
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
	a.rollbackArmed = false
	return nil
}

func (a *Applicator) validateCandidate(ctx context.Context) error {
	stageRoot, err := os.MkdirTemp("", "remotr-journald-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	stagedSystemdDir := filepath.Join(stageRoot, "etc", "systemd")
	stagedMain := filepath.Join(stagedSystemdDir, "journald.conf")
	stagedDir := filepath.Join(stagedSystemdDir, "journald.conf.d")
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		return err
	}
	main, err := os.ReadFile(a.MainConfig) // #nosec G304 -- configured fixed journald main path.
	if os.IsNotExist(err) {
		main = []byte("[Journal]\n")
	} else if err != nil {
		return err
	}
	if err := os.WriteFile(stagedMain, main, 0o644); err != nil {
		return err
	}
	if err := copyDropIns(a.ConfigDir, stagedDir); err != nil {
		return err
	}
	stagedPath := filepath.Join(stagedDir, "90-remotr-"+a.Resource.Name+".conf")
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := os.WriteFile(stagedPath, []byte(a.render()), 0o644); err != nil {
		return err
	}
	if a.ValidateEffective != nil {
		return a.ValidateEffective(ctx, stagedMain, stagedDir, stagedPath)
	}
	_, stderr, err := a.Runner.Run("systemd-analyze", "--root="+stageRoot, "cat-config", "systemd/journald.conf")
	if err != nil {
		return fmt.Errorf("systemd-analyze rejected staged journald policy: %s", bounded(stderr))
	}
	return nil
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	lines := []string{"[Journal]"}
	if a.Resource.Storage != "" {
		lines = append(lines, "Storage="+string(a.Resource.Storage))
	}
	if a.Resource.MaxRetention != "" {
		lines = append(lines, "MaxRetentionSec="+a.Resource.MaxRetention)
	}
	if a.Resource.SystemMaxUseBytes != nil {
		lines = append(lines, "SystemMaxUse="+strconv.FormatInt(*a.Resource.SystemMaxUseBytes, 10))
	}
	if a.Resource.RuntimeMaxUseBytes != nil {
		lines = append(lines, "RuntimeMaxUse="+strconv.FormatInt(*a.Resource.RuntimeMaxUseBytes, 10))
	}
	if a.Resource.RateLimitInterval != "" {
		lines = append(lines, "RateLimitIntervalSec="+a.Resource.RateLimitInterval)
	}
	if a.Resource.RateLimitBurst != nil {
		lines = append(lines, "RateLimitBurst="+strconv.Itoa(*a.Resource.RateLimitBurst))
	}
	for _, forwarding := range []struct {
		name  string
		value *bool
	}{
		{"ForwardToSyslog", a.Resource.ForwardToSyslog},
		{"ForwardToKMsg", a.Resource.ForwardToKernelBuffer},
		{"ForwardToConsole", a.Resource.ForwardToConsole},
		{"ForwardToWall", a.Resource.ForwardToWall},
	} {
		if forwarding.value != nil {
			value := "no"
			if *forwarding.value {
				value = "yes"
			}
			lines = append(lines, forwarding.name+"="+value)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func copyDropIns(source, destination string) error {
	entries, err := os.ReadDir(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("journald drop-in %q is not regular", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(source, entry.Name())) // #nosec G304 -- enumerated drop-in.
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), content, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-journald-")
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

func bounded(stderr []byte) string {
	const limit = 512
	value := strings.TrimSpace(string(stderr))
	if len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

func failedCheck(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

func driftedCheck(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}
