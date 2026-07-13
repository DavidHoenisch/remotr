package apt

import (
	"context"
	"fmt"
	"strings"

	serviceactions "github.com/DavidHoenisch/remotr/internal/applicators/services"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

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
		_, stderr, err := a.Exec.Run("apt-get", "update")
		if err != nil {
			return fmt.Errorf("apt cache refresh failed: %s: %w", bounded(stderr), err)
		}
		return nil
	}
	return a
}

func (a *Applicator) Name() string { return "apt:" + a.Package.Name }

func (a *Applicator) Description() string { return "apt package " + a.Package.Name }

func (a *Applicator) installedVersion() (string, bool) {
	if a.Package.Version == "" {
		_, _, err := a.Exec.Run("dpkg", "-s", a.Package.Name)
		return "", err == nil
	}
	out, _, err := a.Exec.Run("dpkg-query", "-W", "-f=${Status}\\t${Version}", a.Package.Name)
	if err != nil {
		return "", false
	}
	status, version, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	return version, ok && status == "install ok installed" && version != ""
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	version, inst := a.installedVersion()
	if a.Package.Present {
		packageMet := inst
		if a.Package.Version != "" {
			packageMet = inst && version == a.Package.Version
		}
		if a.Package.Hold != nil && packageMet {
			held := a.held()
			return version, held == *a.Package.Hold
		}
		if a.Package.Version != "" {
			return version, packageMet
		}
		return inst, packageMet
	}
	return inst, !inst
}

func (a *Applicator) Apply(ctx context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Package.Present {
		_, packageMet := a.packageState()
		if a.Package.RefreshCache && !packageMet {
			if err := a.RefreshCache(ctx); err != nil {
				return err
			}
		}
		name := a.Package.Name
		if a.Package.Version != "" {
			installed, present := a.installedVersion()
			if present && installed != a.Package.Version {
				_, _, newerErr := a.Exec.Run("dpkg", "--compare-versions", installed, "gt", a.Package.Version)
				if newerErr == nil && (a.Package.AllowDowngrade == nil || !*a.Package.AllowDowngrade) {
					return fmt.Errorf("apt package %q downgrade from %s to %s is not permitted", a.Package.Name, installed, a.Package.Version)
				}
				if newerErr != nil && a.Package.AllowUpgrade != nil && !*a.Package.AllowUpgrade {
					return fmt.Errorf("apt package %q upgrade from %s to %s is not permitted", a.Package.Name, installed, a.Package.Version)
				}
			}
			name += "=" + a.Package.Version
		}
		if !packageMet {
			_, stderr, err := a.Exec.Run("apt-get", "install", "-y", name)
			if err != nil {
				return fmt.Errorf("apt install %q failed: %s: %w", a.Package.Name, bounded(stderr), err)
			}
		}
		if a.Package.Hold != nil && a.held() != *a.Package.Hold {
			action := "unhold"
			if *a.Package.Hold {
				action = "hold"
			}
			if _, stderr, err := a.Exec.Run("apt-mark", action, a.Package.Name); err != nil {
				return fmt.Errorf("apt-mark %s %q failed: %s: %w", action, a.Package.Name, bounded(stderr), err)
			}
		}
		return nil
	}
	if a.Package.Lifecycle == models.LifecyclePurged {
		_, _, err := a.Exec.Run("apt-get", "purge", "-y", a.Package.Name)
		return err
	}
	args := []string{"remove", "-y"}
	if a.Package.RemoveDependencies {
		args = append(args, "--autoremove")
	}
	args = append(args, a.Package.Name)
	_, _, err := a.Exec.Run("apt-get", args...)
	return err
}

// SetCacheRefresh supplies the engine-scoped APT metadata refresh boundary.
// All APT packages in one engine run receive the same function so a changed
// repository is refreshed once before its dependent package transactions.
func (a *Applicator) SetCacheRefresh(refresh func(context.Context) error) {
	if refresh != nil {
		a.cacheRefresh = refresh
	}
}

// RefreshCache performs metadata refresh through the configured shared
// boundary. It is public only to the engine's provider contract.
func (a *Applicator) RefreshCache(ctx context.Context) error {
	if a.cacheRefresh == nil {
		return nil
	}
	return a.cacheRefresh(ctx)
}

func (a *Applicator) packageState() (any, bool) {
	version, installed := a.installedVersion()
	if a.Package.Version != "" {
		return version, installed && version == a.Package.Version
	}
	return installed, installed
}

func (a *Applicator) held() bool {
	out, _, err := a.Exec.Run("apt-mark", "showhold", a.Package.Name)
	return err == nil && strings.TrimSpace(string(out)) == a.Package.Name
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
	if _, _, markerErr := a.Exec.Run("test", "-e", "/var/run/reboot-required"); markerErr == nil {
		result.RebootRequired = executor.RebootRequired
		result.Activation = append(result.Activation, executor.ActivationSignal{Kind: executor.ActivationRebootRequired})
	}
	return result
}
