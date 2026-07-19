// Package sudo manages validated, named sudoers.d fragments.
package sudo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/applicators/files"
	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

// Applicator stages a complete sudoers include tree before atomically
// activating the resource's one fragment.
type Applicator struct {
	Resource          models.SudoResource
	SudoersDir        string
	SudoersPath       string
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
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	return a.rollback.Preflight(ctx, path)
}

// New creates an applicator. A runner may be supplied by the registry to
// retain an exact, injectable visudo argv boundary.
func New(resource models.SudoResource, runners ...executil.Runner) *Applicator {
	runner := executil.Runner(executil.SanitizedOSRunner{})
	if len(runners) > 0 && runners[0] != nil {
		runner = runners[0]
	}
	return &Applicator{
		Resource: resource, SudoersDir: "/etc/sudoers.d", SudoersPath: "/etc/sudoers", Runner: runner,
		LookupRecovery: func(principal string) error {
			_, err := user.Lookup(principal)
			return err
		},
	}
}

func (a *Applicator) Name() string { return "sudo:" + a.Resource.Name }

func (a *Applicator) Description() string { return "sudo fragment " + a.Resource.Name }

func (a *Applicator) fragmentPath() (string, error) {
	if !filepath.IsAbs(a.SudoersDir) {
		return "", fmt.Errorf("sudoers directory must be absolute")
	}
	return filepath.Join(a.SudoersDir, a.Resource.Name), nil
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	path, err := a.fragmentPath()
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated sudoers.d fragment path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return nil, os.IsNotExist(err)
	}
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o440 {
		return nil, false
	}
	return nil, string(content) == a.render()
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	_, compliant := a.State(ctx)
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: "validated sudo fragment"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "validated sudo fragment"}
}

// Preflight ensures all declared recovery identities are available before a
// resource can modify access policy. Engine policy additionally leaves this
// access-risk resource report-only until enforce: true is declared.
func (a *Applicator) Preflight(_ context.Context) error {
	if len(a.Resource.RecoveryPrincipals) == 0 {
		return fmt.Errorf("sudo %q has no recovery principal", a.Resource.Name)
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
	if err := a.Preflight(ctx); err != nil {
		return err
	}
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path) // #nosec G304 -- validated sudoers.d fragment path.
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent && !exists {
		return appErr.ErrStateAlreadyMet
	}
	desired := a.render()
	if a.Resource.Lifecycle == models.LifecyclePresent && exists && string(current) == desired {
		info, statErr := os.Stat(path)
		if statErr == nil && info.Mode().Perm() == 0o440 {
			return appErr.ErrStateAlreadyMet
		}
	}
	if err := a.validateCandidate(ctx, a.Resource.Lifecycle == models.LifecyclePresent, desired); err != nil {
		return err
	}
	if a.rollback != nil {
		if err := a.rollback.Arm(ctx, path); err != nil {
			return err
		}
	} else {
		a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), current...), exists, true
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return os.Remove(path) // #nosec G703 -- validated sudoers.d fragment path.
	}
	return files.New(models.File{Name: a.Resource.Name, Path: path, Content: desired, Mode: []int{0o440}}).Apply(ctx)
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	if err == nil {
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
	return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
}

// Revert restores the protected transaction when registry configuration is
// present. Directly constructed providers retain local compatibility state,
// but ApplyResult deliberately advertises no rollback for that case.
func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback != nil {
		err := a.rollback.Rollback(ctx)
		if errors.Is(err, os.ErrNotExist) {
			return appErr.ErrNoOp
		}
		return err
	}
	path, err := a.fragmentPath()
	if err != nil {
		return err
	}
	if !a.rollbackArmed {
		return appErr.ErrNoOp
	}
	if !a.previousExists {
		return os.Remove(path) // #nosec G703 -- validated sudoers.d fragment path.
	}
	return files.New(models.File{Name: a.Resource.Name, Path: path, Content: string(a.previous), Mode: []int{0o440}}).Apply(ctx)
}

func (a *Applicator) render() string {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		return ""
	}
	runAs := "ALL"
	if len(a.Resource.RunAs) > 0 {
		runAs = strings.Join(a.Resource.RunAs, ",")
	}
	prefix := strings.Join(a.Resource.Subjects, ",") + " ALL=(" + runAs + ") "
	if len(a.Resource.Tags) > 0 {
		prefix += strings.Join(a.Resource.Tags, ": ") + ": "
	}
	return prefix + strings.Join(a.Resource.Commands, ", ") + "\n"
}

func (a *Applicator) validateCandidate(ctx context.Context, present bool, content string) error {
	stageRoot, err := os.MkdirTemp("", "remotr-sudoers-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	stageDir := filepath.Join(stageRoot, "sudoers.d")
	if err := copyDir(a.SudoersDir, stageDir); err != nil {
		return err
	}
	main, err := os.ReadFile(a.SudoersPath) // #nosec G304 -- fixed root sudoers path.
	if err != nil {
		return err
	}
	include := "#includedir " + a.SudoersDir
	if !strings.Contains(string(main), include) {
		return fmt.Errorf("sudoers %q does not include managed fragment directory %q", a.SudoersPath, a.SudoersDir)
	}
	stageMain := filepath.Join(stageRoot, "sudoers")
	main = []byte(strings.Replace(string(main), include, "#includedir "+stageDir, 1))
	if err := os.WriteFile(stageMain, main, 0o440); err != nil {
		return err
	}
	stageFragment := filepath.Join(stageDir, a.Resource.Name)
	if present {
		if err := os.Remove(stageFragment); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(stageFragment, []byte(content), 0o440); err != nil {
			return err
		}
	} else if err := os.Remove(stageFragment); err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.ValidateEffective != nil {
		return a.ValidateEffective(ctx, stageMain, stageDir)
	}
	_, stderr, err := a.Runner.Run("visudo", "-cf", stageMain)
	if err != nil {
		return fmt.Errorf("visudo rejected staged effective sudoers: %s", strings.TrimSpace(string(stderr)))
	}
	return nil
}

func copyDir(source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		sourcePath := filepath.Join(source, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sudoers fragment %q is not a regular file", sourcePath)
		}
		content, err := os.ReadFile(sourcePath) // #nosec G304 -- enumerated root-owned sudoers.d entry.
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), content, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
