package models

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

const NetworkProviderNetworkManager = "network-manager"

var networkResourceName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var networkInterfaceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)
var networkDomain = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

// DNSResolverResource manages portable resolver intent independently from
// interface addressing and routes. Configured and Effective select the
// persistent-provider and active-runtime scopes respectively.
type DNSResolverResource struct {
	ResourceMeta  `yaml:",inline"`
	Name          string   `yaml:"name"`
	Provider      string   `yaml:"provider"`
	Interface     string   `yaml:"interface"`
	Servers       []string `yaml:"servers,omitempty"`
	SearchDomains []string `yaml:"searchDomains,omitempty"`
	Configured    bool     `yaml:"configured,omitempty"`
	Effective     bool     `yaml:"effective,omitempty"`
}

// RouteResource manages one portable route independently in provider
// configuration and the effective kernel route table.
type RouteResource struct {
	ResourceMeta `yaml:",inline"`
	Name         string `yaml:"name"`
	Provider     string `yaml:"provider"`
	Interface    string `yaml:"interface"`
	Destination  string `yaml:"destination"`
	Gateway      string `yaml:"gateway,omitempty"`
	Metric       int    `yaml:"metric,omitempty"`
	Table        int    `yaml:"table,omitempty"`
	Configured   bool   `yaml:"configured,omitempty"`
	Effective    bool   `yaml:"effective,omitempty"`
}

func (r DNSResolverResource) Validate() error {
	if err := validateNetworkIdentity("dnsResolver", r.Name, r.Provider, r.Interface, r.Lifecycle); err != nil {
		return err
	}
	if !r.Configured && !r.Effective {
		return fmt.Errorf("dnsResolver %q requires configured or effective scope", r.Name)
	}
	if r.Lifecycle == LifecycleAbsent {
		if len(r.Servers) != 0 || len(r.SearchDomains) != 0 {
			return fmt.Errorf("absent dnsResolver %q must omit servers and searchDomains", r.Name)
		}
		return nil
	}
	if len(r.Servers) == 0 && len(r.SearchDomains) == 0 {
		return fmt.Errorf("dnsResolver %q requires servers or searchDomains", r.Name)
	}
	if len(r.Servers) > 8 || len(r.SearchDomains) > 16 {
		return fmt.Errorf("dnsResolver %q exceeds resolver entry limits", r.Name)
	}
	seen := map[string]struct{}{}
	for _, server := range r.Servers {
		address, err := netip.ParseAddr(server)
		if err != nil || address.String() != server {
			return fmt.Errorf("dnsResolver %q server %q is not a canonical IP address", r.Name, server)
		}
		if _, exists := seen[server]; exists {
			return fmt.Errorf("dnsResolver %q duplicates server %q", r.Name, server)
		}
		seen[server] = struct{}{}
	}
	clear(seen)
	for _, domain := range r.SearchDomains {
		if strings.TrimSpace(domain) != domain || !networkDomain.MatchString(domain) || strings.Contains(domain, "..") {
			return fmt.Errorf("dnsResolver %q search domain %q is invalid", r.Name, domain)
		}
		for _, label := range strings.Split(domain, ".") {
			if len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return fmt.Errorf("dnsResolver %q search domain %q contains an invalid label", r.Name, domain)
			}
		}
		key := strings.ToLower(domain)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("dnsResolver %q duplicates search domain %q", r.Name, domain)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (r RouteResource) Validate() error {
	if err := validateNetworkIdentity("route", r.Name, r.Provider, r.Interface, r.Lifecycle); err != nil {
		return err
	}
	if !r.Configured && !r.Effective {
		return fmt.Errorf("route %q requires configured or effective scope", r.Name)
	}
	prefix, err := netip.ParsePrefix(r.Destination)
	if err != nil || prefix.Masked().String() != r.Destination {
		return fmt.Errorf("route %q destination %q is not a canonical network prefix", r.Name, r.Destination)
	}
	if r.Gateway != "" {
		gateway, err := netip.ParseAddr(r.Gateway)
		if err != nil || gateway.String() != r.Gateway || gateway.Is4() != prefix.Addr().Is4() {
			return fmt.Errorf("route %q gateway %q is invalid for destination %q", r.Name, r.Gateway, r.Destination)
		}
	}
	if r.Metric < 0 || uint64(r.Metric) > uint64(^uint32(0)) || r.Table < 0 || uint64(r.Table) > uint64(^uint32(0)) {
		return fmt.Errorf("route %q metric and table must fit unsigned 32-bit values", r.Name)
	}
	return nil
}

func validateNetworkIdentity(kind, name, provider, interfaceName string, lifecycle Lifecycle) error {
	if !networkResourceName.MatchString(name) {
		return fmt.Errorf("%s resource name %q is invalid", kind, name)
	}
	if provider != NetworkProviderNetworkManager {
		return fmt.Errorf("%s provider %q is not advertised", kind, provider)
	}
	if !networkInterfaceName.MatchString(interfaceName) {
		return fmt.Errorf("%s %q interface %q is invalid", kind, name, interfaceName)
	}
	if lifecycle != "" && lifecycle != LifecyclePresent && lifecycle != LifecycleAbsent {
		return fmt.Errorf("%s %q lifecycle must be present or absent", kind, name)
	}
	return nil
}
