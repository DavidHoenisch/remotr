package networkresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type RouteApplicator struct {
	Resource  models.RouteResource
	Runner    executil.Runner
	StateDir  string
	SyncURL   string
	ResolveIP func(context.Context, string) ([]net.IPAddr, error)
	Now       func() time.Time
	AfterFunc func(time.Duration, func())

	devicePath      string
	connection      string
	rollbackTimeout time.Duration
	controlPlan     dnsControlPathPlan
}

type RouteObservedScope struct {
	Managed   bool `json:"managed"`
	Compliant bool `json:"compliant"`
}

type RouteStateReport struct {
	Backend         string             `json:"backend"`
	Mode            string             `json:"mode"`
	Configured      RouteObservedScope `json:"configured"`
	Effective       RouteObservedScope `json:"effective"`
	Acknowledged    bool               `json:"acknowledged"`
	RollbackOutcome string             `json:"rollbackOutcome,omitempty"`
}

func NewRoute(resource models.RouteResource, runner executil.Runner) *RouteApplicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &RouteApplicator{
		Resource: resource, Runner: runner, Now: time.Now,
		AfterFunc: func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) },
	}
}

func (a *RouteApplicator) Name() string        { return "route:" + a.Resource.Name }
func (a *RouteApplicator) Description() string { return "route " + a.Resource.Destination }
func (a *RouteApplicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (a *RouteApplicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("route " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return networkCheckFailed(desired, err)
	}
	if a.Resource.Provider != models.NetworkProviderNetworkManager {
		return executor.CheckResult{
			Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable,
			DesiredSummary: desired, ObservedSummary: "route backend is not advertised by this provider",
		}
	}
	if err := a.Resource.Validate(); err != nil {
		return networkCheckFailed(desired, err)
	}
	mode := "report"
	if a.Resource.Enforce != nil && *a.Resource.Enforce {
		mode = "enforce"
	}
	report := RouteStateReport{Backend: a.Resource.Provider, Mode: mode}
	var err error
	if a.Resource.Configured {
		connection, connectionErr := networkManagerConnection(a.Runner, a.Resource.Interface)
		if connectionErr != nil {
			return networkCheckFailed(desired, connectionErr)
		}
		a.connection = connection
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
	if err := a.populateRouteTransactionReport(&report); err != nil {
		return networkCheckFailed(desired, err)
	}
	if (!a.Resource.Configured || report.Configured.Compliant) && (!a.Resource.Effective || report.Effective.Compliant) {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, Actual: report}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "configured or effective route state differs", Actual: report}
}

func (a *RouteApplicator) Apply(ctx context.Context) error {
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status != executor.Drifted {
		if check.Err != nil {
			return check.Err
		}
		return fmt.Errorf("route %q is not applicable: %s", a.Resource.Name, check.Status)
	}
	report := check.Actual.(RouteStateReport)
	if a.devicePath == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	store, err := a.prepareRouteTransaction(ctx)
	if err != nil {
		return err
	}
	if err := a.applyRouteDrift(report); err != nil {
		if _, rollbackErr := store.Rollback(ctx, "apply_failed"); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("NetworkManager checkpoint rollback failed: %w", rollbackErr))
		}
		return err
	}
	a.armRouteRollbackWatchdog(store)
	return nil
}

func (a *RouteApplicator) applyRouteDrift(report RouteStateReport) error {
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

func (a *RouteApplicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired}
	}
	if errors.Is(err, networkstate.ErrAwaitingAcknowledgement) {
		return executor.ApplyResult{
			Status: executor.ApplyDeferred, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired,
			DeferredWork: &executor.DeferredWork{ReasonCode: executor.ReasonDeferred, Summary: "another connectivity transaction is awaiting authenticated acknowledgement"},
		}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired}
}

func (a *RouteApplicator) Revert(ctx context.Context) error {
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
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || status.Intent.Address != "route/"+a.Resource.Name {
		return appErr.ErrNoOp
	}
	_, err = store.Rollback(ctx, "executor_revert")
	return err
}

func (a *RouteApplicator) configuredState(connection string) (RouteObservedScope, error) {
	property := "ipv4.routes"
	if destination, _ := netip.ParsePrefix(a.Resource.Destination); destination.Addr().Is6() {
		property = "ipv6.routes"
	}
	stdout, stderr, err := a.Runner.Run("nmcli", "-g", property, "connection", "show", connection)
	if err != nil {
		return RouteObservedScope{}, fmt.Errorf("observe configured route: %s: %w", boundedDiagnostic(stderr), err)
	}
	present := routeTextMatches(string(stdout), a.Resource)
	wantPresent := a.Resource.Lifecycle != models.LifecycleAbsent
	return RouteObservedScope{Managed: true, Compliant: present == wantPresent}, nil
}

type kernelRoute struct {
	Destination string          `json:"dst"`
	Gateway     string          `json:"gateway"`
	Device      string          `json:"dev"`
	Metric      int             `json:"metric"`
	Table       json.RawMessage `json:"table"`
}

