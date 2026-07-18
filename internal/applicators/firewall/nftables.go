package firewall

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DavidHoenisch/go-sysinfo/nftables"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type nftablesBackend struct {
	exec executil.Runner
}

func (b *nftablesBackend) name() string { return "nftables" }

func (b *nftablesBackend) available() bool {
	r := nftables.Reader{Cmd: cmdRunnerAdapter{runner: b.exec}}
	return r.Available()
}

func (b *nftablesBackend) state(ctx context.Context, rule models.FirewallResource) (bool, error) {
	// nft's JSON expression model does not contain the original textual rule
	// as one stable substring. Use the handle-bearing chain listing, which is
	// also the public provider seam used for exact managed-rule deletion.
	lines, err := b.ownedChainLines(rule)
	if err != nil {
		return false, err
	}
	expected := b.buildNftRule(rule)
	if expected == "" {
		return false, nil
	}
	identity := managedRuleIdentity(rule)
	for _, line := range lines {
		if strings.Contains(line, identity) && strings.Contains(line, expected) {
			return true, nil
		}
	}
	return false, nil
}

func (b *nftablesBackend) apply(ctx context.Context, rule models.FirewallResource) error {
	table := rule.Table
	if table == "" {
		table = "filter"
	}
	chain := rule.Chain
	if chain == "" {
		chain = "input"
	}
	family := rule.Family
	if family == "" {
		family = "inet"
	}

	// Ensure table and chain exist.
	_, _, _ = b.exec.Run("nft", "add", "table", family, table)
	_, _, _ = b.exec.Run("nft", "add", "chain", family, table, chain, "{", "type", "filter", "hook", "input", "priority", "filter;", "}")

	// Build the rule.
	ruleStr := b.buildNftRule(rule)
	if ruleStr == "" {
		return fmt.Errorf("nftables: unable to build rule for %q", rule.Name)
	}

	_, _, err := b.exec.Run("nft", "add", "rule", family, table, chain, ruleStr)
	if err != nil {
		return fmt.Errorf("nftables add rule: %w", err)
	}
	return nil
}

func (b *nftablesBackend) revert(ctx context.Context, rule models.FirewallResource) error {
	table := rule.Table
	if table == "" {
		table = "filter"
	}
	chain := rule.Chain
	if chain == "" {
		chain = "input"
	}
	family := rule.Family
	if family == "" {
		family = "inet"
	}

	out, _, err := b.exec.Run("nft", "-a", "list", "chain", family, table, chain)
	if err != nil {
		return fmt.Errorf("nftables list managed rule: %w", err)
	}
	want := managedRuleIdentity(rule)
	handleRE := regexp.MustCompile(`# handle ([0-9]+)`) // nft stable machine output with -a
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, want) {
			continue
		}
		match := handleRE.FindStringSubmatch(line)
		if len(match) != 2 {
			return fmt.Errorf("nftables managed rule %q has no handle", want)
		}
		if _, err := strconv.ParseUint(match[1], 10, 64); err != nil {
			return fmt.Errorf("invalid nftables handle: %w", err)
		}
		_, _, err = b.exec.Run("nft", "delete", "rule", family, table, chain, "handle", match[1])
		return err
	}
	return nil
}

