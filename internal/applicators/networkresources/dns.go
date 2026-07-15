// Package networkresources implements the separate portable DNS and route
// contracts through explicitly selected network providers.
package networkresources

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type DNSApplicator struct {
	Resource models.DNSResolverResource
	Runner   executil.Runner
}

type DNSObservedScope struct {
	Managed       bool     `json:"managed"`
	Compliant     bool     `json:"compliant"`
	Servers       []string `json:"servers,omitempty"`
	SearchDomains []string `json:"searchDomains,omitempty"`
}

type DNSStateReport struct {
	Backend    string           `json:"backend"`
	Configured DNSObservedScope `json:"configured"`
	Effective  DNSObservedScope `json:"effective"`
}

func NewDNS(resource models.DNSResolverResource, runner executil.Runner) *DNSApplicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &DNSApplicator{Resource: resource, Runner: runner}
}

func (a *DNSApplicator) Name() string        { return "dns-resolver:" + a.Resource.Name }
func (a *DNSApplicator) Description() string { return "DNS resolver " + a.Resource.Name }
func (a *DNSApplicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (a *DNSApplicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("DNS resolver " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return networkCheckFailed(desired, err)
	}
	if err := a.Resource.Validate(); err != nil {
		return networkCheckFailed(desired, err)
	}
	report := DNSStateReport{Backend: a.Resource.Provider}
	connection := ""
	var err error
	if a.Resource.Configured {
		connection, err = networkManagerConnection(a.Runner, a.Resource.Interface)
		if err != nil {
			return networkCheckFailed(desired, err)
		}
		report.Configured, err = a.configuredState(connection)
		if err != nil {
			return networkCheckFailed(desired, err)
		}
	}
	if a.Resource.Effective {
		report.Effective, err = a.effectiveState()
		if err != nil {
			return networkCheckFailed(desired, err)
		}
	}
	if (!a.Resource.Configured || report.Configured.Compliant) && (!a.Resource.Effective || report.Effective.Compliant) {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, Actual: report}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "configured or effective DNS state differs", Actual: report}
}

func (a *DNSApplicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status != executor.Drifted {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("DNS resolver %q is not applicable: %s", a.Resource.Name, check.Status)
	}
	report := check.Actual.(DNSStateReport)
	if a.Resource.Configured && !report.Configured.Compliant {
		connection, err := networkManagerConnection(a.Runner, a.Resource.Interface)
		if err != nil {
			return err
		}
		if err := a.applyConfigured(connection); err != nil {
			return err
		}
	}
	if a.Resource.Effective && !report.Effective.Compliant {
		if err := a.applyEffective(); err != nil {
			return err
		}
	}
	return nil
}

func (a *DNSApplicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
}

func (a *DNSApplicator) Revert(context.Context) error { return appErr.ErrNoOp }

func networkManagerConnection(runner executil.Runner, interfaceName string) (string, error) {
	stdout, stderr, err := runner.Run("nmcli", "-t", "-f", "GENERAL.CONNECTION", "device", "show", interfaceName)
	if err != nil {
		return "", fmt.Errorf("resolve NetworkManager connection for %s: %s: %w", interfaceName, boundedDiagnostic(stderr), err)
	}
	values := propertyValues(stdout, "GENERAL.CONNECTION")
	if len(values) != 1 || values[0] == "" || values[0] == "--" {
		return "", fmt.Errorf("interface %q has no unambiguous active NetworkManager connection", interfaceName)
	}
	return values[0], nil
}

func (a *DNSApplicator) configuredState(connection string) (DNSObservedScope, error) {
	stdout, stderr, err := a.Runner.Run("nmcli", "-t", "-f", "ipv4.dns,ipv6.dns,ipv4.dns-search,ipv6.dns-search", "connection", "show", connection)
	if err != nil {
		return DNSObservedScope{}, fmt.Errorf("observe configured DNS: %s: %w", boundedDiagnostic(stderr), err)
	}
	return a.scope(propertyValuesBySuffix(stdout, ".dns"), propertyValuesBySuffix(stdout, ".dns-search")), nil
}

