package models

import (
	"fmt"
	"net"
	"strings"
)

func (r HostsEntryResource) Validate() error {
	if r.Lifecycle != "" && r.Lifecycle != LifecyclePresent && r.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("hostsEntry %q lifecycle must be present or absent", r.Name)
	}
	if r.Ownership != "" && r.Ownership != OwnershipNamed {
		return fmt.Errorf("hostsEntry %q ownership must be named", r.Name)
	}
	if strings.TrimSpace(r.Name) == "" || strings.ContainsAny(r.Name, "\r\n#") {
		return fmt.Errorf("hostsEntry requires a safe non-empty name")
	}
	if r.Lifecycle == LifecycleAbsent {
		if r.Address != "" || r.CanonicalHost != "" || len(r.Aliases) != 0 {
			return fmt.Errorf("absent hostsEntry %q must omit address, canonicalHost, and aliases", r.Name)
		}
		return nil
	}
	if net.ParseIP(r.Address) == nil {
		return fmt.Errorf("hostsEntry %q address %q is not an IP address", r.Name, r.Address)
	}
	if err := validateHostToken(r.CanonicalHost); err != nil {
		return fmt.Errorf("hostsEntry %q canonicalHost: %w", r.Name, err)
	}
	seen := map[string]struct{}{strings.ToLower(r.CanonicalHost): {}}
	for _, alias := range r.Aliases {
		if err := validateHostToken(alias); err != nil {
			return fmt.Errorf("hostsEntry %q alias %q: %w", r.Name, alias, err)
		}
		key := strings.ToLower(alias)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("hostsEntry %q contains duplicate host %q", r.Name, alias)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateHostToken(value string) error {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\t\r\n#") {
		return fmt.Errorf("must be a non-empty hostname without whitespace or comments")
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("contains invalid DNS label")
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' && ch != '_' {
				return fmt.Errorf("contains invalid hostname character %q", ch)
			}
		}
	}
	return nil
}
