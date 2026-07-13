package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ControlRoute is the kernel route selected for one resolved Remotr
// destination. It intentionally contains no credentials or packet payloads.
type ControlRoute struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"device"`
	Source      string `json:"source,omitempty"`
}

// ControlPathPlan is the reviewable, non-secret safety input for an enforced
// firewall transaction.
type ControlPathPlan struct {
	Host                      string         `json:"host"`
	Protocol                  string         `json:"protocol"`
	Port                      int            `json:"port"`
	Destinations              []string       `json:"destinations"`
	Routes                    []ControlRoute `json:"routes"`
	DNSServers                []string       `json:"dnsServers,omitempty"`
	SearchDomains             []string       `json:"searchDomains,omitempty"`
	EstablishedControlTraffic bool           `json:"establishedControlTraffic"`
	RollbackTimeout           time.Duration  `json:"rollbackTimeout"`
	Risks                     []string       `json:"risks,omitempty"`
}

type ipRoute struct {
	Destination string `json:"dst"`
	Gateway     string `json:"gateway"`
	Device      string `json:"dev"`
	Source      string `json:"prefsrc"`
}

// Preflight resolves every dependency of the active authenticated control
// path before any packet-filter mutation is allowed.
func (a *Applicator) Preflight(ctx context.Context) error {
	if a.Resource.IsAudit() {
		return nil
	}
	if strings.TrimSpace(a.StateDir) == "" {
		return fmt.Errorf("firewall %q requires agent stateDir for timed rollback", a.Resource.Name)
	}
	timeout, err := time.ParseDuration(a.Resource.RollbackTimeout)
	if err != nil || timeout < 30*time.Second || timeout > 15*time.Minute {
		return fmt.Errorf("firewall %q rollbackTimeout must be between 30s and 15m", a.Resource.Name)
	}
	u, err := url.Parse(a.SyncURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("firewall %q requires a valid Remotr sync URL", a.Resource.Name)
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return fmt.Errorf("firewall %q sync URL has unsupported scheme %q", a.Resource.Name, u.Scheme)
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("firewall %q sync URL has invalid port %q", a.Resource.Name, port)
	}

	resolver := a.ResolveIP
	if resolver == nil {
		resolver = net.DefaultResolver.LookupIPAddr
	}
	addresses, err := resolver(ctx, u.Hostname())
	if err != nil {
		return fmt.Errorf("firewall %q resolve Remotr destination %q: %w", a.Resource.Name, u.Hostname(), err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("firewall %q resolve Remotr destination %q: no addresses", a.Resource.Name, u.Hostname())
	}
	destinations := make([]string, 0, len(addresses))
	routes := make([]ControlRoute, 0, len(addresses))
	established := false
	seen := make(map[string]struct{})
	for _, address := range addresses {
		ip := address.IP.String()
		if ip == "<nil>" || ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		destinations = append(destinations, ip)
		routeOutput, _, routeErr := a.Exec.Run("ip", "-json", "route", "get", ip)
		if routeErr != nil {
			return fmt.Errorf("firewall %q inspect route to %s: %w", a.Resource.Name, ip, routeErr)
		}
		var selected []ipRoute
		if err := json.Unmarshal(routeOutput, &selected); err != nil || len(selected) != 1 || selected[0].Device == "" {
			return fmt.Errorf("firewall %q received ambiguous route to %s", a.Resource.Name, ip)
		}
		routes = append(routes, ControlRoute{Destination: ip, Gateway: selected[0].Gateway, Device: selected[0].Device, Source: selected[0].Source})
		traffic, _, trafficErr := a.Exec.Run("ss", "-Htn", "state", "established", "dst", net.JoinHostPort(ip, port))
		if trafficErr != nil {
			return fmt.Errorf("firewall %q inspect established control traffic to %s: %w", a.Resource.Name, ip, trafficErr)
		}
		established = established || strings.TrimSpace(string(traffic)) != ""
	}
	if len(destinations) == 0 {
		return fmt.Errorf("firewall %q resolved no usable Remotr destination", a.Resource.Name)
	}
	dnsServers, searchDomains, err := a.resolverDependencies()
	if err != nil {
		return fmt.Errorf("firewall %q inspect DNS dependencies: %w", a.Resource.Name, err)
	}
	sort.Strings(destinations)
	sort.Slice(routes, func(i, j int) bool { return routes[i].Destination < routes[j].Destination })
	var risks []string
	if risk := a.validateSyncPath(); risk != nil {
		risks = append(risks, risk.Error())
	}
	a.controlPlan = ControlPathPlan{
		Host: u.Hostname(), Protocol: "tcp", Port: portNumber, Destinations: destinations,
		Routes: routes, DNSServers: dnsServers, SearchDomains: searchDomains,
		EstablishedControlTraffic: established, RollbackTimeout: timeout, Risks: risks,
	}
	return nil
}

func (a *Applicator) resolverDependencies() ([]string, []string, error) {
	reader := a.ReadFile
	if reader == nil {
		return nil, nil, fmt.Errorf("resolver configuration reader unavailable")
	}
	raw, err := reader("/etc/resolv.conf")
	if err != nil {
		return nil, nil, err
	}
	var servers, search []string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		switch fields[0] {
		case "nameserver":
			if net.ParseIP(fields[1]) == nil {
				return nil, nil, fmt.Errorf("invalid nameserver %q", fields[1])
			}
			servers = append(servers, fields[1])
		case "search":
			search = append(search, fields[1:]...)
		}
	}
	return servers, search, nil
}

// TransactionPlan returns a defensive copy of the most recent successful
// preflight plan.
func (a *Applicator) TransactionPlan() ControlPathPlan {
	plan := a.controlPlan
	plan.Destinations = append([]string(nil), plan.Destinations...)
	plan.Routes = append([]ControlRoute(nil), plan.Routes...)
	plan.DNSServers = append([]string(nil), plan.DNSServers...)
	plan.SearchDomains = append([]string(nil), plan.SearchDomains...)
	plan.Risks = append([]string(nil), plan.Risks...)
	return plan
}
