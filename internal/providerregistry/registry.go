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
	ID               string
	Capability       Capability
	ContractRevision string
	Matches          func(facts.Facts) bool
}

type Registry struct {
	definitions []Definition
}

func New(definitions ...Definition) (*Registry, error) {
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.ID) == "" || definition.Capability == "" || strings.TrimSpace(definition.ContractRevision) == "" || definition.Matches == nil {
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
		Definition{ID: "apt", Capability: CapabilityPackage, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Package == types.Apt }},
		Definition{ID: "pacman", Capability: CapabilityPackage, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Package == types.Pacman }},
		Definition{ID: "systemd", Capability: CapabilityInit, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Init == facts.InitSystemd }},
		Definition{ID: "openrc", Capability: CapabilityInit, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Init == facts.InitOpenRC }},
		Definition{ID: "sysv", Capability: CapabilityInit, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Init == facts.InitSysV }},
		Definition{ID: "firewalld", Capability: CapabilityFirewall, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Firewall == facts.FirewallFirewalld }},
		Definition{ID: "nftables", Capability: CapabilityFirewall, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Firewall == facts.FirewallNftables }},
		Definition{ID: "network-manager", Capability: CapabilityNetwork, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Network == facts.NetworkManager }},
		Definition{ID: "systemd-networkd", Capability: CapabilityNetwork, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Network == facts.NetworkSystemdNetwork }},
		Definition{ID: "netplan", Capability: CapabilityNetwork, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Network == facts.NetworkNetplan }},
		Definition{ID: "apparmor", Capability: CapabilitySecurity, ContractRevision: "1", Matches: func(f facts.Facts) bool { return f.Security == facts.SecurityAppArmor }},
		Definition{ID: "dconf", Capability: CapabilityDesktop, ContractRevision: "1", Matches: func(f facts.Facts) bool { return containsDesktop(f.Desktop, facts.DesktopDconf) }},
		Definition{ID: "gsettings", Capability: CapabilityDesktop, ContractRevision: "1", Matches: func(f facts.Facts) bool { return containsDesktop(f.Desktop, facts.DesktopGSettings) }},
		Definition{ID: "chromium", Capability: CapabilityBrowser, ContractRevision: "1", Matches: func(f facts.Facts) bool { return containsBrowser(f.Browser, facts.BrowserChromium) }},
		Definition{ID: "google-chrome", Capability: CapabilityBrowser, ContractRevision: "1", Matches: func(f facts.Facts) bool { return containsBrowser(f.Browser, facts.BrowserGoogleChrome) }},
		Definition{ID: "firefox", Capability: CapabilityBrowser, ContractRevision: "1", Matches: func(f facts.Facts) bool { return containsBrowser(f.Browser, facts.BrowserFirefox) }},
	)
}

// Definitions returns the registered provider contracts supported by the
// endpoint's current normalized facts.
func (r *Registry) Definitions(endpoint facts.Facts) []Definition {
	endpoint = endpoint.Normalized()
	definitions := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		if definition.Matches(endpoint) {
			definitions = append(definitions, definition)
		}
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Capability == definitions[j].Capability {
			return definitions[i].ID < definitions[j].ID
		}
		return definitions[i].Capability < definitions[j].Capability
	})
	return definitions
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
