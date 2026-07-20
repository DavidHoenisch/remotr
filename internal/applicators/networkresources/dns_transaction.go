package networkresources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
)

const defaultNetworkManagerRollbackTimeout = 2 * time.Minute

type dnsControlPathPlan struct {
	Host         string            `json:"host"`
	Destinations []string          `json:"destinations"`
	Routes       []dnsControlRoute `json:"routes"`
}

type dnsControlRoute struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"device"`
}

type dnsRouteObservation struct {
	Destination string `json:"dst"`
	Gateway     string `json:"gateway"`
	Device      string `json:"dev"`
}

// Preflight resolves and routes the active Remotr control endpoint before any
// resolver state changes and binds the mutation to one NetworkManager device.
func (a *DNSApplicator) Preflight(ctx context.Context) error {
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if a.Resource.Enforce == nil || !*a.Resource.Enforce {
		return fmt.Errorf("dnsResolver %q requires explicit enforce authorization", a.Resource.Name)
	}
	if strings.TrimSpace(a.StateDir) == "" {
		return fmt.Errorf("dnsResolver %q requires agent stateDir for timed rollback", a.Resource.Name)
	}
	u, err := url.Parse(a.SyncURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("dnsResolver %q requires a valid Remotr sync URL", a.Resource.Name)
	}
	addresses, err := a.resolveControlAddresses(ctx, u.Hostname())
	if err != nil {
		return fmt.Errorf("dnsResolver %q resolve Remotr destination %q: %w", a.Resource.Name, u.Hostname(), err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("dnsResolver %q resolve Remotr destination %q: no addresses", a.Resource.Name, u.Hostname())
	}
	routes := make([]dnsControlRoute, 0, len(addresses))
	for _, destination := range addresses {
		stdout, stderr, routeErr := a.Runner.Run("ip", "-json", "route", "get", destination)
		if routeErr != nil {
			return fmt.Errorf("dnsResolver %q inspect route to %s: %s: %w", a.Resource.Name, destination, boundedDiagnostic(stderr), routeErr)
		}
		var observed []dnsRouteObservation
		if err := json.Unmarshal(stdout, &observed); err != nil || len(observed) != 1 || observed[0].Device == "" {
			return fmt.Errorf("dnsResolver %q received ambiguous route to %s", a.Resource.Name, destination)
		}
		routes = append(routes, dnsControlRoute{Destination: destination, Gateway: observed[0].Gateway, Device: observed[0].Device})
	}
	stdout, stderr, err := a.Runner.Run("nmcli", "-g", "GENERAL.DBUS-PATH", "device", "show", a.Resource.Interface)
	if err != nil {
		return fmt.Errorf("dnsResolver %q resolve NetworkManager device object: %s: %w", a.Resource.Name, boundedDiagnostic(stderr), err)
	}
	devicePath := strings.TrimSpace(string(stdout))
	if !strings.HasPrefix(devicePath, "/org/freedesktop/NetworkManager/Devices/") || strings.ContainsAny(devicePath, " \t\r\n") {
		return fmt.Errorf("dnsResolver %q received invalid NetworkManager device object", a.Resource.Name)
	}
	a.devicePath = devicePath
	a.rollbackTimeout = defaultNetworkManagerRollbackTimeout
	a.controlPlan = dnsControlPathPlan{Host: u.Hostname(), Destinations: addresses, Routes: routes}
	return nil
}

func (a *DNSApplicator) resolveControlAddresses(ctx context.Context, host string) ([]string, error) {
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

func (a *DNSApplicator) PreflightRollback(ctx context.Context) error {
	if a.devicePath == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	store, intent, err := a.dnsTransactionIntent("/org/freedesktop/NetworkManager/Checkpoint/preflight")
	if err != nil {
		return err
	}
	return store.Preflight(ctx, intent)
}

func (a *DNSApplicator) prepareDNSTransaction(ctx context.Context) (*networkstate.Store, error) {
	stdout, stderr, err := a.Runner.Run(
		"busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager",
		"org.freedesktop.NetworkManager", "CheckpointCreate", "aouu", "1", a.devicePath,
		strconv.FormatInt(int64(a.rollbackTimeout/time.Second), 10), "0",
	)
	if err != nil {
		return nil, fmt.Errorf("create NetworkManager DNS checkpoint: %s: %w", boundedDiagnostic(stderr), err)
	}
	checkpoint := parseNetworkManagerCheckpoint(stdout)
	if checkpoint == "" {
		return nil, errors.New("create NetworkManager DNS checkpoint: invalid object path")
	}
	store, intent, err := a.dnsTransactionIntent(checkpoint)
	if err == nil {
		_, err = store.Prepare(ctx, intent)
	}
	if err != nil {
		_, _, _ = a.Runner.Run("busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager", "org.freedesktop.NetworkManager", "CheckpointDestroy", "o", checkpoint)
		return nil, err
	}
	return store, nil
}

func (a *DNSApplicator) dnsTransactionIntent(checkpoint string) (*networkstate.Store, networkstate.Intent, error) {
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
		ID: fmt.Sprintf("%x", idSum[:16]), Address: "dnsResolver/" + a.Resource.Name,
		ArtifactDigest: fmt.Sprintf("sha256:%x", resourceSum), Attempt: attempt,
		Backend: "network-manager", Deadline: now.Add(a.rollbackTimeout), Checkpoint: checkpoint,
		PlanHash: fmt.Sprintf("sha256:%x", planSum),
	}
	return store, intent, nil
}

func (a *DNSApplicator) armDNSRollbackWatchdog(store *networkstate.Store) {
	if store == nil || a.AfterFunc == nil {
		return
	}
	a.AfterFunc(a.rollbackTimeout, func() {
		_, _ = store.Reconcile(context.Background())
	})
}

func (a *DNSApplicator) populateDNSTransactionReport(report *DNSStateReport) error {
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
	if status.Intent == nil || status.Intent.Address != "dnsResolver/"+a.Resource.Name {
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

func (a *DNSApplicator) Revert(ctx context.Context) error {
	if strings.TrimSpace(a.StateDir) == "" {
		return appErr.ErrNoOp
	}
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Runner, Now: a.now})
	if err != nil {
		return err
	}
	status, err := store.Status()
	if err != nil {
		return err
	}
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || status.Intent.Address != "dnsResolver/"+a.Resource.Name {
		return appErr.ErrNoOp
	}
	_, err = store.Rollback(ctx, "executor_revert")
	return err
}

func (a *DNSApplicator) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func parseNetworkManagerCheckpoint(raw []byte) string {
	fields := strings.Fields(string(raw))
	if len(fields) != 2 || fields[0] != "o" {
		return ""
	}
	path := strings.Trim(fields[1], "\"")
	if !strings.HasPrefix(path, "/org/freedesktop/NetworkManager/Checkpoint/") || strings.ContainsAny(path, " \t\r\n") {
		return ""
	}
	return path
}
