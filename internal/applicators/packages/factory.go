package packages

import (
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/aur"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/dnf"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// SelectPackageApplicator returns a package applicator for the given distro.
func SelectPackageApplicator(distro types.Distro, pkg models.Package, exec executil.Runner) (executor.Handler, error) {
	pm := pkg.PM
	if pm == "" {
		switch distro {
		case types.Arch:
			pm = types.Pacman
		default:
			pm = types.Apt
		}
	}
	switch pm {
	case types.Apt:
		return apt.New(pkg, exec), nil
	case types.Pacman, types.Yay:
		return aur.New(pkg, exec), nil
	case types.Dnf:
		return dnf.New(pkg, exec), nil
	default:
		return nil, fmt.Errorf("unsupported package manager %q", pm)
	}
}
