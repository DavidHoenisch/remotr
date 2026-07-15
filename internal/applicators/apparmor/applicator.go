// Package apparmor manages named AppArmor policy profiles.
package apparmor

import (
	"bufio"
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

type ObserveModeFunc func(context.Context, string) (models.AppArmorMode, error)
type ValidateStageFunc func(context.Context, string) error
type ActivateProfileFunc func(context.Context, string, models.AppArmorMode, models.AppArmorMode) error

type Applicator struct {
	Resource          models.AppArmorProfileResource
	ProfilesDir       string
	DisableDir        string
	ProfilesStatePath string
	Runner            executil.Runner
	ObserveMode       ObserveModeFunc
	ValidateStage     ValidateStageFunc
	ActivateProfile   ActivateProfileFunc
	previous          []byte
	previousExists    bool
	previousDisabled  bool
	previousMode      models.AppArmorMode
	armed             bool
}

func New(resource models.AppArmorProfileResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	applicator := &Applicator{
		Resource: resource, ProfilesDir: "/etc/apparmor.d", DisableDir: "/etc/apparmor.d/disable",
		ProfilesStatePath: "/sys/kernel/security/apparmor/profiles", Runner: runner,
	}
	applicator.ObserveMode = applicator.observeMode
	applicator.ValidateStage = applicator.validateStage
	applicator.ActivateProfile = applicator.activateProfile
	return applicator
}

func (a *Applicator) Name() string { return "apparmor:" + a.Resource.Name }

func (a *Applicator) Description() string { return "AppArmor profile " + a.Resource.Profile }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.ProfilesDir) || !filepath.IsAbs(a.DisableDir) {
		return "", fmt.Errorf("AppArmor provider directories must be absolute")
	}
	return filepath.Join(a.ProfilesDir, "remotr-"+a.Resource.Name), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("AppArmor profile " + a.Resource.Profile + " mode=" + string(a.Resource.Mode))
	path, err := a.path()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	if os.IsNotExist(err) {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named profile is absent"}
	}
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	mode, err := a.ObserveMode(ctx, a.Resource.Profile)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	observed := executor.RedactedSummary("profile=" + a.Resource.Profile + " mode=" + string(mode))
	modeMet := mode == a.Resource.Mode
	if a.Resource.Mode == models.AppArmorDisabled {
		_, symlinkErr := os.Lstat(filepath.Join(a.DisableDir, filepath.Base(path)))
		modeMet = modeMet && symlinkErr == nil
	}
	if string(content) == a.Resource.Content && modeMet {
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
	if err := os.MkdirAll(a.ProfilesDir, 0o755); err != nil {
		return err
	}
	staged, err := stage(path, []byte(a.Resource.Content))
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	if err := a.ValidateStage(ctx, staged); err != nil {
		return fmt.Errorf("validate staged AppArmor profile %q: %w", a.Resource.Name, err)
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	previousMode, err := a.ObserveMode(ctx, a.Resource.Profile)
	if err != nil {
		return err
	}
	disablePath := filepath.Join(a.DisableDir, filepath.Base(path))
	_, disableErr := os.Lstat(disablePath)
	previousDisabled := disableErr == nil
	if disableErr != nil && !os.IsNotExist(disableErr) {
		return disableErr
	}
	if err := os.Rename(staged, path); err != nil {
		return err
	}
	if err := a.ActivateProfile(ctx, path, a.Resource.Mode, previousMode); err != nil {
		_ = restoreProfile(path, previous, previousExists)
		if previousExists {
			_ = a.ActivateProfile(ctx, path, previousMode, a.Resource.Mode)
		}
		return fmt.Errorf("activate AppArmor profile %q: %w", a.Resource.Name, err)
	}
	a.previous, a.previousExists, a.previousMode, a.previousDisabled, a.armed = previous, previousExists, previousMode, previousDisabled, true
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
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
	if err := restoreProfile(path, a.previous, a.previousExists); err != nil {
		return err
	}
	if a.previousExists {
		if err := a.ActivateProfile(ctx, path, a.previousMode, a.Resource.Mode); err != nil {
			return err
		}
	}
	a.previous = nil
	a.armed = false
	return nil
}

func (a *Applicator) validateStage(_ context.Context, staged string) error {
	_, stderr, err := a.Runner.Run("apparmor_parser", "-Q", "-T", staged)
	if err != nil {
		if len(stderr) > 0 {
			return fmt.Errorf("apparmor_parser diagnostic was redacted: %w", err)
		}
		return err
	}
	return nil
}

func (a *Applicator) activateProfile(_ context.Context, path string, desired, current models.AppArmorMode) error {
	disablePath := filepath.Join(a.DisableDir, filepath.Base(path))
	if desired == models.AppArmorDisabled {
		if current != models.AppArmorDisabled {
			if _, stderr, err := a.Runner.Run("apparmor_parser", "-R", path); err != nil {
				return redactedParserError(stderr, err)
			}
		}
		if err := os.MkdirAll(a.DisableDir, 0o755); err != nil {
			return err
		}
		if err := os.Remove(disablePath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink("../"+filepath.Base(path), disablePath)
	}
	if err := os.Remove(disablePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	args := []string{"-r", "-W"}
	if desired == models.AppArmorComplain {
		args = append(args, "-C")
	}
	args = append(args, path)
	_, stderr, err := a.Runner.Run("apparmor_parser", args...)
	return redactedParserError(stderr, err)
}

func redactedParserError(stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	if len(stderr) > 0 {
		return fmt.Errorf("apparmor_parser diagnostic was redacted: %w", err)
	}
	return err
}

func (a *Applicator) observeMode(_ context.Context, profile string) (models.AppArmorMode, error) {
	file, err := os.Open(a.ProfilesStatePath)
	if os.IsNotExist(err) {
		return models.AppArmorDisabled, nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	prefix := profile + " ("
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, ")") {
			continue
		}
		mode := models.AppArmorMode(strings.TrimSuffix(strings.TrimPrefix(line, prefix), ")"))
		if mode == models.AppArmorEnforce || mode == models.AppArmorComplain {
			return mode, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return models.AppArmorDisabled, nil
}

func stage(path string, content []byte) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-apparmor-")
	if err != nil {
		return "", err
	}
	name := file.Name()
	defer func() {
		if file != nil {
			_ = file.Close()
		}
	}()
	if err := file.Chmod(0o644); err != nil {
		os.Remove(name)
		return "", err
	}
	if _, err := file.Write(content); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := file.Sync(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	file = nil
	return name, nil
}

func restoreProfile(path string, previous []byte, exists bool) error {
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
