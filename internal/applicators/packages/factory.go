package packages

import (
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/customapps"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/aur"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/flatpak"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pwa"
	"github.com/DavidHoenisch/remotr/internal/apppackages"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// SelectPackageApplicator returns a package applicator for the given distro.
func SelectPackageApplicator(distro types.Distro, pkg models.Package, f facts.Facts, exec executil.Runner, urls apppackages.URLResolver) (executor.Handler, error) {
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
	case types.Pacman:
		return aur.New(pkg, exec), nil
	case types.Yay:
		return nil, fmt.Errorf("unsupported package manager %q: no truthful AUR provider is advertised", pm)
	case types.Dnf:
		return nil, fmt.Errorf("unsupported package manager %q: deferred to a future RPM-family OpenSpec change", pm)
	case types.Flatpak:
		return flatpak.New(pkg, exec), nil
	case types.Pwa:
		return pwa.New(pkg, exec), nil
	case types.Remotr:
		return customapps.New(pkg, f, exec, urls), nil
	default:
		return nil, fmt.Errorf("unsupported package manager %q", pm)
	}
}
