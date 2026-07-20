package networkresources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
)

// Preflight proves the active Remotr path does not overlap the managed route
// and binds the mutation to one active NetworkManager connection and device.
func (a *RouteApplicator) Preflight(ctx context.Context) error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if a.Resource.Enforce == nil || !*a.Resource.Enforce {
		return fmt.Errorf("route %q requires explicit enforce authorization", a.Resource.Name)
	}
	if strings.TrimSpace(a.StateDir) == "" {
		return fmt.Errorf("route %q requires agent stateDir for timed rollback", a.Resource.Name)
	}
	u, err := url.Parse(a.SyncURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("route %q requires a valid Remotr sync URL", a.Resource.Name)
	}
	addresses, err := a.resolveRouteControlAddresses(ctx, u.Hostname())
	if err != nil {
		return fmt.Errorf("route %q resolve Remotr destination %q: %w", a.Resource.Name, u.Hostname(), err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("route %q resolve Remotr destination %q: no addresses", a.Resource.Name, u.Hostname())
	}
	managedPrefix, _ := netip.ParsePrefix(a.Resource.Destination)
	routes := make([]dnsControlRoute, 0, len(addresses))
	for _, destination := range addresses {
		address, parseErr := netip.ParseAddr(destination)
		if parseErr != nil {
			return fmt.Errorf("route %q received invalid Remotr destination %q", a.Resource.Name, destination)
		}
		if managedPrefix.Contains(address) {
			return fmt.Errorf("route %q destination %q contains protected Remotr address %s", a.Resource.Name, a.Resource.Destination, destination)
		}
		stdout, stderr, routeErr := a.Runner.Run("ip", "-json", "route", "get", destination)
		if routeErr != nil {
			return fmt.Errorf("route %q inspect route to %s: %s: %w", a.Resource.Name, destination, boundedDiagnostic(stderr), routeErr)
		}
		var observed []dnsRouteObservation
		if err := json.Unmarshal(stdout, &observed); err != nil || len(observed) != 1 || observed[0].Device == "" {
			return fmt.Errorf("route %q received ambiguous control path to %s", a.Resource.Name, destination)
		}
		routes = append(routes, dnsControlRoute{Destination: destination, Gateway: observed[0].Gateway, Device: observed[0].Device})
	}
	if a.connection == "" {
		a.connection, err = networkManagerConnection(a.Runner, a.Resource.Interface)
		if err != nil {
			return err
		}
	}
	stdout, stderr, err := a.Runner.Run("nmcli", "-g", "GENERAL.DBUS-PATH", "device", "show", a.Resource.Interface)
	if err != nil {
		return fmt.Errorf("route %q resolve NetworkManager device object: %s: %w", a.Resource.Name, boundedDiagnostic(stderr), err)
	}
	devicePath := strings.TrimSpace(string(stdout))
	if !strings.HasPrefix(devicePath, "/org/freedesktop/NetworkManager/Devices/") || strings.ContainsAny(devicePath, " \t\r\n") {
		return fmt.Errorf("route %q received invalid NetworkManager device object", a.Resource.Name)
	}
	a.devicePath = devicePath
	a.rollbackTimeout = defaultNetworkManagerRollbackTimeout
	a.controlPlan = dnsControlPathPlan{Host: u.Hostname(), Destinations: addresses, Routes: routes}
	return nil
}

