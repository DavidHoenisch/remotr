package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type SessionProxyMode string

const (
	SessionProxyNone      SessionProxyMode = "none"
	SessionProxyManual    SessionProxyMode = "manual"
	SessionProxyAutomatic SessionProxyMode = "automatic"
)

type SessionProxyPolicy struct {
	Mode         SessionProxyMode `yaml:"mode"`
	HTTPHost     string           `yaml:"httpHost,omitempty"`
	HTTPPort     uint32           `yaml:"httpPort,omitempty"`
	HTTPSHost    string           `yaml:"httpsHost,omitempty"`
	HTTPSPort    uint32           `yaml:"httpsPort,omitempty"`
	AutomaticURL string           `yaml:"automaticUrl,omitempty"`
	IgnoreHosts  []string         `yaml:"ignoreHosts,omitempty"`
}

func (p SessionProxyPolicy) Validate() error {
	switch p.Mode {
	case SessionProxyNone:
		if p.HTTPHost != "" || p.HTTPPort != 0 || p.HTTPSHost != "" || p.HTTPSPort != 0 || p.AutomaticURL != "" || len(p.IgnoreHosts) != 0 {
			return fmt.Errorf("none proxy mode must omit proxy endpoints")
		}
	case SessionProxyManual:
		if strings.TrimSpace(p.HTTPHost) == "" && strings.TrimSpace(p.HTTPSHost) == "" {
			return fmt.Errorf("manual proxy mode requires an HTTP or HTTPS host")
		}
		if (p.HTTPHost == "") != (p.HTTPPort == 0) || (p.HTTPSHost == "") != (p.HTTPSPort == 0) {
			return fmt.Errorf("manual proxy host and port must be declared together")
		}
		if p.HTTPPort > 65535 || p.HTTPSPort > 65535 {
			return fmt.Errorf("proxy ports must be from 1 through 65535")
		}
		if p.AutomaticURL != "" {
			return fmt.Errorf("manual proxy mode must omit automaticUrl")
		}
	case SessionProxyAutomatic:
		parsed, err := url.ParseRequestURI(p.AutomaticURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("automatic proxy mode requires an absolute automaticUrl")
		}
		if p.HTTPHost != "" || p.HTTPPort != 0 || p.HTTPSHost != "" || p.HTTPSPort != 0 {
			return fmt.Errorf("automatic proxy mode must omit manual endpoints")
		}
	default:
		return fmt.Errorf("session proxy mode %q is invalid", p.Mode)
	}
	for _, host := range p.IgnoreHosts {
		if strings.TrimSpace(host) != host || host == "" || strings.ContainsAny(host, "\x00\r\n") {
			return fmt.Errorf("proxy ignore host %q is invalid", host)
		}
	}
	return nil
}

var (
	sessionPolicyName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,126}$`)
	mimeTypeName      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]*/[A-Za-z0-9][A-Za-z0-9.+_-]*$`)
	desktopFileName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.desktop$`)
)

// SessionPolicyResource exposes GNOME-compatible lock, idle, proxy, lockdown,
// and default-application state as explicit managed fields.
type SessionPolicyResource struct {
	ResourceMeta         `yaml:",inline"`
	Name                 string                  `yaml:"name"`
	Provider             DesktopSettingProvider  `yaml:"provider"`
	Selector             InteractiveUserSelector `yaml:"selector"`
	LockEnabled          *bool                   `yaml:"lockEnabled,omitempty"`
	IdleTimeoutSeconds   *uint32                 `yaml:"idleTimeoutSeconds,omitempty"`
	LockDelaySeconds     *uint32                 `yaml:"lockDelaySeconds,omitempty"`
	Proxy                *SessionProxyPolicy     `yaml:"proxy,omitempty"`
	DisableUserSwitching *bool                   `yaml:"disableUserSwitching,omitempty"`
	DisableLogout        *bool                   `yaml:"disableLogout,omitempty"`
	DisableCommandLine   *bool                   `yaml:"disableCommandLine,omitempty"`
	DefaultApplications  map[string]string       `yaml:"defaultApplications,omitempty"`
}

func (r SessionPolicyResource) Validate() error {
	if !sessionPolicyName.MatchString(r.Name) {
		return fmt.Errorf("session policy name must be a safe identifier")
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent {
		return fmt.Errorf("session policy supports only present lifecycle")
	}
	if r.Provider != DesktopSettingProviderDconf && r.Provider != DesktopSettingProviderGSettings {
		return fmt.Errorf("session policy provider %q is invalid", r.Provider)
	}
	if err := r.Selector.Validate(); err != nil {
		return fmt.Errorf("session policy selector: %w", err)
	}
	if r.LockEnabled == nil && r.IdleTimeoutSeconds == nil && r.LockDelaySeconds == nil && r.Proxy == nil &&
		r.DisableUserSwitching == nil && r.DisableLogout == nil && r.DisableCommandLine == nil && len(r.DefaultApplications) == 0 {
		return fmt.Errorf("session policy requires at least one managed field")
	}
	if r.Proxy != nil {
		if err := r.Proxy.Validate(); err != nil {
			return err
		}
	}
	for mime, desktopFile := range r.DefaultApplications {
		if !mimeTypeName.MatchString(mime) {
			return fmt.Errorf("default application MIME type %q is invalid", mime)
		}
		if !desktopFileName.MatchString(desktopFile) {
			return fmt.Errorf("default application desktop file %q is invalid", desktopFile)
		}
	}
	return nil
}
