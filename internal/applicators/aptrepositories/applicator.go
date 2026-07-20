// Package aptrepositories manages owned APT source and preference fragments.
package aptrepositories

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// ResolveCredential obtains a protected APT auth.conf stanza. The reference,
// rather than the secret itself, belongs in desired state.
type ResolveCredential func(context.Context, string) (string, error)

// Applicator owns only files prefixed by remotr- and the resource name.
type Applicator struct {
	Repository        models.APTRepository
	SourcesDir        string
	PreferencesDir    string
	AuthDir           string
	Runner            executil.Runner
	ResolveCredential ResolveCredential
	rollback          *filetx.Handle
}

// ConfigureRollback protects the source, priority, and credential fragments
// as one sensitive transaction.
func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := filetx.New(store, address, artifactDigest, true)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
}

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	sourcePath, err := a.sourcePath()
	if err != nil {
		return err
	}
	preferencePath, err := a.preferencePath()
	if err != nil {
		return err
	}
	authPath, err := a.authPath()
	if err != nil {
		return err
	}
	return a.rollback.Preflight(ctx, sourcePath, preferencePath, authPath)
}

// New creates the APT repository applicator.
func New(repository models.APTRepository, runner executil.Runner) *Applicator {
	if repository.Lifecycle == "" {
		repository.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{
		Repository: repository, SourcesDir: "/etc/apt/sources.list.d", PreferencesDir: "/etc/apt/preferences.d", AuthDir: "/etc/apt/auth.conf.d", Runner: runner,
		ResolveCredential: func(_ context.Context, reference string) (string, error) { return secrets.ReadFileRef(reference) },
	}
}

func (a *Applicator) Name() string { return "apt-repository:" + a.Repository.Name }

func (a *Applicator) Description() string { return "APT repository " + a.Repository.Name }

func (a *Applicator) sourcePath() (string, error)     { return a.ownedPath(a.SourcesDir, ".list") }
func (a *Applicator) preferencePath() (string, error) { return a.ownedPath(a.PreferencesDir, ".pref") }
func (a *Applicator) authPath() (string, error)       { return a.ownedPath(a.AuthDir, ".conf") }

func (a *Applicator) ownedPath(dir, suffix string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("APT repository directory must be absolute")
	}
	if err := a.Repository.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(dir, "remotr-"+a.Repository.Name+suffix), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("owned APT repository " + a.Repository.Name)
	sourcePath, err := a.sourcePath()
	if err != nil {
		return checkFailure(desired, err)
	}
	preferencePath, err := a.preferencePath()
	if err != nil {
		return checkFailure(desired, err)
	}
	authPath, err := a.authPath()
	if err != nil {
		return checkFailure(desired, err)
	}
	if a.Repository.Lifecycle == models.LifecycleAbsent {
		for _, path := range []string{sourcePath, preferencePath, authPath} {
			if _, err := os.Stat(path); err == nil {
				return drift(desired, "owned fragment exists")
			} else if !os.IsNotExist(err) {
				return checkFailure(desired, err)
			}
		}
		return compliant(desired)
	}
	if err := contentMatches(sourcePath, a.sourceFragment(), 0o644); err != nil {
		if os.IsNotExist(err) || errors.Is(err, errContentMismatch) {
			return drift(desired, "source fragment differs")
		}
		return checkFailure(desired, err)
	}
	if a.Repository.Priority == 0 {
		if _, err := os.Stat(preferencePath); err == nil {
			return drift(desired, "owned preference fragment exists")
		} else if !os.IsNotExist(err) {
			return checkFailure(desired, err)
		}
	} else if err := contentMatches(preferencePath, a.preferenceFragment(), 0o644); err != nil {
		if os.IsNotExist(err) || errors.Is(err, errContentMismatch) {
			return drift(desired, "priority fragment differs")
		}
		return checkFailure(desired, err)
	}
	if a.Repository.CredentialRef == "" {
		if _, err := os.Stat(authPath); err == nil {
			return drift(desired, "owned credential fragment exists")
		} else if !os.IsNotExist(err) {
			return checkFailure(desired, err)
		}
	} else {
		credential, err := a.ResolveCredential(ctx, a.Repository.CredentialRef)
		if err != nil {
			return checkFailure(desired, credentialResolutionError(err))
		}
		if err := contentMatches(authPath, credential+"\n", 0o600); err != nil {
			if os.IsNotExist(err) || errors.Is(err, errContentMismatch) {
				return drift(desired, "credential fragment differs")
			}
			return checkFailure(desired, err)
		}
	}
	return compliant(desired)
}

