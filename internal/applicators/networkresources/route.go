package networkresources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type RouteApplicator struct {
	Resource models.RouteResource
	Runner   executil.Runner
}

type RouteObservedScope struct {
	Managed   bool `json:"managed"`
	Compliant bool `json:"compliant"`
}

type RouteStateReport struct {
	Backend    string             `json:"backend"`
	Configured RouteObservedScope `json:"configured"`
	Effective  RouteObservedScope `json:"effective"`
}

func NewRoute(resource models.RouteResource, runner executil.Runner) *RouteApplicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &RouteApplicator{Resource: resource, Runner: runner}
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
	if err := a.Resource.Validate(); err != nil {
		return networkCheckFailed(desired, err)
	}
	report := RouteStateReport{Backend: a.Resource.Provider}
	var err error
	if a.Resource.Configured {
		connection, connectionErr := networkManagerConnection(a.Runner, a.Resource.Interface)
		if connectionErr != nil {
			return networkCheckFailed(desired, connectionErr)
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
		return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
}

func (a *RouteApplicator) Revert(context.Context) error { return appErr.ErrNoOp }

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
		args = append(args, "table", strconv.Itoa(a.Resource.Table))
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
	family := []string{}
	if destination, _ := netip.ParsePrefix(a.Resource.Destination); destination.Addr().Is6() {
		family = append(family, "-6")
	}
	operation := "replace"
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		operation = "del"
	}
	args := append(family, "route", operation, a.Resource.Destination)
	if a.Resource.Gateway != "" {
		args = append(args, "via", a.Resource.Gateway)
	}
	args = append(args, "dev", a.Resource.Interface)
	if a.Resource.Metric != 0 {
		args = append(args, "metric", strconv.Itoa(a.Resource.Metric))
	}
	if a.Resource.Table != 0 {
		args = append(args, "table", strconv.Itoa(a.Resource.Table))
	}
	_, stderr, err := a.Runner.Run("ip", args...)
	if err != nil {
		return fmt.Errorf("change effective route %s: %s: %w", a.Resource.Destination, boundedDiagnostic(stderr), err)
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
		if strings.Contains(line, resource.Destination) && (resource.Gateway == "" || strings.Contains(line, resource.Gateway)) &&
			(resource.Metric == 0 || strings.Contains(line, strconv.Itoa(resource.Metric))) &&
			(resource.Table == 0 || strings.Contains(line, "table="+strconv.Itoa(resource.Table))) {
			return true
		}
	}
	return false
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
