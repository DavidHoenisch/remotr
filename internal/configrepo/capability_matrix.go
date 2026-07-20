package configrepo

import (
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// ValidateProviderRelease proves that every distro-specific package,
// repository, and signing-trust resource has a matching passing evidence row
// for each exact authored target. Callers use this after ordinary state
// validation and before storing a release artifact.
func ValidateProviderRelease(state models.State, matrix providermatrix.Matrix) error {
	if err := providermatrix.Validate(matrix); err != nil {
		return fmt.Errorf("provider release matrix: %w", err)
	}
	for _, configuration := range state.Configurations {
		requiresQualification := len(configuration.Packages) > 0 || len(configuration.APTSigningKeys) > 0 || len(configuration.APTRepositories) > 0 || len(configuration.PacmanSigningKeys) > 0 || len(configuration.PacmanRepositories) > 0
		if !requiresQualification {
			continue
		}
		if len(configuration.TargetDistros) == 0 {
			return fmt.Errorf("configuration %q: exact targetDistros are required for provider release qualification", configuration.Name)
		}
		if len(configuration.TargetArch) == 0 {
			return fmt.Errorf("configuration %q: exact targetArch is required for provider release qualification", configuration.Name)
		}
		for _, distro := range configuration.TargetDistros {
			for _, architecture := range configuration.TargetArch {
				for _, pkg := range configuration.Packages {
					backend := pkg.PM
					if backend == "" {
						backend = factsPackageManager(distro)
					}
					if backend == types.Flatpak || backend == types.Pwa || backend == types.Remotr {
						continue
					}
					if err := requireProviderRelease(matrix, "package", distro, architecture, string(backend)); err != nil {
						return fmt.Errorf("resource %q: %w", models.ResourceAddress(configuration.Name, pkg.Name), err)
					}
				}
				if len(configuration.APTSigningKeys) > 0 || len(configuration.APTRepositories) > 0 {
					if err := requireProviderRelease(matrix, "repository", distro, architecture, "apt"); err != nil {
						return fmt.Errorf("configuration %q repository/apt: %w", configuration.Name, err)
					}
				}
				if len(configuration.PacmanSigningKeys) > 0 || len(configuration.PacmanRepositories) > 0 {
					if err := requireProviderRelease(matrix, "repository", distro, architecture, "pacman"); err != nil {
						return fmt.Errorf("configuration %q repository/pacman: %w", configuration.Name, err)
					}
				}
			}
		}
	}
	return nil
}

func requireProviderRelease(matrix providermatrix.Matrix, provider string, distro types.Distro, architecture types.Architecture, backend string) error {
	distribution, release := qualifiedDistroRelease(distro)
	qualifiedArchitecture := ""
	if architecture == types.X86 {
		qualifiedArchitecture = "amd64"
	}
	claim := providermatrix.Claim{
		Provider: provider, Distribution: distribution, Release: release,
		Architecture: qualifiedArchitecture, Backend: backend,
		ContractRevision: "v1", Environment: "container",
	}
	if distribution == "" || release == "" || qualifiedArchitecture == "" || !providermatrix.Advertised(matrix, claim) {
		return fmt.Errorf("missing passing provider evidence for %s/%s on %s %s %s", provider, backend, strings.ToLower(string(distro)), release, qualifiedArchitecture)
	}
	return nil
}

func qualifiedDistroRelease(distro types.Distro) (string, string) {
	switch distro {
	case types.Debian:
		return "debian", "12"
	case types.Ubuntu:
		return "ubuntu", "24.04"
	case types.Arch:
		return "arch", "2026-07-06"
	default:
		return "", ""
	}
}

func factsPackageManager(distro types.Distro) types.PackageManager {
	if distro == types.Arch {
		return types.Pacman
	}
	return types.Apt
}

func validateCapabilityMatrix(state models.State) error {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		return err
	}
	for i := range state.Configurations {
		configuration := &state.Configurations[i]
		resources, err := registry.Resources(configuration)
		if err != nil {
			return err
		}
		for _, resource := range resources {
			if err := capabilitymatrix.ValidateStatic(state.SchemaVersion, *configuration, resource.Value()); err != nil {
				return fmt.Errorf("resource %q: %w", models.ResourceAddress(configuration.Name, resource.Name()), err)
			}
		}
	}
	return nil
}
