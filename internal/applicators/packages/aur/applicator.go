package aur

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// Applicator manages packages via pacman (Arch).
type Applicator struct {
	Package models.Package
	Exec    executil.Runner
}

func New(pkg models.Package, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Package: pkg, Exec: exec}
}

func (a *Applicator) Name() string { return "pacman:" + a.Package.Name }

func (a *Applicator) Description() string { return "pacman package " + a.Package.Name }

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

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Package.Present {
		if a.Package.RefreshCache {
			if _, stderr, err := a.Exec.Run("pacman", "-Sy", "--noconfirm"); err != nil {
				return fmt.Errorf("pacman cache refresh failed: %s: %w", bounded(stderr), err)
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
		}
		_, _, err := a.Exec.Run("pacman", "-S", "--noconfirm", a.Package.Name)
		return err
	}
	remove := "-R"
	if a.Package.RemoveDependencies {
		remove = "-Rs"
	}
	_, _, err := a.Exec.Run("pacman", remove, "--noconfirm", a.Package.Name)
	return err
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
