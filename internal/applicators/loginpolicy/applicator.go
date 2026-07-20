// Package loginpolicy manages provider-owned Debian/Ubuntu PAM profiles.
package loginpolicy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

type Applicator struct {
	Resource          models.LoginPolicyResource
	ProfilesDir       string
	PAMDir            string
	Runner            executil.Runner
	LookupRecovery    func(string) error
	ValidateEffective func(context.Context, string, string) error
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

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	path, err := a.profilePath()
	if err != nil {
		return err
	}
	return a.rollback.Preflight(ctx, path)
}

func New(resource models.LoginPolicyResource, runners ...executil.Runner) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	if resource.Priority == 0 {
		resource.Priority = 900
	}
	runner := executil.Runner(executil.SanitizedOSRunner{})
	if len(runners) > 0 && runners[0] != nil {
		runner = runners[0]
	}
	return &Applicator{
		Resource: resource, ProfilesDir: "/usr/share/pam-configs", PAMDir: "/etc/pam.d", Runner: runner,
		LookupRecovery: func(principal string) error { _, err := user.Lookup(principal); return err },
	}
}

func (a *Applicator) Name() string { return "login-policy:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return "provider-owned PAM login policy " + a.Resource.Name
}

func (a *Applicator) profilePath() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.ProfilesDir) || !filepath.IsAbs(a.PAMDir) {
		return "", fmt.Errorf("PAM provider directories must be absolute")
	}
	return filepath.Join(a.ProfilesDir, "remotr-"+a.Resource.Name), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(_ context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("named provider-owned PAM policy " + a.Resource.Name)
	path, err := a.profilePath()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated provider-owned profile path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named profile is absent"}
		}
		if err != nil {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named profile exists"}
	}
	if os.IsNotExist(err) {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named profile is absent"}
	}
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	info, err := os.Stat(path)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	if string(content) == a.render() && info.Mode().Perm() == 0o644 {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: "named profile matches"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named profile differs"}
}

func (a *Applicator) Preflight(_ context.Context) error {
	if len(a.Resource.RecoveryPrincipals) == 0 {
		return fmt.Errorf("login policy %q has no recovery principal", a.Resource.Name)
	}
	lookup := a.LookupRecovery
	if lookup == nil {
		lookup = func(principal string) error { _, err := user.Lookup(principal); return err }
	}
	for _, principal := range a.Resource.RecoveryPrincipals {
		if err := lookup(principal); err != nil {
			return fmt.Errorf("recovery principal %q: %w", principal, err)
		}
	}
	return nil
}

