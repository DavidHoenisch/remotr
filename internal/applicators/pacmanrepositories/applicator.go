// Package pacmanrepositories manages narrowly owned Pacman repository
// fragments without editing distribution-owned configuration.
package pacmanrepositories

import (
	"bytes"
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

var errFragmentMismatch = errors.New("Pacman repository fragment differs")

const (
	includeBoundaryBegin = "# BEGIN Remotr managed Pacman repositories"
	includeBoundaryEnd   = "# END Remotr managed Pacman repositories"
)

// Applicator owns exactly one repository fragment below FragmentsDir.
type Applicator struct {
	Repository   models.PacmanRepository
	FragmentsDir string
	ConfigPath   string
	Runner       executil.Runner
}

// New creates a Pacman repository applicator.
func New(repository models.PacmanRepository, runner executil.Runner) *Applicator {
	if repository.Lifecycle == "" {
		repository.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{
		Repository: repository, FragmentsDir: "/etc/pacman.d/remotr-repositories",
		ConfigPath: "/etc/pacman.conf", Runner: runner,
	}
}

func (a *Applicator) Name() string { return "pacman-repository:" + a.Repository.Name }

func (a *Applicator) Description() string { return "Pacman repository " + a.Repository.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("owned Pacman repository " + a.Repository.Name)
	path, err := a.fragmentPath()
	if err != nil {
		return checkFailure(desired, err)
	}
	if err := ctx.Err(); err != nil {
		return checkFailure(desired, err)
	}
	if a.Repository.CredentialRef != "" {
		return checkFailure(desired, unsupportedCredentialError())
	}
	if a.Repository.Lifecycle == models.LifecycleAbsent {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return a.checkBoundary(ctx, desired)
		} else if err != nil {
			return checkFailure(desired, err)
		}
		return drift(desired, "owned repository fragment exists")
	}
	if err := contentMatches(path, a.fragment(), 0o644); err != nil {
		if os.IsNotExist(err) || errors.Is(err, errFragmentMismatch) {
			return drift(desired, "owned repository fragment differs")
		}
		return checkFailure(desired, err)
	}
	boundaryCheck := a.checkBoundary(ctx, desired)
	if boundaryCheck.Status != executor.Compliant {
		return boundaryCheck
	}
	if a.Repository.Lifecycle == models.LifecyclePresent {
		if err := a.validateNative(ctx, a.ConfigPath, true); err != nil {
			return checkFailure(desired, err)
		}
	}
	return compliant(desired)
}

func (a *Applicator) checkBoundary(ctx context.Context, desired executor.RedactedSummary) executor.CheckResult {
	if err := ctx.Err(); err != nil {
		return checkFailure(desired, err)
	}
	content, _, err := a.readConfig()
	if err != nil {
		return checkFailure(desired, err)
	}
	state := a.inspectBoundary(content)
	if state == boundaryMalformed {
		return drift(desired, "managed include boundary is malformed")
	}
	required, err := a.managedRepositoryRequiresInclude()
	if err != nil {
		return checkFailure(desired, err)
	}
	if required && state != boundaryPresent {
		return drift(desired, "managed include boundary is absent")
	}
	if !required && state != boundaryAbsent {
		return drift(desired, "managed include boundary is unnecessary")
	}
	return compliant(desired)
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := a.Repository.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.Repository.CredentialRef != "" {
		return unsupportedCredentialError()
	}
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status == executor.CheckFailed {
		return check.Err
	}
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	if _, _, err := a.preflightBoundary(); err != nil {
		return err
	}
	previous, err := captureOwnedFragment(path)
	if err != nil {
		return err
	}
	if a.Repository.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove owned Pacman repository fragment: %w", err)
		}
		if err := a.reconcileBoundary(ctx); err != nil {
			return restoreAfterPartialFailure(path, previous, err)
		}
		return nil
	}
	if err := a.stageValidateAndActivate(ctx, path, []byte(a.fragment()), 0o644, ".remotr-pacman-repository-", a.Repository.Lifecycle == models.LifecyclePresent); err != nil {
		return err
	}
	if err := a.reconcileBoundary(ctx); err != nil {
		return restoreAfterPartialFailure(path, previous, err)
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

func (*Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) fragmentPath() (string, error) {
	if !filepath.IsAbs(a.FragmentsDir) || filepath.Clean(a.FragmentsDir) != a.FragmentsDir {
		return "", errors.New("Pacman repository fragments directory must be a clean absolute path")
	}
	if err := a.Repository.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(a.FragmentsDir, a.Repository.Name+".conf"), nil
}

func (a *Applicator) configPath() (string, error) {
	if !filepath.IsAbs(a.ConfigPath) || filepath.Clean(a.ConfigPath) != a.ConfigPath {
		return "", errors.New("Pacman configuration path must be a clean absolute path")
	}
	return a.ConfigPath, nil
}

func (a *Applicator) fragment() string {
	level := "Required DatabaseRequired"
	if a.Repository.SignatureLevel == models.PacmanSignatureRequiredDatabaseOptional {
		level = "Required DatabaseOptional"
	}
	lines := []string{
		"# Managed by Remotr. Do not edit.",
		"[options]",
		"Architecture = " + a.Repository.Architecture,
		"",
		"[" + a.Repository.Name + "]",
		"SigLevel = " + level,
	}
	for _, server := range a.Repository.Servers {
		lines = append(lines, "Server = "+server)
	}
	if a.Repository.Lifecycle != models.LifecycleDisabled {
		return strings.Join(lines, "\n") + "\n"
	}
	disabled := []string{"# Disabled by Remotr."}
	for _, line := range lines[1:] {
		if line == "" {
			disabled = append(disabled, "#")
		} else {
			disabled = append(disabled, "# "+line)
		}
	}
	return strings.Join(disabled, "\n") + "\n"
}

func (a *Applicator) validateNative(ctx context.Context, path string, requireRepository bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	args := []string{"--config", path}
	if requireRepository {
		args = append(args, "--repo", a.Repository.Name)
	}
	if _, _, err := a.Runner.Run("pacman-conf", args...); err != nil {
		return fmt.Errorf("Pacman repository %q native validation failed", a.Repository.Name)
	}
	return nil
}

func (a *Applicator) stageValidateAndActivate(ctx context.Context, path string, content []byte, mode os.FileMode, prefix string, requireRepository bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Pacman configuration directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), prefix)
	if err != nil {
		return fmt.Errorf("stage Pacman repository fragment: %w", err)
	}
	stagedPath := temporary.Name()
	defer os.Remove(stagedPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("stage Pacman repository fragment: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("stage Pacman repository fragment: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("stage Pacman repository fragment: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("stage Pacman repository fragment: %w", err)
	}
	if err := a.validateNative(ctx, stagedPath, requireRepository); err != nil {
		return err
	}
	if err := os.Rename(stagedPath, path); err != nil {
		return fmt.Errorf("activate Pacman repository fragment: %w", err)
	}
	return nil
}

type boundaryState uint8

const (
	boundaryAbsent boundaryState = iota
	boundaryPresent
	boundaryMalformed
)

func (a *Applicator) includeBoundary() string {
	return "\n" + includeBoundaryBegin + "\nInclude = " + filepath.Join(a.FragmentsDir, "*.conf") + "\n" + includeBoundaryEnd + "\n"
}

func (a *Applicator) inspectBoundary(content []byte) boundaryState {
	beginCount := bytes.Count(content, []byte(includeBoundaryBegin))
	endCount := bytes.Count(content, []byte(includeBoundaryEnd))
	if beginCount == 0 && endCount == 0 {
		return boundaryAbsent
	}
	if beginCount == 1 && endCount == 1 && bytes.Count(content, []byte(a.includeBoundary())) == 1 {
		return boundaryPresent
	}
	return boundaryMalformed
}

func (a *Applicator) readConfig() ([]byte, os.FileMode, error) {
	path, err := a.configPath()
	if err != nil {
		return nil, 0, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read Pacman configuration: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errors.New("Pacman configuration must be a regular file")
	}
	content, err := os.ReadFile(path) // #nosec G304 -- fixed provider configuration path.
	if err != nil {
		return nil, 0, fmt.Errorf("read Pacman configuration: %w", err)
	}
	return content, info.Mode().Perm(), nil
}

func (a *Applicator) preflightBoundary() ([]byte, os.FileMode, error) {
	content, mode, err := a.readConfig()
	if err != nil {
		return nil, 0, err
	}
	if a.inspectBoundary(content) == boundaryMalformed {
		return nil, 0, errors.New("managed include boundary is malformed")
	}
	return content, mode, nil
}

func (a *Applicator) reconcileBoundary(ctx context.Context) error {
	content, mode, err := a.preflightBoundary()
	if err != nil {
		return err
	}
	required, err := a.managedRepositoryRequiresInclude()
	if err != nil {
		return err
	}
	state := a.inspectBoundary(content)
	boundary := []byte(a.includeBoundary())
	desired := content
	switch {
	case required && state == boundaryAbsent:
		desired = append(bytes.Clone(content), boundary...)
	case !required && state == boundaryPresent:
		desired = bytes.Replace(content, boundary, nil, 1)
	default:
		return nil
	}
	path, err := a.configPath()
	if err != nil {
		return err
	}
	requireRepository := required && a.Repository.Lifecycle == models.LifecyclePresent
	return a.stageValidateAndActivate(ctx, path, desired, mode, ".remotr-pacman-config-", requireRepository)
}

func (a *Applicator) managedRepositoryRequiresInclude() (bool, error) {
	entries, err := os.ReadDir(a.FragmentsDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read managed Pacman repository fragments: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".conf" {
			continue
		}
		path := filepath.Join(a.FragmentsDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("managed Pacman repository fragment %q must be a regular file", entry.Name())
		}
		content, err := os.ReadFile(path) // #nosec G304 -- entry is below the fixed owned directory.
		if err != nil {
			return false, err
		}
		if hasActiveRepositorySection(content) {
			return true, nil
		}
	}
	return false, nil
}

func hasActiveRepositorySection(content []byte) bool {
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") || len(line) < 3 || line[0] != '[' || line[len(line)-1] != ']' {
			continue
		}
		if !strings.EqualFold(line, "[options]") {
			return true
		}
	}
	return false
}

