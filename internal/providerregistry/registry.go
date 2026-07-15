// Package providerregistry maps normalized endpoint facts to provider identities.
package providerregistry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/types"
)

type Capability string

const (
	CapabilityPackage  Capability = "package"
	CapabilityInit     Capability = "init"
	CapabilityFirewall Capability = "firewall"
	CapabilityNetwork  Capability = "network"
	CapabilitySecurity Capability = "security"
	CapabilityDesktop  Capability = "desktop"
	CapabilityBrowser  Capability = "browser"
)

type Definition struct {
	ID         string
	Capability Capability
	Matches    func(facts.Facts) bool
}

type Registry struct {
	definitions []Definition
}

func New(definitions ...Definition) (*Registry, error) {
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.ID) == "" || definition.Capability == "" || definition.Matches == nil {
			return nil, fmt.Errorf("incomplete provider definition %#v", definition)
		}
		key := string(definition.Capability) + "\x00" + definition.ID
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate provider %q for capability %q", definition.ID, definition.Capability)
		}
		seen[key] = struct{}{}
	}
	return &Registry{definitions: append([]Definition(nil), definitions...)}, nil
}

func NewDefault() (*Registry, error) {
	return New(
		Definition{"apt", CapabilityPackage, func(f facts.Facts) bool { return f.Package == types.Apt }},
		Definition{"pacman", CapabilityPackage, func(f facts.Facts) bool { return f.Package == types.Pacman }},
		Definition{"systemd", CapabilityInit, func(f facts.Facts) bool { return f.Init == facts.InitSystemd }},
		Definition{"openrc", CapabilityInit, func(f facts.Facts) bool { return f.Init == facts.InitOpenRC }},
		Definition{"sysv", CapabilityInit, func(f facts.Facts) bool { return f.Init == facts.InitSysV }},
		Definition{"firewalld", CapabilityFirewall, func(f facts.Facts) bool { return f.Firewall == facts.FirewallFirewalld }},
		Definition{"nftables", CapabilityFirewall, func(f facts.Facts) bool { return f.Firewall == facts.FirewallNftables }},
		Definition{"network-manager", CapabilityNetwork, func(f facts.Facts) bool { return f.Network == facts.NetworkManager }},
		Definition{"systemd-networkd", CapabilityNetwork, func(f facts.Facts) bool { return f.Network == facts.NetworkSystemdNetwork }},
		Definition{"netplan", CapabilityNetwork, func(f facts.Facts) bool { return f.Network == facts.NetworkNetplan }},
		Definition{"apparmor", CapabilitySecurity, func(f facts.Facts) bool { return f.Security == facts.SecurityAppArmor }},
		Definition{"dconf", CapabilityDesktop, func(f facts.Facts) bool { return containsDesktop(f.Desktop, facts.DesktopDconf) }},
		Definition{"gsettings", CapabilityDesktop, func(f facts.Facts) bool { return containsDesktop(f.Desktop, facts.DesktopGSettings) }},
		Definition{"chromium", CapabilityBrowser, func(f facts.Facts) bool { return containsBrowser(f.Browser, facts.BrowserChromium) }},
		Definition{"google-chrome", CapabilityBrowser, func(f facts.Facts) bool { return containsBrowser(f.Browser, facts.BrowserGoogleChrome) }},
		Definition{"firefox", CapabilityBrowser, func(f facts.Facts) bool { return containsBrowser(f.Browser, facts.BrowserFirefox) }},
	)
}

func (r *Registry) Resolve(endpoint facts.Facts) map[Capability][]string {
	endpoint = endpoint.Normalized()
	resolved := make(map[Capability][]string)
	for _, definition := range r.definitions {
		if definition.Matches(endpoint) {
			resolved[definition.Capability] = append(resolved[definition.Capability], definition.ID)
		}
	}
	for capability := range resolved {
		sort.Strings(resolved[capability])
	}
	return resolved
}

func containsDesktop(backends []facts.DesktopBackend, want facts.DesktopBackend) bool {
	for _, backend := range backends {
		if backend == want {
			return true
		}
	}
	return false
}

func containsBrowser(backends []facts.BrowserBackend, want facts.BrowserBackend) bool {
	for _, backend := range backends {
		if backend == want {
			return true
		}
	}
	return false
}