func (a *Applicator) Apply(ctx context.Context) error {
	path, err := a.profilePath()
	if err != nil {
		return err
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if err := a.Preflight(ctx); err != nil {
		return err
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated provider-owned profile path.
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
	if err := a.activateCandidate(path); err != nil {
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
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
}

func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback != nil {
		err := a.rollback.RollbackThen(ctx, func() error {
			if _, stderr, err := a.Runner.Run("pam-auth-update", "--package"); err != nil {
				return fmt.Errorf("pam-auth-update failed while restoring prior policy: %s", strings.TrimSpace(string(stderr)))
			}
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			return appErr.ErrNoOp
		}
		return err
	}
	if !a.rollbackArmed {
		return appErr.ErrNoOp
	}
	path, err := a.profilePath()
	if err != nil {
		return err
	}
	if err := restore(path, a.previous, a.previousExists); err != nil {
		return err
	}
	if _, stderr, err := a.Runner.Run("pam-auth-update", "--package"); err != nil {
		return fmt.Errorf("pam-auth-update failed while restoring prior policy: %s", strings.TrimSpace(string(stderr)))
	}
	a.previous = nil
	a.rollbackArmed = false
	return nil
}

func (a *Applicator) validateCandidate(ctx context.Context) error {
	if err := validateAvailableModules(a.Resource.Rules); err != nil {
		return err
	}
	stageRoot, err := os.MkdirTemp("", "remotr-pam-policy-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	stagedProfiles := filepath.Join(stageRoot, "pam-configs")
	stagedPAM := filepath.Join(stageRoot, "pam.d")
	if err := copyFlatDir(a.ProfilesDir, stagedProfiles); err != nil {
		return err
	}
	if err := copyFlatDir(a.PAMDir, stagedPAM); err != nil {
		return err
	}
	stagedProfile := filepath.Join(stagedProfiles, "remotr-"+a.Resource.Name)
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(stagedProfile); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := os.WriteFile(stagedProfile, []byte(a.render()), 0o644); err != nil {
		return err
	}
	if a.ValidateEffective != nil {
		return a.ValidateEffective(ctx, stagedProfiles, stagedPAM)
	}
	return validateEffectiveTree(stagedProfiles, stagedPAM)
}

func validateAvailableModules(rules []models.PAMRule) error {
	for _, rule := range rules {
		if pamModuleAvailable(rule.Module) {
			continue
		}
		return fmt.Errorf("PAM module %q is unavailable", rule.Module)
	}
	return nil
}

func pamModuleAvailable(module string) bool {
	if filepath.IsAbs(module) {
		return regularFile(module)
	}
	directories := []string{
		"/lib/security",
		"/lib64/security",
		"/usr/lib/security",
		"/usr/lib64/security",
	}
	for _, pattern := range []string{"/lib/*/security", "/usr/lib/*/security"} {
		matches, _ := filepath.Glob(pattern)
		directories = append(directories, matches...)
	}
	for _, directory := range directories {
		if regularFile(filepath.Join(directory, module)) {
			return true
		}
	}
	return false
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (a *Applicator) activateCandidate(path string) error {
	previous, err := os.ReadFile(path) // #nosec G304 -- validated provider-owned profile path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if err := atomicWrite(path, []byte(a.render()), 0o644); err != nil {
		return err
	}
	if _, stderr, err := a.Runner.Run("pam-auth-update", "--package"); err != nil {
		restoreErr := restore(path, previous, previousExists)
		_, _, recoveryErr := a.Runner.Run("pam-auth-update", "--package")
		return fmt.Errorf("pam-auth-update rejected provider profile: %s (profile restore: %v; stack restore: %v)", strings.TrimSpace(string(stderr)), restoreErr, recoveryErr)
	}
	return nil
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Name: Remotr %s\nDefault: yes\nPriority: %d\n", a.Resource.Name, a.Resource.Priority)
	sections := []struct {
		kind            models.PAMSection
		label           string
		profileType     string
		interactiveOnly bool
	}{
		{models.PAMAuth, "Auth", "Primary", false},
		{models.PAMAccount, "Account", "Primary", false},
		{models.PAMPassword, "Password", "Primary", false},
		{models.PAMSession, "Session", "Additional", false},
		{models.PAMSessionInteractive, "Session", "Additional", true},
	}
	for _, section := range sections {
		wroteHeader := false
		for _, rule := range a.Resource.Rules {
			if rule.Section != section.kind {
				continue
			}
			if !wroteHeader {
				fmt.Fprintf(&out, "%s-Type: %s\n", section.label, section.profileType)
				if section.interactiveOnly {
					out.WriteString("Session-Interactive-Only: yes\n")
				}
				fmt.Fprintf(&out, "%s:\n", section.label)
				wroteHeader = true
			}
			fmt.Fprintf(&out, "\t%s\t%s", rule.Control, rule.Module)
			if len(rule.Arguments) > 0 {
				out.WriteByte(' ')
				out.WriteString(strings.Join(rule.Arguments, " "))
			}
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func validateEffectiveTree(profilesDir, pamDir string) error {
	for _, dir := range []string{profilesDir, pamDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 -- enumerated isolated stage.
			if err != nil {
				return err
			}
			if strings.ContainsRune(string(content), '\x00') {
				return fmt.Errorf("PAM stack file %q contains NUL", entry.Name())
			}
			if dir == pamDir {
				if err := validatePAMServiceFile(filepath.Join(dir, entry.Name()), pamDir, content); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validatePAMServiceFile(path, pamDir string, content []byte) error {
	for lineIndex, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] == "@include" {
			if len(fields) != 2 {
				return fmt.Errorf("PAM stack %q line %d has malformed @include", filepath.Base(path), lineIndex+1)
			}
			if err := validatePAMInclude(pamDir, fields[1]); err != nil {
				return fmt.Errorf("PAM stack %q line %d: %w", filepath.Base(path), lineIndex+1, err)
			}
			continue
		}
		if len(fields) < 3 {
			return fmt.Errorf("PAM stack %q line %d requires type, control, and module", filepath.Base(path), lineIndex+1)
		}
		pamType := strings.TrimPrefix(fields[0], "-")
		switch pamType {
		case "auth", "account", "password", "session":
		default:
			return fmt.Errorf("PAM stack %q line %d has unknown type %q", filepath.Base(path), lineIndex+1, fields[0])
		}
		moduleIndex := 2
		if strings.HasPrefix(fields[1], "[") {
			moduleIndex = -1
			for i := 1; i < len(fields); i++ {
				if strings.HasSuffix(fields[i], "]") {
					moduleIndex = i + 1
					break
				}
			}
			if moduleIndex < 0 || moduleIndex >= len(fields) {
				return fmt.Errorf("PAM stack %q line %d has malformed bracketed control", filepath.Base(path), lineIndex+1)
			}
		} else {
			switch fields[1] {
			case "required", "requisite", "sufficient", "optional":
			case "include", "substack":
				if err := validatePAMInclude(pamDir, fields[2]); err != nil {
					return fmt.Errorf("PAM stack %q line %d: %w", filepath.Base(path), lineIndex+1, err)
				}
				continue
			default:
				return fmt.Errorf("PAM stack %q line %d has unknown control %q", filepath.Base(path), lineIndex+1, fields[1])
			}
		}
		module := fields[moduleIndex]
		if strings.ContainsAny(module, "\x00/\\") && !filepath.IsAbs(module) {
			return fmt.Errorf("PAM stack %q line %d has invalid module path", filepath.Base(path), lineIndex+1)
		}
		if !strings.HasSuffix(module, ".so") {
			return fmt.Errorf("PAM stack %q line %d has invalid module %q", filepath.Base(path), lineIndex+1, module)
		}
		if !pamModuleAvailable(module) {
			return fmt.Errorf("PAM stack %q line %d references unavailable module %q", filepath.Base(path), lineIndex+1, module)
		}
	}
	return nil
}

func validatePAMInclude(pamDir, service string) error {
	if service == "" || filepath.Base(service) != service || strings.HasPrefix(service, ".") {
		return fmt.Errorf("invalid included PAM service %q", service)
	}
	info, err := os.Stat(filepath.Join(pamDir, service))
	if err != nil {
		return fmt.Errorf("included PAM service %q is unavailable: %w", service, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("included PAM service %q is not regular", service)
	}
	return nil
}

func copyFlatDir(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("PAM provider file %q is not regular", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(source, entry.Name())) // #nosec G304 -- enumerated provider directory entry.
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), content, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-pam-profile-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(mode); err != nil {
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

func restore(path string, previous []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return atomicWrite(path, previous, 0o644)
}