func (a *RouteApplicator) resolveRouteControlAddresses(ctx context.Context, host string) ([]string, error) {
	if address := net.ParseIP(host); address != nil {
		return []string{address.String()}, nil
	}
	resolver := a.ResolveIP
	if resolver == nil {
		resolver = net.DefaultResolver.LookupIPAddr
	}
	resolved, err := resolver(ctx, host)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(resolved))
	addresses := make([]string, 0, len(resolved))
	for _, resolvedAddress := range resolved {
		address := resolvedAddress.IP.String()
		if address == "" || address == "<nil>" {
			continue
		}
		if _, found := seen[address]; found {
			continue
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	return addresses, nil
}

func (a *RouteApplicator) PreflightRollback(ctx context.Context) error {
	if a.devicePath == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	store, intent, err := a.routeTransactionIntent("/org/freedesktop/NetworkManager/Checkpoint/preflight")
	if err != nil {
		return err
	}
	return store.Preflight(ctx, intent)
}

func (a *RouteApplicator) prepareRouteTransaction(ctx context.Context) (*networkstate.Store, error) {
	stdout, stderr, err := a.Runner.Run(
		"busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager",
		"org.freedesktop.NetworkManager", "CheckpointCreate", "aouu", "1", a.devicePath,
		strconv.FormatInt(int64(a.rollbackTimeout/time.Second), 10), "0",
	)
	if err != nil {
		return nil, fmt.Errorf("create NetworkManager route checkpoint: %s: %w", boundedDiagnostic(stderr), err)
	}
	checkpoint := parseNetworkManagerCheckpoint(stdout)
	if checkpoint == "" {
		return nil, errors.New("create NetworkManager route checkpoint: invalid object path")
	}
	store, intent, err := a.routeTransactionIntent(checkpoint)
	if err == nil {
		_, err = store.Prepare(ctx, intent)
	}
	if err != nil {
		_, _, _ = a.Runner.Run("busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager", "org.freedesktop.NetworkManager", "CheckpointDestroy", "o", checkpoint)
		return nil, err
	}
	return store, nil
}

func (a *RouteApplicator) routeTransactionIntent(checkpoint string) (*networkstate.Store, networkstate.Intent, error) {
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Runner, Now: a.now})
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	current, err := store.Status()
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	attempt := 1
	if current.Intent != nil {
		if current.Intent.Phase == networkstate.PhaseAwaitingAcknowledgement {
			return nil, networkstate.Intent{}, fmt.Errorf("%w: %s", networkstate.ErrAwaitingAcknowledgement, current.Intent.ID)
		}
		attempt = current.Intent.Attempt + 1
	}
	resourceJSON, err := json.Marshal(a.Resource)
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	resourceSum := sha256.Sum256(resourceJSON)
	planJSON, err := json.Marshal(a.controlPlan)
	if err != nil {
		return nil, networkstate.Intent{}, err
	}
	planSum := sha256.Sum256(planJSON)
	now := a.now()
	idSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%d:%d", resourceSum, attempt, now.UnixNano())))
	intent := networkstate.Intent{
		ID: fmt.Sprintf("%x", idSum[:16]), Address: "route/" + a.Resource.Name,
		ArtifactDigest: fmt.Sprintf("sha256:%x", resourceSum), Attempt: attempt,
		Backend: "network-manager", Deadline: now.Add(a.rollbackTimeout), Checkpoint: checkpoint,
		PlanHash: fmt.Sprintf("sha256:%x", planSum), Interface: a.Resource.Interface, Connection: a.connection,
	}
	return store, intent, nil
}

func (a *RouteApplicator) armRouteRollbackWatchdog(store *networkstate.Store) {
	if store == nil || a.AfterFunc == nil {
		return
	}
	a.AfterFunc(a.rollbackTimeout, func() {
		_, _ = store.Reconcile(context.Background())
	})
}

func (a *RouteApplicator) populateRouteTransactionReport(report *RouteStateReport) error {
	if report == nil || strings.TrimSpace(a.StateDir) == "" {
		return nil
	}
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Runner, Now: a.now})
	if err != nil {
		return err
	}
	status, err := store.Status()
	if err != nil {
		return err
	}
	if status.Intent == nil || status.Intent.Address != "route/"+a.Resource.Name {
		return nil
	}
	switch status.Intent.Phase {
	case networkstate.PhaseAwaitingAcknowledgement:
		report.RollbackOutcome = "awaiting_acknowledgement"
	case networkstate.PhaseAcknowledged:
		report.Acknowledged = status.Intent.AuthenticatedAck
		report.RollbackOutcome = "acknowledged"
	case networkstate.PhaseRolledBack:
		report.RollbackOutcome = "rolled_back"
	case networkstate.PhaseRollbackFailed:
		report.RollbackOutcome = "rollback_failed"
	}
	return nil
}

func (a *RouteApplicator) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}
