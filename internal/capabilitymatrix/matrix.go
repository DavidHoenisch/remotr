// Package capabilitymatrix validates provider requirements against declared
// targets at author time and normalized local facts at runtime.
package capabilitymatrix

import (
	"fmt"
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
	}
	return nil
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
	}
	return nil
}
