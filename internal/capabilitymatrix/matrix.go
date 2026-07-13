// Package capabilitymatrix validates provider requirements against declared
// targets at author time and normalized local facts at runtime.
package capabilitymatrix

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// ValidateStatic rejects provider/target combinations that cannot be true for
// any endpoint described by a canonical configuration.
func ValidateStatic(schemaVersion int, configuration models.Configuration, value any) error {
	if schemaVersion != 1 {
		return nil
	}
	switch resource := value.(type) {
	case *models.Package:
		if resource.PM == types.Dnf {
			return fmt.Errorf("DNF provider is deferred to the RPM-family roadmap")
		}
		for _, target := range configuration.TargetDistros {
			if !packageProviderSupportsDistro(resource.PM, target) {
				return fmt.Errorf("provider %q is incompatible with target distro %q", resource.PM, target)
			}
		}
		for provider := range resource.ProviderOptions {
			if !knownPackageProvider(provider) {
				return fmt.Errorf("providerOptions selects unknown package provider %q", provider)
			}
			if resource.PM != "" && provider != string(resource.PM) {
				return fmt.Errorf("providerOptions for %q cannot be used with selected provider %q", provider, resource.PM)
			}
		}
	case *models.APTSigningKey, *models.APTRepository:
		for _, target := range configuration.TargetDistros {
			if target != types.Debian && target != types.Ubuntu {
				return fmt.Errorf("APT signing-key provider is incompatible with target distro %q", target)
			}
		}
	}
	return nil
}

// Requirements returns stable capability identifiers needed by a resource.
func Requirements(kind models.ResourceKind, value any) []string {
	requirements := []string{"resource:" + string(kind), "schema:1"}
	switch resource := value.(type) {
	case *models.Package:
		if resource.PM != "" {
			requirements = append(requirements, "provider:package/"+string(resource.PM))
		}
	case *models.FirewallResource:
		if backend := strings.ToLower(strings.TrimSpace(resource.Backend)); backend != "" {
			requirements = append(requirements, "provider:firewall/"+backend)
		}
	case *models.SystemdResource, *models.SystemdUserResource:
		requirements = append(requirements, "provider:init/systemd")
	case *models.APTSigningKey, *models.APTRepository:
		requirements = append(requirements, "provider:repository/apt")
	case *models.SysctlResource:
		requirements = append(requirements, "provider:kernel/sysctl")
	case *models.KernelModuleResource:
		requirements = append(requirements, "provider:kernel/modules")
	case *models.HostnameResource:
		requirements = append(requirements, "provider:host/hostnamectl")
	case *models.HostLocaleResource:
		requirements = append(requirements, "provider:host/localectl")
	case *models.TimeSyncResource:
		requirements = append(requirements, "provider:time-sync/"+resource.Provider)
	case *models.MountResource:
		requirements = append(requirements, "provider:storage/mount")
	}
	sort.Strings(requirements)
	return requirements
}

func packageProviderSupportsDistro(provider types.PackageManager, distro types.Distro) bool {
	switch provider {
	case "", types.Flatpak, types.Pwa, types.Remotr:
		return true
	case types.Apt:
		return distro == types.Debian || distro == types.Ubuntu
	case types.Pacman, types.Yay:
		return distro == types.Arch
	default:
		return false
	}
}

func knownPackageProvider(provider string) bool {
	switch types.PackageManager(provider) {
	case types.Apt, types.Pacman, types.Yay, types.Flatpak, types.Pwa, types.Remotr:
		return true
	default:
		return false
	}
}

// UnsupportedError reports a local provider fact mismatch without classifying
// it as ordinary drift.
type UnsupportedError struct {
	Capability string
	Required   string
	Observed   string
}

func (e UnsupportedError) Error() string {
	return fmt.Sprintf("required %s provider %q is unavailable (observed %q)", e.Capability, e.Required, e.Observed)
}

// CheckRuntime returns UnsupportedError when local normalized facts cannot
// satisfy a resource's provider contract.
func CheckRuntime(value any, endpoint facts.Facts) error {
	endpoint = endpoint.Normalized()
	switch resource := value.(type) {
	case *models.Package:
		required := resource.PM
		switch required {
		case "", types.Flatpak, types.Pwa, types.Remotr:
			return nil
		case types.Dnf:
			return UnsupportedError{Capability: "package", Required: string(required), Observed: string(endpoint.Package)}
		case types.Yay:
			if endpoint.Package == types.Pacman {
				return nil
			}
		default:
			if endpoint.Package == required {
				return nil
			}
		}
		return UnsupportedError{Capability: "package", Required: string(required), Observed: string(endpoint.Package)}
	case *models.FirewallResource:
		required := strings.ToLower(strings.TrimSpace(resource.Backend))
		if required != "" && required != string(endpoint.Firewall) {
			return UnsupportedError{Capability: "firewall", Required: required, Observed: string(endpoint.Firewall)}
		}
	case *models.SystemdResource, *models.SystemdUserResource:
		if endpoint.Init != "" && endpoint.Init != facts.InitSystemd {
			return UnsupportedError{Capability: "init", Required: string(facts.InitSystemd), Observed: string(endpoint.Init)}
		}
	case *models.APTSigningKey, *models.APTRepository:
		if endpoint.Package != types.Apt {
			return UnsupportedError{Capability: "repository", Required: "apt", Observed: string(endpoint.Package)}
		}
	}
	return nil
}
