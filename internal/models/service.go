package models

import (
	"fmt"
	"regexp"
)

// ServiceProvider identifies the init-service backend selected for a portable
// service resource. Non-systemd providers remain capability-gated until their
// provider contract suites pass.
type ServiceProvider string

const (
	ServiceProviderSystemd ServiceProvider = "systemd"
	ServiceProviderOpenRC  ServiceProvider = "openrc"
	ServiceProviderSysV    ServiceProvider = "sysv"
)

type ServiceScope string

const (
	ServiceScopeSystem ServiceScope = "system"
	ServiceScopeUser   ServiceScope = "user"
)

// ServiceResource expresses steady service state independently from action
// requests such as restart or reload.
type ServiceResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string          `yaml:"name"`
	Provider     ServiceProvider `yaml:"provider"`
	Scope        ServiceScope    `yaml:"scope"`
	Service      string          `yaml:"service"`
	Users        string          `yaml:"users,omitempty"`
	Linger       bool            `yaml:"linger,omitempty"`
	Enabled      *bool           `yaml:"enabled,omitempty"`
	Active       *bool           `yaml:"active,omitempty"`
	Masked       *bool           `yaml:"masked,omitempty"`
}

var serviceIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]*$`)

func (r ServiceResource) Validate() error {
	if !endpointScheduleName.MatchString(r.Name) {
		return fmt.Errorf("service resource name %q is invalid", r.Name)
	}
	if r.Lifecycle != "" {
		return fmt.Errorf("service state does not support lifecycle %q", r.Lifecycle)
	}
	if !serviceIdentity.MatchString(r.Service) {
		return fmt.Errorf("service identity %q is invalid", r.Service)
	}
	switch r.Provider {
	case ServiceProviderSystemd:
	case ServiceProviderOpenRC, ServiceProviderSysV:
		if r.Masked != nil {
			return fmt.Errorf("service provider %s does not support masked state", r.Provider)
		}
		return fmt.Errorf("service provider %s is not advertised until its provider contract passes", r.Provider)
	default:
		return fmt.Errorf("service provider %q is invalid", r.Provider)
	}
	switch r.Scope {
	case ServiceScopeSystem:
		if r.Users != "" || r.Linger {
			return fmt.Errorf("system service must not declare users or linger")
		}
	case ServiceScopeUser:
		if r.Provider != ServiceProviderSystemd {
			return fmt.Errorf("user service scope is unsupported by provider %s", r.Provider)
		}
		if r.Users != "interactive" {
			return fmt.Errorf("user service requires users: interactive")
		}
	default:
		return fmt.Errorf("service scope %q is invalid", r.Scope)
	}
	if r.Enabled == nil && r.Active == nil && r.Masked == nil && !r.Linger {
		return fmt.Errorf("service must manage at least one of enabled, active, masked, or linger")
	}
	if r.Masked != nil && *r.Masked {
		if r.Enabled != nil && *r.Enabled {
			return fmt.Errorf("masked service cannot be enabled")
		}
		if r.Active != nil && *r.Active {
			return fmt.Errorf("masked service cannot be active")
		}
	}
	return nil
}