func (b *nftablesBackend) stateOwned(ctx context.Context, resource models.FirewallResource) (bool, error) {
	lines, err := b.ownedChainLines(resource)
	if err != nil {
		return false, err
	}
	desired := make(map[string]struct{}, len(resource.Rules))
	for _, member := range resource.MemberResources() {
		identity := managedRuleIdentity(member)
		desired[identity] = struct{}{}
		found := false
		for _, line := range lines {
			if strings.Contains(line, identity) && strings.Contains(line, b.buildNftRule(member)) {
				found = true
				break
			}
		}
		if !found && resource.Lifecycle != models.LifecycleAbsent {
			return false, nil
		}
	}
	if resource.Ownership != models.OwnershipAuthoritative && resource.Lifecycle != models.LifecycleAbsent {
		return true, nil
	}
	prefix := `comment "remotr:` + resource.Name + `/`
	for _, line := range lines {
		identity := nftManagedIdentity(line, prefix)
		if identity == "" {
			continue
		}
		if resource.Lifecycle == models.LifecycleAbsent {
			return false, nil
		}
		if _, ok := desired[identity]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func (b *nftablesBackend) applyOwned(ctx context.Context, resource models.FirewallResource) error {
	lines, err := b.ownedChainLines(resource)
	if err != nil {
		return err
	}
	desired := make(map[string]struct{}, len(resource.Rules))
	if resource.Lifecycle != models.LifecycleAbsent {
		for _, member := range resource.MemberResources() {
			identity := managedRuleIdentity(member)
			desired[identity] = struct{}{}
			present := false
			for _, line := range lines {
				if strings.Contains(line, identity) && strings.Contains(line, b.buildNftRule(member)) {
					present = true
					break
				}
			}
			if !present {
				if err := b.apply(ctx, member); err != nil {
					return err
				}
			}
		}
	}
	if resource.Ownership != models.OwnershipAuthoritative && resource.Lifecycle != models.LifecycleAbsent {
		return nil
	}

	prefix := `comment "remotr:` + resource.Name + `/`
	type staleRule struct{ handle string }
	var stale []staleRule
	handleRE := regexp.MustCompile(`# handle ([0-9]+)`)
	for _, line := range lines {
		identity := nftManagedIdentity(line, prefix)
		if identity == "" {
			continue
		}
		if _, keep := desired[identity]; keep && resource.Lifecycle != models.LifecycleAbsent {
			continue
		}
		match := handleRE.FindStringSubmatch(line)
		if len(match) != 2 {
			return fmt.Errorf("nftables managed rule %q has no handle", identity)
		}
		stale = append(stale, staleRule{handle: match[1]})
	}
	if len(stale) > resource.CleanupLimit {
		return fmt.Errorf("nftables authoritative cleanup for %q would remove %d rules, exceeding cleanupLimit %d", resource.Name, len(stale), resource.CleanupLimit)
	}
	family, table, chain := nftBoundary(resource)
	for _, rule := range stale {
		if _, _, err := b.exec.Run("nft", "delete", "rule", family, table, chain, "handle", rule.handle); err != nil {
			return fmt.Errorf("nftables delete stale managed rule handle %s: %w", rule.handle, err)
		}
	}
	return nil
}

func (b *nftablesBackend) ownedChainLines(resource models.FirewallResource) ([]string, error) {
	family, table, chain := nftBoundary(resource)
	out, _, err := b.exec.Run("nft", "-a", "list", "chain", family, table, chain)
	if err != nil {
		return nil, fmt.Errorf("nftables list owned chain: %w", err)
	}
	return strings.Split(string(out), "\n"), nil
}

func nftBoundary(resource models.FirewallResource) (string, string, string) {
	family, table, chain := resource.Family, resource.Table, resource.Chain
	if family == "" {
		family = "inet"
	}
	if table == "" {
		table = "filter"
	}
	if chain == "" {
		chain = "input"
	}
	return family, table, chain
}

func nftManagedIdentity(line, prefix string) string {
	start := strings.Index(line, prefix)
	if start < 0 {
		return ""
	}
	start += len(`comment "`)
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return ""
	}
	return line[start : start+end]
}

// buildNftRule constructs an nftables rule string from a FirewallResource.
func (b *nftablesBackend) buildNftRule(rule models.FirewallResource) string {
	// If a raw rule is provided, use it directly.
	if strings.TrimSpace(rule.Rule) != "" {
		return strings.TrimSpace(rule.Rule)
	}

	var parts []string

	if len(rule.Sources) > 0 {
		if len(rule.Sources) == 1 {
			parts = append(parts, fmt.Sprintf("ip saddr %s", rule.Sources[0]))
		} else {
			parts = append(parts, fmt.Sprintf("ip saddr { %s }", strings.Join(rule.Sources, ", ")))
		}
	}
	if len(rule.Destinations) > 0 {
		if len(rule.Destinations) == 1 {
			parts = append(parts, fmt.Sprintf("ip daddr %s", rule.Destinations[0]))
		} else {
			parts = append(parts, fmt.Sprintf("ip daddr { %s }", strings.Join(rule.Destinations, ", ")))
		}
	}
	if len(rule.Ports) > 0 {
		proto := strings.ToLower(rule.Protocol)
		if proto == "" {
			proto = "tcp"
		}
		if len(rule.Ports) == 1 {
			parts = append(parts, fmt.Sprintf("%s dport %d", proto, rule.Ports[0]))
		} else {
			portStrs := make([]string, len(rule.Ports))
			for i, p := range rule.Ports {
				portStrs[i] = fmt.Sprintf("%d", p)
			}
			parts = append(parts, fmt.Sprintf("%s dport { %s }", proto, strings.Join(portStrs, ", ")))
		}
	}
	if len(rule.Services) > 0 {
		// Services map to well-known ports for nftables.
		for _, svc := range rule.Services {
			parts = append(parts, fmt.Sprintf("# service %s", svc))
		}
	}

	action := strings.ToLower(rule.Action)
	switch action {
	case "allow":
		parts = append(parts, "accept")
	case "deny", "drop":
		parts = append(parts, "drop")
	case "reject":
		parts = append(parts, "reject")
	default:
		parts = append(parts, action)
	}

	if len(parts) == 0 {
		return ""
	}
	parts = append(parts, fmt.Sprintf("comment \"%s\"", managedRuleIdentity(rule)))
	return strings.Join(parts, " ")
}

func managedRuleIdentity(rule models.FirewallResource) string { return "remotr:" + rule.Name }