func (a *RouteApplicator) effectiveState() (RouteObservedScope, error) {
	args := []string{"-json", "route", "show", "exact", a.Resource.Destination}
	if destination, _ := netip.ParsePrefix(a.Resource.Destination); destination.Addr().Is6() {
		args = append([]string{"-6"}, args...)
	}
	if a.Resource.Table != 0 {
		args = append(args, "table", "all")
	}
	stdout, stderr, err := a.Runner.Run("ip", args...)
	if err != nil {
		return RouteObservedScope{}, fmt.Errorf("observe effective route: %s: %w", boundedDiagnostic(stderr), err)
	}
	var routes []kernelRoute
	if err := json.Unmarshal(stdout, &routes); err != nil {
		return RouteObservedScope{}, fmt.Errorf("decode effective route state: %w", err)
	}
	present := false
	for _, route := range routes {
		if route.Destination == a.Resource.Destination && route.Device == a.Resource.Interface && route.Gateway == a.Resource.Gateway && route.Metric == a.Resource.Metric && routeTable(route.Table) == a.Resource.Table {
			present = true
			break
		}
	}
	wantPresent := a.Resource.Lifecycle != models.LifecycleAbsent
	return RouteObservedScope{Managed: true, Compliant: present == wantPresent}, nil
}

func (a *RouteApplicator) applyConfigured(connection string) error {
	property := "+ipv4.routes"
	if destination, _ := netip.ParsePrefix(a.Resource.Destination); destination.Addr().Is6() {
		property = "+ipv6.routes"
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		property = "-" + strings.TrimPrefix(property, "+")
	}
	_, stderr, err := a.Runner.Run("nmcli", "connection", "modify", connection, property, routeSpec(a.Resource))
	if err != nil {
		return fmt.Errorf("configure route on %s: %s: %w", connection, boundedDiagnostic(stderr), err)
	}
	return nil
}

func (a *RouteApplicator) applyEffective() error {
	if a.Resource.Configured {
		_, stderr, err := a.Runner.Run("nmcli", "device", "reapply", a.Resource.Interface)
		if err != nil {
			return fmt.Errorf("activate configured route on %s: %s: %w", a.Resource.Interface, boundedDiagnostic(stderr), err)
		}
		return nil
	}
	property := "+ipv4.routes"
	if destination, _ := netip.ParsePrefix(a.Resource.Destination); destination.Addr().Is6() {
		property = "+ipv6.routes"
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		property = "-" + strings.TrimPrefix(property, "+")
	}
	_, stderr, err := a.Runner.Run("nmcli", "device", "modify", a.Resource.Interface, property, routeSpec(a.Resource))
	if err != nil {
		return fmt.Errorf("modify effective route %s: %s: %w", a.Resource.Destination, boundedDiagnostic(stderr), err)
	}
	return nil
}

func routeSpec(resource models.RouteResource) string {
	parts := []string{resource.Destination}
	if resource.Gateway != "" {
		parts = append(parts, resource.Gateway)
	}
	if resource.Metric != 0 {
		parts = append(parts, strconv.Itoa(resource.Metric))
	}
	value := strings.Join(parts, " ")
	if resource.Table != 0 {
		value += ", table=" + strconv.Itoa(resource.Table)
	}
	return value
}

func routeTextMatches(raw string, resource models.RouteResource) bool {
	for _, line := range strings.Split(raw, "\n") {
		if configuredRouteIdentity(line) == routeIdentity(resource) {
			return true
		}
	}
	return false
}

type networkManagerRouteIdentity struct {
	Destination string
	Gateway     string
	Metric      int
	Table       int
}

func routeIdentity(resource models.RouteResource) networkManagerRouteIdentity {
	return networkManagerRouteIdentity{
		Destination: resource.Destination,
		Gateway:     resource.Gateway,
		Metric:      resource.Metric,
		Table:       resource.Table,
	}
}

func configuredRouteIdentity(line string) networkManagerRouteIdentity {
	line = strings.Trim(strings.TrimSpace(line), "{}")
	if line == "" {
		return networkManagerRouteIdentity{}
	}
	identity := networkManagerRouteIdentity{}
	parts := strings.Split(line, ",")
	positional := strings.Fields(strings.TrimSpace(parts[0]))
	if len(positional) > 0 && !strings.Contains(parts[0], "=") {
		identity.Destination = positional[0]
		if len(positional) > 1 {
			identity.Gateway = positional[1]
		}
		if len(positional) > 2 {
			identity.Metric, _ = strconv.Atoi(positional[2])
		}
		parts = parts[1:]
	}
	for _, part := range parts {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "ip":
			identity.Destination = value
		case "nh":
			identity.Gateway = value
		case "mt":
			identity.Metric, _ = strconv.Atoi(value)
		case "table":
			identity.Table, _ = strconv.Atoi(value)
		}
	}
	return identity
}

func routeTable(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number int
	if json.Unmarshal(raw, &number) == nil {
		return number
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		switch name {
		case "main":
			return 254
		case "default":
			return 253
		case "local":
			return 255
		}
	}
	return -1
}
