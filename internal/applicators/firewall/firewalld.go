package firewall

import (
	"context"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/go-sysinfo/firewalld"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type firewalldBackend struct {
	exec executil.Runner
}

func (b *firewalldBackend) name() string { return "firewalld" }

func (b *firewalldBackend) available() bool {
	// Use go-sysinfo for the availability check.
	r := firewalld.Reader{Cmd: cmdRunnerAdapter{runner: b.exec}}
	return r.Available()
}

func (b *firewalldBackend) state(ctx context.Context, rule models.FirewallResource) (bool, error) {
	r := firewalld.Reader{Cmd: cmdRunnerAdapter{runner: b.exec}}
	summary, err := r.GetRulesetSummary()
	if err != nil {
		return false, err
	}
	if summary == nil {
		return false, nil
	}

	zones := rule.Zones
	if len(zones) == 0 {
		// If no zones specified, check the default zone.
		defaultZone, err := r.GetDefaultZone()
		if err != nil {
			return false, err
		}
		zones = []string{defaultZone}
	}

	for _, zoneName := range zones {
		var zone *firewalld.ZoneInfo
		for _, z := range summary.Zones {
			if z.Name == zoneName {
				zone = &z
				break
			}
		}
		if zone == nil {
			return false, nil
		}

		// Check services
		for _, svc := range rule.Services {
			if !contains(zone.Services, svc) {
				return false, nil
			}
		}

		// Check ports (format: "port/protocol")
		for _, port := range rule.Ports {
			proto := strings.ToLower(rule.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			portStr := fmt.Sprintf("%d/%s", port, proto)
			if !contains(zone.Ports, portStr) {
				return false, nil
			}
		}

		// Check sources
		for _, src := range rule.Sources {
			if !contains(zone.Sources, src) {
				return false, nil
			}
		}
	}

	return true, nil
}

func (b *firewalldBackend) apply(ctx context.Context, rule models.FirewallResource) error {
	zones := rule.Zones
	if len(zones) == 0 {
		r := firewalld.Reader{Cmd: cmdRunnerAdapter{runner: b.exec}}
		defaultZone, err := r.GetDefaultZone()
		if err != nil {
			return err
		}
		zones = []string{defaultZone}
	}

	for _, zone := range zones {
		// Apply services
		for _, svc := range rule.Services {
			if _, _, err := b.exec.Run("firewall-cmd", "--zone", zone, "--add-service", svc, "--permanent"); err != nil {
				return fmt.Errorf("firewalld add service %q to zone %q: %w", svc, zone, err)
			}
		}

		// Apply ports
		for _, port := range rule.Ports {
			proto := strings.ToLower(rule.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			portStr := fmt.Sprintf("%d/%s", port, proto)
			if _, _, err := b.exec.Run("firewall-cmd", "--zone", zone, "--add-port", portStr, "--permanent"); err != nil {
				return fmt.Errorf("firewalld add port %q to zone %q: %w", portStr, zone, err)
			}
		}

		// Apply sources
		for _, src := range rule.Sources {
			if _, _, err := b.exec.Run("firewall-cmd", "--zone", zone, "--add-source", src, "--permanent"); err != nil {
				return fmt.Errorf("firewalld add source %q to zone %q: %w", src, zone, err)
			}
		}

		// Apply rich rules for action-based rules (allow/deny/reject/drop)
		if rule.Action != "" && (len(rule.Sources) > 0 || len(rule.Destinations) > 0 || len(rule.Ports) > 0) {
			richRule := b.buildRichRule(rule)
			if richRule != "" {
				if _, _, err := b.exec.Run("firewall-cmd", "--zone", zone, "--add-rich-rule", richRule, "--permanent"); err != nil {
					return fmt.Errorf("firewalld add rich rule to zone %q: %w", zone, err)
				}
			}
		}
	}

	// Reload firewalld to apply permanent changes.
	_, _, err := b.exec.Run("firewall-cmd", "--reload")
	return err
}

func (b *firewalldBackend) revert(ctx context.Context, rule models.FirewallResource) error {
	zones := rule.Zones
	if len(zones) == 0 {
		r := firewalld.Reader{Cmd: cmdRunnerAdapter{runner: b.exec}}
		defaultZone, err := r.GetDefaultZone()
		if err != nil {
			return err
		}
		zones = []string{defaultZone}
	}

	for _, zone := range zones {
		for _, svc := range rule.Services {
			_, _, _ = b.exec.Run("firewall-cmd", "--zone", zone, "--remove-service", svc, "--permanent")
		}
		for _, port := range rule.Ports {
			proto := strings.ToLower(rule.Protocol)
			if proto == "" {
				proto = "tcp"
			}
			portStr := fmt.Sprintf("%d/%s", port, proto)
			_, _, _ = b.exec.Run("firewall-cmd", "--zone", zone, "--remove-port", portStr, "--permanent")
		}
		for _, src := range rule.Sources {
			_, _, _ = b.exec.Run("firewall-cmd", "--zone", zone, "--remove-source", src, "--permanent")
		}
		if rule.Action != "" {
			richRule := b.buildRichRule(rule)
			if richRule != "" {
				_, _, _ = b.exec.Run("firewall-cmd", "--zone", zone, "--remove-rich-rule", richRule, "--permanent")
			}
		}
	}

	_, _, err := b.exec.Run("firewall-cmd", "--reload")
	return err
}

func (b *firewalldBackend) stateOwned(ctx context.Context, resource models.FirewallResource) (bool, error) {
	current, desired, err := b.ownedZoneRules(resource)
	if err != nil {
		return false, err
	}
	for rule := range desired {
		if _, ok := current[rule]; !ok && resource.Lifecycle != models.LifecycleAbsent {
			return false, nil
		}
	}
	if resource.Ownership == models.OwnershipAuthoritative || resource.Lifecycle == models.LifecycleAbsent {
		for rule := range current {
			if _, ok := desired[rule]; !ok || resource.Lifecycle == models.LifecycleAbsent {
				return false, nil
			}
		}
	}
	return true, nil
}

func (b *firewalldBackend) applyOwned(ctx context.Context, resource models.FirewallResource) error {
	current, desired, err := b.ownedZoneRules(resource)
	if err != nil {
		return err
	}
	zone := resource.Zones[0]
	if resource.Lifecycle != models.LifecycleAbsent {
		for rule := range desired {
			if _, ok := current[rule]; ok {
				continue
			}
			if _, _, err := b.exec.Run("firewall-cmd", "--zone", zone, "--add-rich-rule", rule, "--permanent"); err != nil {
				return fmt.Errorf("firewalld add rule to owned zone %q: %w", zone, err)
			}
		}
	}
	if resource.Ownership == models.OwnershipAuthoritative || resource.Lifecycle == models.LifecycleAbsent {
		var stale []string
		for rule := range current {
			if _, keep := desired[rule]; keep && resource.Lifecycle != models.LifecycleAbsent {
				continue
			}
			stale = append(stale, rule)
		}
		if len(stale) > resource.CleanupLimit {
			return fmt.Errorf("firewalld authoritative cleanup for %q would remove %d rules, exceeding cleanupLimit %d", resource.Name, len(stale), resource.CleanupLimit)
		}
		for _, rule := range stale {
			if _, _, err := b.exec.Run("firewall-cmd", "--zone", zone, "--remove-rich-rule", rule, "--permanent"); err != nil {
				return fmt.Errorf("firewalld remove stale rule from owned zone %q: %w", zone, err)
			}
		}
	}
	_, _, err = b.exec.Run("firewall-cmd", "--reload")
	return err
}

func (b *firewalldBackend) ownedZoneRules(resource models.FirewallResource) (map[string]struct{}, map[string]struct{}, error) {
	if len(resource.Zones) != 1 {
		return nil, nil, fmt.Errorf("firewalld owned collection %q requires exactly one zone", resource.Name)
	}
	out, _, err := b.exec.Run("firewall-cmd", "--zone", resource.Zones[0], "--list-rich-rules", "--permanent")
	if err != nil {
		return nil, nil, fmt.Errorf("firewalld list owned zone %q: %w", resource.Zones[0], err)
	}
	current := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if rule := strings.TrimSpace(line); rule != "" {
			current[rule] = struct{}{}
		}
	}
	desired := make(map[string]struct{}, len(resource.Rules))
	for _, member := range resource.MemberResources() {
		if rule := b.buildRichRule(member); rule != "" {
			desired[rule] = struct{}{}
		}
	}
	return current, desired, nil
}

func (b *firewalldBackend) buildRichRule(rule models.FirewallResource) string {
	// Build a firewalld rich rule string.
	// Example: rule family="ipv4" source address="10.0.0.0/8" port protocol="tcp" port="22" accept
	var parts []string
	parts = append(parts, "rule")

	if len(rule.Sources) > 0 {
		parts = append(parts, fmt.Sprintf("source address=\"%s\"", strings.Join(rule.Sources, ",")))
	}
	if len(rule.Destinations) > 0 {
		parts = append(parts, fmt.Sprintf("destination address=\"%s\"", strings.Join(rule.Destinations, ",")))
	}
	if len(rule.Ports) > 0 {
		proto := strings.ToLower(rule.Protocol)
		if proto == "" {
			proto = "tcp"
		}
		if len(rule.Ports) == 1 {
			parts = append(parts, fmt.Sprintf("port protocol=\"%s\" port=\"%d\"", proto, rule.Ports[0]))
		} else {
			ports := make([]string, len(rule.Ports))
			for i, p := range rule.Ports {
				ports[i] = fmt.Sprintf("%d", p)
			}
			parts = append(parts, fmt.Sprintf("port protocol=\"%s\" port=\"%s\"", proto, strings.Join(ports, ",")))
		}
	}
	if len(rule.Services) > 0 {
		parts = append(parts, fmt.Sprintf("service name=\"%s\"", strings.Join(rule.Services, ",")))
	}

	action := strings.ToLower(rule.Action)
	switch action {
	case "allow":
		parts = append(parts, "accept")
	case "deny", "drop":
		parts = append(parts, "drop")
	case "reject":
		parts = append(parts, "reject")
	}

	return strings.Join(parts, " ")
}