func (a *Applicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status == executor.CheckFailed {
		return check.Err
	}
	sourcePath, err := a.sourcePath()
	if err != nil {
		return err
	}
	preferencePath, err := a.preferencePath()
	if err != nil {
		return err
	}
	authPath, err := a.authPath()
	if err != nil {
		return err
	}
	if a.Repository.Lifecycle == models.LifecycleAbsent {
		if a.rollback != nil {
			if err := a.rollback.Arm(ctx, sourcePath, preferencePath, authPath); err != nil {
				return err
			}
		}
		for _, path := range []string{sourcePath, preferencePath, authPath} {
			if err := removeOwned(path); err != nil {
				return err
			}
		}
		return nil
	}
	credential := ""
	if a.Repository.CredentialRef != "" {
		credential, err = a.ResolveCredential(ctx, a.Repository.CredentialRef)
		if err != nil {
			return credentialResolutionError(err)
		}
	}
	if a.rollback != nil {
		if err := a.rollback.Arm(ctx, sourcePath, preferencePath, authPath); err != nil {
			return err
		}
	}
	if err := atomicWrite(sourcePath, []byte(a.sourceFragment()), 0o644); err != nil {
		return fmt.Errorf("write APT source fragment: %w", err)
	}
	if a.Repository.Priority == 0 {
		if err := removeOwned(preferencePath); err != nil {
			return err
		}
	} else if err := atomicWrite(preferencePath, []byte(a.preferenceFragment()), 0o644); err != nil {
		return fmt.Errorf("write APT preference fragment: %w", err)
	}
	if a.Repository.CredentialRef == "" {
		if err := removeOwned(authPath); err != nil {
			return err
		}
	} else if err := atomicWrite(authPath, []byte(credential+"\n"), 0o600); err != nil {
		return fmt.Errorf("write APT credential fragment: %w", err)
	}
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
}

func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback == nil {
		return appErr.ErrNoOp
	}
	err := a.rollback.Rollback(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return appErr.ErrNoOp
	}
	return err
}

func (a *Applicator) sourceFragment() string {
	var options []string
	if len(a.Repository.Architectures) > 0 {
		options = append(options, "arch="+strings.Join(a.Repository.Architectures, ","))
	}
	options = append(options, "signed-by=/etc/apt/keyrings/"+a.Repository.SigningKey+".gpg")
	prefix := "deb [" + strings.Join(options, " ") + "] " + a.Repository.URL + " "
	var lines []string
	for _, suite := range a.Repository.Suites {
		lines = append(lines, prefix+suite+" "+strings.Join(a.Repository.Components, " "))
	}
	line := strings.Join(lines, "\n")
	if a.Repository.Lifecycle == models.LifecycleDisabled {
		var disabled []string
		for _, entry := range lines {
			disabled = append(disabled, "# "+entry)
		}
		return "# disabled by Remotr\n" + strings.Join(disabled, "\n") + "\n"
	}
	return line + "\n"
}

func (a *Applicator) preferenceFragment() string {
	u, _ := urlHost(a.Repository.URL)
	return fmt.Sprintf("Package: *\nPin: origin %q\nPin-Priority: %d\n", u, a.Repository.Priority)
}

func urlHost(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return parsed.Hostname(), nil
}

var errContentMismatch = errors.New("content differs")

func contentMatches(path, want string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != mode {
		return errContentMismatch
	}
	got, err := os.ReadFile(path) // #nosec G304 -- path is constructed from validated resource name.
	if err != nil {
		return err
	}
	if string(got) != want {
		return errContentMismatch
	}
	return nil
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-apt-repository-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
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

func removeOwned(path string) error {
	err := os.Remove(path) // #nosec G703 -- path is constructed from validated resource name.
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func compliant(desired executor.RedactedSummary) executor.CheckResult {
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
}

func drift(desired executor.RedactedSummary, observed string) executor.CheckResult {
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(observed)}
}

func checkFailure(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

func credentialResolutionError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("repository credential reference could not be resolved")
}