type ownedFragmentSnapshot struct {
	exists  bool
	content []byte
	mode    os.FileMode
}

func captureOwnedFragment(path string) (ownedFragmentSnapshot, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ownedFragmentSnapshot{}, nil
	}
	if err != nil {
		return ownedFragmentSnapshot{}, fmt.Errorf("inspect owned Pacman repository fragment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ownedFragmentSnapshot{}, errors.New("owned Pacman repository fragment must be a regular file before activation")
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated resource name below a fixed root.
	if err != nil {
		return ownedFragmentSnapshot{}, fmt.Errorf("read owned Pacman repository fragment: %w", err)
	}
	return ownedFragmentSnapshot{exists: true, content: content, mode: info.Mode().Perm()}, nil
}

func restoreAfterPartialFailure(path string, previous ownedFragmentSnapshot, cause error) error {
	if err := restoreOwnedFragment(path, previous); err != nil {
		return fmt.Errorf("%w; restore previous Pacman repository fragment: %v", cause, err)
	}
	return cause
}

func restoreOwnedFragment(path string, previous ownedFragmentSnapshot) error {
	if !previous.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-pacman-recovery-")
	if err != nil {
		return err
	}
	stagedPath := temporary.Name()
	defer os.Remove(stagedPath)
	if err := temporary.Chmod(previous.mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(previous.content); err != nil {
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
	return os.Rename(stagedPath, path)
}

func contentMatches(path, want string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != mode {
		return errFragmentMismatch
	}
	got, err := os.ReadFile(path) // #nosec G304 -- validated resource name below a fixed root.
	if err != nil {
		return err
	}
	if string(got) != want {
		return errFragmentMismatch
	}
	return nil
}

func compliant(desired executor.RedactedSummary) executor.CheckResult {
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func drift(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{
		Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
		DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed),
	}
}

func checkFailure(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

func unsupportedCredentialError() error {
	return errors.New("Pacman repository credentials are not supported by the native provider")
}
