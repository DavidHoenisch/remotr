package models

import (
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

const NetworkProviderNetworkManager = "network-manager"

const (
	NetworkProfileEthernet = "ethernet"
	NetworkProfileWiFi     = "wifi"
)

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

// NetworkInterfaceSelector identifies exactly one interface without relying
// on enumeration order. Every populated field must match the same device.
type NetworkInterfaceSelector struct {
	Name         string `yaml:"name,omitempty"`
	PermanentMAC string `yaml:"permanentMAC,omitempty"`
	Type         string `yaml:"type,omitempty"`
}

// NetworkProfileResource declares portable profile intent. Profiles are
// audit-first; guarded enforcement requires a provider rollback timeout.
type NetworkProfileResource struct {
	ResourceMeta    `yaml:",inline"`
	Name            string                   `yaml:"name"`
	Provider        string                   `yaml:"provider"`
	Selector        NetworkInterfaceSelector `yaml:"selector"`
	ProfileName     string                   `yaml:"profileName"`
	ProfileType     string                   `yaml:"profileType"`
	AutoConnect     *bool                    `yaml:"autoConnect,omitempty"`
	MTU             int                      `yaml:"mtu,omitempty"`
	IPv4Method      string                   `yaml:"ipv4Method,omitempty"`
	IPv6Method      string                   `yaml:"ipv6Method,omitempty"`
	Addresses       []string                 `yaml:"addresses,omitempty"`
	SSID            string                   `yaml:"ssid,omitempty"`
	CredentialRef   string                   `yaml:"credentialRef,omitempty"`
	Audit           *bool                    `yaml:"audit,omitempty"`
	RollbackTimeout string                   `yaml:"rollbackTimeout,omitempty"`
}

func (r NetworkProfileResource) IsAudit() bool {
	return r.Audit == nil || *r.Audit
}

func (r NetworkProfileResource) Validate() error {
	if !networkResourceName.MatchString(r.Name) {
		return fmt.Errorf("networkProfile resource name %q is invalid", r.Name)
	}
	if r.Provider != NetworkProviderNetworkManager {
		return fmt.Errorf("networkProfile provider %q is not advertised", r.Provider)
	}
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("networkProfile %q lifecycle must be present or absent", r.Name)
	}
	if strings.TrimSpace(r.ProfileName) == "" || strings.TrimSpace(r.ProfileName) != r.ProfileName || strings.ContainsAny(r.ProfileName, "\r\n") {
		return fmt.Errorf("networkProfile %q profileName is invalid", r.Name)
	}
	if r.ProfileType != NetworkProfileEthernet && r.ProfileType != NetworkProfileWiFi {
		return fmt.Errorf("networkProfile %q profileType must be ethernet or wifi", r.Name)
	}
	if err := r.Selector.Validate(); err != nil {
		return fmt.Errorf("networkProfile %q selector: %w", r.Name, err)
	}
	if r.Selector.Type != "" && r.Selector.Type != r.ProfileType {
		return fmt.Errorf("networkProfile %q selector type conflicts with profileType", r.Name)
	}
	if r.MTU != 0 && (r.MTU < 576 || r.MTU > 9216) {
		return fmt.Errorf("networkProfile %q mtu must be between 576 and 9216", r.Name)
	}
	for _, method := range []struct{ field, value string }{{"ipv4Method", r.IPv4Method}, {"ipv6Method", r.IPv6Method}} {
		if method.value != "" && method.value != "auto" && method.value != "manual" && method.value != "disabled" && method.value != "ignore" {
			return fmt.Errorf("networkProfile %q %s %q is invalid", r.Name, method.field, method.value)
		}
	}
	for _, address := range r.Addresses {
		prefix, err := netip.ParsePrefix(address)
		if err != nil || prefix.String() != address {
			return fmt.Errorf("networkProfile %q address %q is not a canonical prefix", r.Name, address)
		}
	}
	if r.ProfileType == NetworkProfileEthernet && (r.SSID != "" || r.CredentialRef != "") {
		return fmt.Errorf("ethernet networkProfile %q cannot declare Wi-Fi fields", r.Name)
	}
	if r.ProfileType == NetworkProfileWiFi && strings.TrimSpace(r.SSID) == "" {
		return fmt.Errorf("Wi-Fi networkProfile %q requires ssid", r.Name)
	}
	if strings.ContainsAny(r.CredentialRef, "\r\n\x00") {
		return fmt.Errorf("networkProfile %q credentialRef is invalid", r.Name)
	}
	if r.CredentialRef != "" {
		provider, identifier, found := strings.Cut(r.CredentialRef, ":")
		if !found || strings.TrimSpace(identifier) == "" || strings.ContainsAny(identifier, " \t") || (provider != "remotr" && provider != "local-file" && provider != "file") {
			return fmt.Errorf("networkProfile %q credentialRef must use remotr or local-file reference syntax", r.Name)
		}
	}
	if !r.IsAudit() {
		timeout, err := time.ParseDuration(r.RollbackTimeout)
		if err != nil || timeout < 30*time.Second || timeout > 15*time.Minute {
			return fmt.Errorf("networkProfile %q rollbackTimeout must be between 30s and 15m", r.Name)
		}
	}
	return nil
}

func (s NetworkInterfaceSelector) Validate() error {
	if s.Name == "" && s.PermanentMAC == "" && s.Type == "" {
		return fmt.Errorf("at least one of name, permanentMAC, or type is required")
	}
	if s.Name != "" && !networkInterfaceName.MatchString(s.Name) {
		return fmt.Errorf("name %q is invalid", s.Name)
	}
	if s.Type != "" && s.Type != NetworkProfileEthernet && s.Type != NetworkProfileWiFi {
		return fmt.Errorf("type %q is invalid", s.Type)
	}
	if s.PermanentMAC != "" {
		address, err := net.ParseMAC(s.PermanentMAC)
		if err != nil || len(address) != 6 {
			return fmt.Errorf("permanentMAC %q is invalid", s.PermanentMAC)
		}
	}
	return nil
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
