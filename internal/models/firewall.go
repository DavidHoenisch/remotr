package models

import (
	"fmt"
	"strings"
	"time"
)

// Validate checks firewall lifecycle, provider boundary, and bounded cleanup
// semantics before a connectivity-risk provider can be constructed.
func (f FirewallResource) Validate() error {
	if f.Lifecycle != "" && f.Lifecycle != LifecyclePresent && f.Lifecycle != LifecycleAbsent {
		return fmt.Errorf("firewall %q lifecycle must be present or absent", f.Name)
	}
	ownership := f.Ownership
	if ownership == "" {
		ownership = OwnershipNamed
	}
	switch ownership {
	case OwnershipNamed:
		if len(f.Rules) != 0 {
			return fmt.Errorf("named firewall %q must use the top-level rule fields", f.Name)
		}
		if f.CleanupLimit != 0 {
			return fmt.Errorf("named firewall %q cannot set cleanupLimit", f.Name)
		}
	case OwnershipFragment, OwnershipAuthoritative:
		if f.Lifecycle != LifecycleAbsent && len(f.Rules) == 0 {
			return fmt.Errorf("firewall %q ownership %q requires rules", f.Name, ownership)
		}
		if hasTopLevelFirewallRule(f) {
			return fmt.Errorf("firewall %q ownership %q must put rule fields under rules", f.Name, ownership)
		}
		if strings.EqualFold(f.Backend, "firewalld") && len(f.Zones) != 1 {
			return fmt.Errorf("firewalld firewall %q ownership %q requires exactly one owned zone", f.Name, ownership)
		}
		if ownership == OwnershipAuthoritative || f.Lifecycle == LifecycleAbsent {
			if f.CleanupLimit < 1 || f.CleanupLimit > 1000 {
				return fmt.Errorf("firewall cleanup for %q cleanupLimit must be between 1 and 1000", f.Name)
			}
		} else if f.CleanupLimit != 0 {
			return fmt.Errorf("firewall fragment %q cannot set cleanupLimit", f.Name)
		}
	default:
		return fmt.Errorf("firewall %q ownership must be named, fragment, or authoritative", f.Name)
	}

	seen := make(map[string]struct{}, len(f.Rules))
	for i, rule := range f.Rules {
		if strings.TrimSpace(rule.Name) == "" || strings.Contains(rule.Name, "/") {
			return fmt.Errorf("firewall %q rule %d requires a non-empty name without '/'", f.Name, i+1)
		}
		if _, exists := seen[rule.Name]; exists {
			return fmt.Errorf("firewall %q contains duplicate rule %q", f.Name, rule.Name)
		}
		seen[rule.Name] = struct{}{}
		if err := validateFirewallActionAndPorts(f.Name+"/"+rule.Name, rule.Action, rule.Ports, rule.Rule); err != nil {
			return err
		}
	}
	if len(f.Rules) == 0 {
		if err := validateFirewallActionAndPorts(f.Name, f.Action, f.Ports, f.Rule); err != nil {
			return err
		}
	}
	if !f.IsAudit() {
		if !strings.EqualFold(f.Backend, "nftables") {
			return fmt.Errorf("enforced firewall %q requires backend nftables until another backend provides transactional restore", f.Name)
		}
		timeout, err := time.ParseDuration(f.RollbackTimeout)
		if err != nil || timeout < 30*time.Second || timeout > 15*time.Minute {
			return fmt.Errorf("enforced firewall %q rollbackTimeout must be between 30s and 15m", f.Name)
		}
	} else if f.RollbackTimeout != "" {
		return fmt.Errorf("audit firewall %q cannot set rollbackTimeout", f.Name)
	}
	return nil
}

func hasTopLevelFirewallRule(f FirewallResource) bool {
	return f.Action != "" || f.Protocol != "" || len(f.Ports) != 0 || len(f.Sources) != 0 ||
		len(f.Destinations) != 0 || len(f.Services) != 0 || f.Rule != ""
}

func validateFirewallActionAndPorts(name, action string, ports []int, raw string) error {
	switch strings.ToLower(action) {
	case "allow", "deny", "drop", "reject":
	case "":
		if strings.TrimSpace(raw) == "" {
			return fmt.Errorf("firewall rule %q requires action or rule", name)
		}
	default:
		return fmt.Errorf("firewall rule %q has unsupported action %q", name, action)
	}
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("firewall rule %q port %d is outside 1..65535", name, port)
		}
	}
	return nil
}

// MemberResources expands an owned collection into the individual rules
// understood by the existing backends while retaining a stable owner prefix.
func (f FirewallResource) MemberResources() []FirewallResource {
	resources := make([]FirewallResource, 0, len(f.Rules))
	for _, rule := range f.Rules {
		resources = append(resources, FirewallResource{
			ResourceMeta:    ResourceMeta{Lifecycle: LifecyclePresent, Ownership: OwnershipNamed},
			Name:            f.Name + "/" + rule.Name,
			Audit:           f.Audit,
			Action:          rule.Action,
			Protocol:        rule.Protocol,
			Ports:           append([]int(nil), rule.Ports...),
			Sources:         append([]string(nil), rule.Sources...),
			Destinations:    append([]string(nil), rule.Destinations...),
			Services:        append([]string(nil), rule.Services...),
			Zones:           append([]string(nil), f.Zones...),
			Backend:         f.Backend,
			Table:           f.Table,
			Chain:           f.Chain,
			Family:          f.Family,
			Rule:            rule.Rule,
			ProtectRemotr:   f.ProtectRemotr,
			RollbackTimeout: f.RollbackTimeout,
		})
	}
	return resources
}