func (a *DNSApplicator) effectiveState() (DNSObservedScope, error) {
	stdout, stderr, err := a.Runner.Run("nmcli", "-t", "-f", "IP4.DNS,IP6.DNS,IP4.DOMAIN,IP6.DOMAIN", "device", "show", a.Resource.Interface)
	if err != nil {
		return DNSObservedScope{}, fmt.Errorf("observe effective DNS: %s: %w", boundedDiagnostic(stderr), err)
	}
	return a.scope(propertyValuesByContains(stdout, ".DNS"), propertyValuesByContains(stdout, ".DOMAIN")), nil
}

func (a *DNSApplicator) scope(servers, domains []string) DNSObservedScope {
	servers, domains = normalized(servers), normalized(domains)
	wantServers, wantDomains := normalized(a.Resource.Servers), normalized(a.Resource.SearchDomains)
	present := a.Resource.Lifecycle != models.LifecycleAbsent
	compliant := len(servers) == 0 && len(domains) == 0
	if present {
		compliant = slices.Equal(servers, wantServers) && slices.Equal(domains, wantDomains)
	}
	return DNSObservedScope{Managed: true, Compliant: compliant, Servers: servers, SearchDomains: domains}
}

func (a *DNSApplicator) applyConfigured(connection string) error {
	servers4, servers6 := splitAddressFamilies(a.Resource.Servers)
	domains := strings.Join(a.Resource.SearchDomains, ",")
	ignore := "yes"
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		ignore = "no"
		servers4, servers6, domains = nil, nil, ""
	}
	_, stderr, err := a.Runner.Run("nmcli", "connection", "modify", connection,
		"ipv4.ignore-auto-dns", ignore, "ipv4.dns", strings.Join(servers4, ","),
		"ipv6.ignore-auto-dns", ignore, "ipv6.dns", strings.Join(servers6, ","),
		"ipv4.dns-search", domains, "ipv6.dns-search", domains)
	if err != nil {
		return fmt.Errorf("configure DNS on %s: %s: %w", connection, boundedDiagnostic(stderr), err)
	}
	return nil
}

func (a *DNSApplicator) applyEffective() error {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		_, stderr, err := a.Runner.Run("resolvectl", "revert", a.Resource.Interface)
		if err != nil {
			return fmt.Errorf("revert effective DNS on %s: %s: %w", a.Resource.Interface, boundedDiagnostic(stderr), err)
		}
		return nil
	}
	args := append([]string{"dns", a.Resource.Interface}, a.Resource.Servers...)
	if _, stderr, err := a.Runner.Run("resolvectl", args...); err != nil {
		return fmt.Errorf("set effective DNS servers on %s: %s: %w", a.Resource.Interface, boundedDiagnostic(stderr), err)
	}
	args = append([]string{"domain", a.Resource.Interface}, a.Resource.SearchDomains...)
	if _, stderr, err := a.Runner.Run("resolvectl", args...); err != nil {
		return fmt.Errorf("set effective DNS domains on %s: %s: %w", a.Resource.Interface, boundedDiagnostic(stderr), err)
	}
	return nil
}

func propertyValues(raw []byte, property string) []string {
	var values []string
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && key == property {
			values = appendCSV(values, value)
		}
	}
	return values
}

func propertyValuesBySuffix(raw []byte, suffix string) []string {
	var values []string
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.HasSuffix(key, suffix) {
			values = appendCSV(values, value)
		}
	}
	return values
}

func propertyValuesByContains(raw []byte, marker string) []string {
	var values []string
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.Contains(key, marker) {
			values = appendCSV(values, value)
		}
	}
	return values
}

func appendCSV(values []string, raw string) []string {
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" && value != "--" {
			values = append(values, value)
		}
	}
	return values
}

func normalized(values []string) []string {
	values = append([]string(nil), values...)
	sort.Strings(values)
	return slices.Compact(values)
}

func splitAddressFamilies(servers []string) ([]string, []string) {
	var ipv4, ipv6 []string
	for _, server := range servers {
		if strings.Contains(server, ":") {
			ipv6 = append(ipv6, server)
		} else {
			ipv4 = append(ipv4, server)
		}
	}
	return ipv4, ipv6
}

func networkCheckFailed(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

func boundedDiagnostic(raw []byte) string {
	value := strings.TrimSpace(string(raw))
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}
