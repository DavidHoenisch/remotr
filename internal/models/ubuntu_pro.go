package models

import (
	"fmt"
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

type UbuntuProLandscape struct {
	State              string   `yaml:"state"`
	AccountName        string   `yaml:"accountName,omitempty"`
	ComputerTitle      string   `yaml:"computerTitle,omitempty"`
	ServerURL          string   `yaml:"serverURL,omitempty"`
	PingURL            string   `yaml:"pingURL,omitempty"`
	Tags               []string `yaml:"tags,omitempty"`
	AccessGroup        string   `yaml:"accessGroup,omitempty"`
	RegistrationKeyRef string   `yaml:"registrationKeyRef,omitempty"`
	CARef              string   `yaml:"caRef,omitempty"`
}

// UbuntuProResource manages subscription attachment and only the explicitly
// listed service contracts. TokenRef remains a provider-neutral reference.
type UbuntuProResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string              `yaml:"name"`
	TokenRef     string              `yaml:"tokenRef,omitempty"`
	Services     []UbuntuProService  `yaml:"services,omitempty"`
	Landscape    *UbuntuProLandscape `yaml:"landscape,omitempty"`
}

func (resource UbuntuProResource) Validate() error {
	if resource.Lifecycle != UbuntuProAttached && resource.Lifecycle != UbuntuProDetached {
		return fmt.Errorf("ubuntuPro lifecycle must be attached or detached")
	}
	if resource.Lifecycle == UbuntuProAttached {
		if _, err := secretref.ParseSelected(resource.TokenRef); err != nil {
			return fmt.Errorf("ubuntuPro tokenRef: %w", err)
		}
	} else if resource.TokenRef != "" || len(resource.Services) != 0 || resource.Landscape != nil {
		return fmt.Errorf("detached ubuntuPro resource forbids tokenRef, services, and landscape")
	}
	if len(resource.ProviderOptions) != 0 || len(resource.Validation) != 0 || len(resource.PreApplyValidation) != 0 {
		return fmt.Errorf("ubuntuPro does not accept provider options or raw validation commands")
	}
	seen := make(map[string]bool, len(resource.Services))
	for index, service := range resource.Services {
		if service.Name != strings.TrimSpace(service.Name) || (service.Name != "esm-infra" && service.Name != "livepatch") {
			return fmt.Errorf("ubuntuPro services[%d].name %q is not in the service catalog", index, service.Name)
		}
		if seen[service.Name] {
			return fmt.Errorf("ubuntuPro services contains duplicate service %q", service.Name)
		}
		seen[service.Name] = true
		if service.State != UbuntuProServiceEnabled && service.State != UbuntuProServiceDisabled {
			return fmt.Errorf("ubuntuPro service %q state must be enabled or disabled", service.Name)
		}
		if service.EnableMode != "" && service.EnableMode != UbuntuProEnableFull && service.EnableMode != UbuntuProEnableAccessOnly {
			return fmt.Errorf("ubuntuPro service %q has invalid enableMode %q", service.Name, service.EnableMode)
		}
		if service.DisableMode != "" && service.DisableMode != UbuntuProRetainPackages && service.DisableMode != UbuntuProPurgePackages {
			return fmt.Errorf("ubuntuPro service %q has invalid disableMode %q", service.Name, service.DisableMode)
		}
	}
	return nil
}
