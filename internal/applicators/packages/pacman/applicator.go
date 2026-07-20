// Package pacman manages native Arch repository packages through pacman.
package pacman

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	serviceactions "github.com/DavidHoenisch/remotr/internal/applicators/services"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Applicator manages native repository packages via pacman on Arch.
type Applicator struct {
	Package      models.Package
	Exec         executil.Runner
	cacheRefresh func(context.Context) error
}

func New(pkg models.Package, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.SanitizedOSRunner{}
	}
	a := &Applicator{Package: pkg, Exec: exec}
	a.cacheRefresh = func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, stderr, err := a.Exec.Run("pacman", "-Sy", "--noconfirm")
		if err != nil {
			return fmt.Errorf("pacman metadata refresh failed: %s: %w", bounded(stderr), err)
		}
		return nil
	}
	return a
}

func (a *Applicator) Name() string { return "pacman:" + a.Package.Name }

func (a *Applicator) Description() string { return "pacman package " + a.Package.Name }

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	if a.Package.Lifecycle == models.LifecyclePurged {
		return executor.CheckResult{
			Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable,
			DesiredSummary: "purged lifecycle", ObservedSummary: "pacman does not support package purge",
		}
	}
	actual, compliant := a.State(ctx)
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, Actual: actual}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, Actual: actual}
}

func (a *Applicator) installedVersion() (string, bool) {
	out, _, err := a.Exec.Run("pacman", "-Q", a.Package.Name)
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", true
	}
	return fields[len(fields)-1], true
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	version, inst := a.installedVersion()
	if a.Package.Present {
		if a.Package.Version != "" {
			return version, inst && version == a.Package.Version
		}
		return inst, inst
	}
	return inst, !inst
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.Package.Lifecycle == models.LifecyclePurged {
		return fmt.Errorf("pacman package %q: lifecycle %q is unsupported", a.Package.Name, models.LifecyclePurged)
	}
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Package.Present {
		if a.Package.RefreshCache {
			if err := a.RefreshCache(ctx); err != nil {
				return err
			}
		}
		if a.Package.Version != "" {
			available, err := a.availableVersion()
			if err != nil {
				return err
			}
			if available != a.Package.Version {
				return fmt.Errorf("pacman package %q version %s is unavailable (repository offers %s)", a.Package.Name, a.Package.Version, available)
			}
			artifact, err := a.resolveArtifact()
			if err != nil {
				return err
			}
			if artifact.Version != a.Package.Version {
				return fmt.Errorf(
					"pacman package %q resolved artifact %q version %q after repository offered %q; refusing changed resolution",
					a.Package.Name, artifact.Name, artifact.Version, available,
				)
			}
			installed, present := a.installedVersion()
			if present && installed != a.Package.Version {
				out, _, err := a.Exec.Run("vercmp", installed, a.Package.Version)
				if err != nil {
					return fmt.Errorf("compare pacman versions: %w", err)
				}
				comparison, err := strconv.Atoi(strings.TrimSpace(string(out)))
				if err != nil {
					return fmt.Errorf("parse pacman version comparison: %w", err)
				}
				if comparison > 0 && (a.Package.AllowDowngrade == nil || !*a.Package.AllowDowngrade) {
					return fmt.Errorf("pacman package %q downgrade from %s to %s is not permitted", a.Package.Name, installed, a.Package.Version)
				}
				if comparison < 0 && a.Package.AllowUpgrade != nil && !*a.Package.AllowUpgrade {
					return fmt.Errorf("pacman package %q upgrade from %s to %s is not permitted", a.Package.Name, installed, a.Package.Version)
				}
			}
			if _, stderr, err := a.Exec.Run("pacman", "-U", "--noconfirm", artifact.Location); err != nil {
				return fmt.Errorf("install resolved pacman package %q: %s: %w", a.Package.Name, bounded(stderr), err)
			}
			return nil
		}
		_, stderr, err := a.Exec.Run("pacman", "-S", "--noconfirm", a.Package.Name)
		if err != nil {
			return fmt.Errorf("install pacman package %q: %s: %w", a.Package.Name, bounded(stderr), err)
		}
		return nil
	}
	remove := "-R"
	if a.Package.RemoveDependencies {
		remove = "-Rs"
	}
	_, stderr, err := a.Exec.Run("pacman", remove, "--noconfirm", a.Package.Name)
	if err != nil {
		return fmt.Errorf("remove pacman package %q: %s: %w", a.Package.Name, bounded(stderr), err)
	}
	return nil
}

// SetCacheRefresh supplies the engine-scoped Pacman metadata refresh boundary.
func (a *Applicator) SetCacheRefresh(refresh func(context.Context) error) {
	if refresh != nil {
		a.cacheRefresh = refresh
	}
}

// RefreshCache performs metadata refresh through the configured shared
// boundary. It is public only to the engine's provider contract.
func (a *Applicator) RefreshCache(ctx context.Context) error {
	if refresh, ok := executor.PackageMetadataRefresh(ctx, "pacman"); ok {
		return refresh(ctx)
	}
	if a.cacheRefresh == nil {
		return nil
	}
	return a.cacheRefresh(ctx)
}

type resolvedArtifact struct {
	Name     string
	Version  string
	Location string
}

func (a *Applicator) resolveArtifact() (resolvedArtifact, error) {
	const format = "%n\t%v\t%l"
	out, stderr, err := a.Exec.Run("pacman", "-Sp", "--print-format", format, a.Package.Name)
	if err != nil {
		return resolvedArtifact{}, fmt.Errorf("resolve pacman package %q: %s: %w", a.Package.Name, bounded(stderr), err)
	}
	var resolved resolvedArtifact
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			return resolvedArtifact{}, fmt.Errorf("resolve pacman package %q returned malformed artifact metadata", a.Package.Name)
		}
		candidate := resolvedArtifact{
			Name: strings.TrimSpace(fields[0]), Version: strings.TrimSpace(fields[1]), Location: strings.TrimSpace(fields[2]),
		}
		if candidate.Name != a.Package.Name {
			continue
		}
		if candidate.Version == "" || candidate.Location == "" {
			return resolvedArtifact{}, fmt.Errorf("resolve pacman package %q returned incomplete artifact metadata", a.Package.Name)
		}
		if resolved.Name != "" {
			return resolvedArtifact{}, fmt.Errorf("resolve pacman package %q returned multiple target artifacts", a.Package.Name)
		}
		resolved = candidate
	}
	if resolved.Name == "" {
		return resolvedArtifact{}, fmt.Errorf("resolve pacman package %q returned no target artifact", a.Package.Name)
	}
	return resolved, nil
}

func (a *Applicator) availableVersion() (string, error) {
	out, stderr, err := a.Exec.Run("pacman", "-Si", a.Package.Name)
	if err != nil {
		return "", fmt.Errorf("query pacman package %q: %s: %w", a.Package.Name, bounded(stderr), err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(key) == "Version" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("query pacman package %q returned no version", a.Package.Name)
}

func bounded(value []byte) string {
	const max = 1024
	value = []byte(strings.TrimSpace(string(value)))
	if len(value) > max {
		value = value[:max]
	}
	return string(value)
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if err == appErr.ErrStateAlreadyMet {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	result.Activation = append(result.Activation, serviceactions.ActivationSignals(a.Package.Notifications)...)
	return result
}
