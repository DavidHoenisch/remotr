package models

import (
	"fmt"
	"net/url"
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

type UbuntuProLandscape struct {
	State              UbuntuProLandscapeState `yaml:"state"`
	AccountName        string                  `yaml:"accountName,omitempty"`
	ComputerTitle      string                  `yaml:"computerTitle,omitempty"`
	ServerURL          string                  `yaml:"serverURL,omitempty"`
	PingURL            string                  `yaml:"pingURL,omitempty"`
	Tags               []string                `yaml:"tags,omitempty"`
	AccessGroup        string                  `yaml:"accessGroup,omitempty"`
	RegistrationKeyRef string                  `yaml:"registrationKeyRef,omitempty"`
	CARef              string                  `yaml:"caRef,omitempty"`
}

type UbuntuProLandscapeState string

const (
	UbuntuProLandscapeEnrolled   UbuntuProLandscapeState = "enrolled"
	UbuntuProLandscapeUnenrolled UbuntuProLandscapeState = "unenrolled"
)

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
	if len(resource.Services) > len(ubuntuProServiceCatalog) {
		return fmt.Errorf("ubuntuPro services exceeds %d entries", len(ubuntuProServiceCatalog))
	}
	seen := make(map[string]bool, len(resource.Services))
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
	if resource.Landscape != nil {
		if err := resource.Landscape.Validate(); err != nil {
			return fmt.Errorf("ubuntuPro landscape: %w", err)
		}
	}
	return nil
}

func (landscape UbuntuProLandscape) Validate() error {
	for name, value := range map[string]string{
		"accountName": landscape.AccountName, "computerTitle": landscape.ComputerTitle, "accessGroup": landscape.AccessGroup,
	} {
		if len(value) > 256 {
			return fmt.Errorf("%s exceeds 256 bytes", name)
		}
	}
	if len(landscape.ServerURL) > 2048 || len(landscape.PingURL) > 2048 {
		return fmt.Errorf("Landscape URL exceeds 2048 bytes")
	}
	if len(landscape.Tags) > 32 {
		return fmt.Errorf("tags exceeds 32 entries")
	}
	seenTags := make(map[string]bool, len(landscape.Tags))
	for _, tag := range landscape.Tags {
		if tag == "" || len(tag) > 128 || strings.TrimSpace(tag) != tag {
			return fmt.Errorf("tag must be non-empty, trimmed, and at most 128 bytes")
		}
		if seenTags[tag] {
			return fmt.Errorf("duplicate Landscape tag %q", tag)
		}
		seenTags[tag] = true
	}
	if landscape.State != UbuntuProLandscapeEnrolled && landscape.State != UbuntuProLandscapeUnenrolled {
		return fmt.Errorf("state must be enrolled or unenrolled")
	}
	if landscape.State == UbuntuProLandscapeUnenrolled {
		if landscape.AccountName != "" || landscape.ServerURL != "" || landscape.PingURL != "" || landscape.RegistrationKeyRef != "" || landscape.CARef != "" {
			return fmt.Errorf("unenrolled state forbids enrollment fields")
		}
		return nil
	}
	if strings.TrimSpace(landscape.AccountName) == "" || strings.TrimSpace(landscape.ComputerTitle) == "" {
		return fmt.Errorf("enrolled state requires accountName and computerTitle")
	}
	if (landscape.ServerURL == "") != (landscape.PingURL == "") {
		return fmt.Errorf("serverURL and pingURL must be supplied together")
	}
	for name, raw := range map[string]string{"serverURL": landscape.ServerURL, "pingURL": landscape.PingURL} {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("%s must be an absolute HTTPS URL", name)
		}
	}
	for name, reference := range map[string]string{"registrationKeyRef": landscape.RegistrationKeyRef, "caRef": landscape.CARef} {
		if reference == "" {
			continue
		}
		if _, err := secretref.ParseSelected(reference); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}
