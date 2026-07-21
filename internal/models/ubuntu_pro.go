package models

import (
	"fmt"
	"slices"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/secretref"
)

const (
	UbuntuProAttached Lifecycle = "attached"
	UbuntuProDetached Lifecycle = "detached"
)

type UbuntuProServiceState string

const (
	UbuntuProServiceEnabled  UbuntuProServiceState = "enabled"
	UbuntuProServiceDisabled UbuntuProServiceState = "disabled"
)

type UbuntuProEnableMode string

const (
	UbuntuProEnableFull       UbuntuProEnableMode = "full"
	UbuntuProEnableAccessOnly UbuntuProEnableMode = "access-only"
)

type UbuntuProDisableMode string

const (
	UbuntuProRetainPackages UbuntuProDisableMode = "retain-packages"
	UbuntuProPurgePackages  UbuntuProDisableMode = "purge"
)

type UbuntuProService struct {
	Name        string                `yaml:"name"`
	State       UbuntuProServiceState `yaml:"state"`
	EnableMode  UbuntuProEnableMode   `yaml:"enableMode,omitempty"`
	Variant     string                `yaml:"variant,omitempty"`
	DisableMode UbuntuProDisableMode  `yaml:"disableMode,omitempty"`
}

// UbuntuProResource manages subscription attachment and only the explicitly
// listed service contracts. TokenRef remains a provider-neutral reference.
type UbuntuProResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string             `yaml:"name"`
	TokenRef     string             `yaml:"tokenRef,omitempty"`
	Services     []UbuntuProService `yaml:"services,omitempty"`
}

func (resource UbuntuProResource) Validate() error {
	if resource.Lifecycle != UbuntuProAttached && resource.Lifecycle != UbuntuProDetached {
		return fmt.Errorf("ubuntuPro lifecycle must be attached or detached")
	}
	if resource.Lifecycle == UbuntuProAttached {
		if _, err := secretref.ParseSelected(resource.TokenRef); err != nil {
			return fmt.Errorf("ubuntuPro tokenRef: %w", err)
		}
	} else if resource.TokenRef != "" || len(resource.Services) != 0 {
		return fmt.Errorf("detached ubuntuPro resource forbids tokenRef and services")
	}
	if len(resource.ProviderOptions) != 0 || len(resource.Validation) != 0 || len(resource.PreApplyValidation) != 0 {
		return fmt.Errorf("ubuntuPro does not accept provider options or raw validation commands")
	}
	if len(resource.Services) > len(ubuntuProServiceCatalog) {
		return fmt.Errorf("ubuntuPro services exceeds %d entries", len(ubuntuProServiceCatalog))
	}
	seen := make(map[string]bool, len(resource.Services))
	enabledServices := make(map[string]bool, len(resource.Services))
	for index, service := range resource.Services {
		contract, cataloged := ubuntuProServiceContract(service.Name)
		if service.Name != strings.TrimSpace(service.Name) || !cataloged {
			if reason, historical := ubuntuProHistoricalServices[service.Name]; historical {
				return fmt.Errorf("ubuntuPro services[%d].name %q: %s", index, service.Name, reason)
			}
			return fmt.Errorf("ubuntuPro services[%d].name %q is not in the stable service catalog", index, service.Name)
		}
		if seen[service.Name] {
			return fmt.Errorf("ubuntuPro services contains duplicate service %q", service.Name)
		}
		seen[service.Name] = true
		if service.State != UbuntuProServiceEnabled && service.State != UbuntuProServiceDisabled {
			return fmt.Errorf("ubuntuPro service %q state must be enabled or disabled", service.Name)
		}
		if service.State == UbuntuProServiceEnabled {
			for _, incompatible := range contract.IncompatibleWith {
				if enabledServices[incompatible] {
					return fmt.Errorf("ubuntuPro enabled services %q and %q are incompatible", incompatible, service.Name)
				}
			}
			enabledServices[service.Name] = true
		}
		if service.EnableMode != "" && !slices.Contains(contract.EnableModes, service.EnableMode) {
			return fmt.Errorf("ubuntuPro service %q does not support enableMode %q", service.Name, service.EnableMode)
		}
		if service.EnableMode != "" && service.State != UbuntuProServiceEnabled {
			return fmt.Errorf("ubuntuPro service %q enableMode requires state enabled", service.Name)
		}
		if service.Variant != "" && !slices.Contains(contract.Variants, service.Variant) {
			return fmt.Errorf("ubuntuPro service %q does not support variant %q", service.Name, service.Variant)
		}
		if service.DisableMode != "" && !slices.Contains(contract.DisableModes, service.DisableMode) {
			return fmt.Errorf("ubuntuPro service %q does not support disableMode %q", service.Name, service.DisableMode)
		}
		if service.DisableMode != "" && service.State != UbuntuProServiceDisabled {
			return fmt.Errorf("ubuntuPro service %q disableMode requires state disabled", service.Name)
		}
	}
	return nil
}
