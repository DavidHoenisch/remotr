// Package networkfiles implements guarded netplan and systemd-networkd
// profile providers with separate persistent and effective observations.
package networkfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type Applicator struct {
	Resource        models.NetworkProfileResource
	Runner          executil.Runner
	ConfigDir       string
	StateDir        string
	Now             func() time.Time
	AfterFunc       func(time.Duration, func())
	selected        string
	rollbackTimeout time.Duration
}

type ProfileReport struct {
	Backend               string                 `json:"backend"`
	Mode                  string                 `json:"mode"`
	Interface             string                 `json:"interface,omitempty"`
	ProfileName           string                 `json:"profileName"`
	Configured            ProfileConfiguredState `json:"configured"`
	Effective             ProfileEffectiveState  `json:"effective"`
	CredentialReference   string                 `json:"credentialReference,omitempty"`
	CredentialFingerprint string                 `json:"credentialFingerprint,omitempty"`
	Acknowledged          bool                   `json:"acknowledged"`
	RollbackOutcome       string                 `json:"rollbackOutcome,omitempty"`
}

type ProfileConfiguredState struct {
	Compliant bool   `json:"compliant"`
	Present   bool   `json:"present"`
	Path      string `json:"path"`
}

type ProfileEffectiveState struct {
	Compliant bool     `json:"compliant"`
	Connected bool     `json:"connected"`
	Addresses []string `json:"addresses,omitempty"`
}

type linkState struct {
	IfName    string `json:"ifname"`
	Address   string `json:"address"`
	PermAddr  string `json:"permaddr"`
	LinkType  string `json:"link_type"`
	OperState string `json:"operstate"`
	AddrInfo  []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	} `json:"addr_info"`
}

func New(resource models.NetworkProfileResource, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	configDir := "/etc/netplan"
	if resource.Provider == models.NetworkProviderSystemdNetworkd {
		configDir = "/etc/systemd/network"
	}
	return &Applicator{
		Resource: resource, Runner: runner, ConfigDir: configDir, Now: time.Now,
		AfterFunc: func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) },
	}
}

func (a *Applicator) Name() string { return "network-profile:" + a.Resource.Name }
func (a *Applicator) Description() string {
	return fmt.Sprintf("%s network profile %s", a.Resource.Provider, a.Resource.ProfileName)
}
func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (a *Applicator) Preflight(context.Context) error {
	if a.Resource.IsAudit() {
		return nil
	}
	if err := a.Resource.Validate(); err != nil {
		return err
	}
	if a.Resource.Enforce == nil || !*a.Resource.Enforce {
		return fmt.Errorf("networkProfile %q requires explicit enforce authorization", a.Resource.Name)
	}
	if strings.TrimSpace(a.StateDir) == "" {
		return fmt.Errorf("networkProfile %q requires agent stateDir for timed rollback", a.Resource.Name)
	}
	timeout, err := time.ParseDuration(a.Resource.RollbackTimeout)
	if err != nil || timeout < 30*time.Second || timeout > 15*time.Minute {
		return fmt.Errorf("networkProfile %q rollbackTimeout must be between 30s and 15m", a.Resource.Name)
	}
	if !filepath.IsAbs(a.ConfigDir) || filepath.Clean(a.ConfigDir) != a.ConfigDir {
		return fmt.Errorf("networkProfile %q configuration directory is invalid", a.Resource.Name)
	}
	switch a.Resource.Provider {
	case models.NetworkProviderNetplan:
		if _, _, err := a.Runner.Run("netplan", "--help"); err != nil {
			return fmt.Errorf("networkProfile %q netplan provider unavailable: %w", a.Resource.Name, err)
		}
	case models.NetworkProviderSystemdNetworkd:
		if _, _, err := a.Runner.Run("networkctl", "--version"); err != nil {
			return fmt.Errorf("networkProfile %q systemd-networkd provider unavailable: %w", a.Resource.Name, err)
		}
	}
	links, err := a.links()
	if err != nil {
		return err
	}
	matches := selectLinks(links, a.Resource.Selector)
	if len(matches) != 1 {
		return fmt.Errorf("networkProfile %q selector matched %d interfaces", a.Resource.Name, len(matches))
	}
	a.selected = matches[0].IfName
	a.rollbackTimeout = timeout
	return nil
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("network profile " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return checkFailed(desired, executor.ReasonProbeFailed, err)
	}
	if err := a.Resource.Validate(); err != nil {
		return checkFailed(desired, executor.ReasonProbeFailed, err)
	}
	links, err := a.links()
	if err != nil {
		return checkFailed(desired, executor.ReasonProbeFailed, err)
	}
	matches := selectLinks(links, a.Resource.Selector)
	if len(matches) != 1 {
		reason := executor.ReasonCode("interface_not_found")
		if len(matches) > 1 {
			reason = "ambiguous_interface"
		}
		return checkFailed(desired, reason, fmt.Errorf("networkProfile %q selector matched %d interfaces", a.Resource.Name, len(matches)))
	}
	mode := "enforce"
	if a.Resource.IsAudit() {
		mode = "audit"
	}
	report := ProfileReport{Backend: a.Resource.Provider, Mode: mode, Interface: matches[0].IfName, ProfileName: a.Resource.ProfileName}
	report.Configured, err = a.configuredState(matches[0].IfName)
	if err != nil {
		return checkFailed(desired, executor.ReasonProbeFailed, err)
	}
	report.Effective, err = a.effectiveState(matches[0].IfName)
	if err != nil {
		return checkFailed(desired, executor.ReasonProbeFailed, err)
	}
	if a.Resource.CredentialRef != "" {
		report.CredentialReference = a.Resource.CredentialRef
		report.CredentialFingerprint = credentialFingerprint(a.Resource.CredentialRef)
	}
	if report.Configured.Compliant && report.Effective.Compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, Actual: report}
	}
	reason := executor.ReasonCode(executor.ReasonStateDrift)
	observed := executor.RedactedSummary("configured or effective network profile state differs")
	if a.Resource.IsAudit() {
		reason = "audit_plan"
		observed = "audit-only; network profile unchanged"
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: reason, DesiredSummary: desired, ObservedSummary: observed, Actual: report}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if a.Resource.IsAudit() {
		return fmt.Errorf("networkProfile %q is audit-only; guarded enforcement is not enabled", a.Resource.Name)
	}
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.selected == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	store, err := a.prepareTransaction(ctx)
	if err != nil {
		return err
	}
	if err := a.mutate(); err != nil {
		if _, rollbackErr := store.Rollback(ctx, "apply_failed"); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("%s rollback failed: %w", a.Resource.Provider, rollbackErr))
		}
		return err
	}
	a.armRollbackWatchdog(store)
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if a.Resource.IsAudit() {
		return executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired, Err: err}
	}
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired}
	}
	if err == nil {
		return executor.ApplyResult{Status: executor.Changed, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired}
	}
	if errors.Is(err, networkstate.ErrAwaitingAcknowledgement) {
		return executor.ApplyResult{
			Status: executor.ApplyDeferred, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired,
			DeferredWork: &executor.DeferredWork{ReasonCode: executor.ReasonDeferred, Summary: "another connectivity transaction is awaiting authenticated acknowledgement"},
		}
	}
	result := executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackTransactional, RebootRequired: executor.RebootNotRequired, Err: err}
	if store, storeErr := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Runner, Now: a.now}); storeErr == nil {
		if status, statusErr := store.Status(); statusErr == nil && status.Intent != nil && status.Intent.Phase == networkstate.PhaseRolledBack {
			result.Rollback = &executor.RollbackResult{Status: executor.Reverted}
		}
	}
	if result.Rollback == nil {
		result.Rollback = &executor.RollbackResult{Status: executor.NoRollback}
	}
	return result
}

func (a *Applicator) Revert(ctx context.Context) error {
	if a.Resource.IsAudit() {
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
	if status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement {
		return appErr.ErrNoOp
	}
	_, err = store.Rollback(ctx, "executor_revert")
	return err
}

func (a *Applicator) prepareTransaction(ctx context.Context) (*networkstate.Store, error) {
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Runner, Now: a.now})
	if err != nil {
		return nil, err
	}
	current, err := store.Status()
	if err != nil {
		return nil, err
	}
	if current.Intent != nil && current.Intent.Phase == networkstate.PhaseAwaitingAcknowledgement {
		return nil, fmt.Errorf("%w: %s", networkstate.ErrAwaitingAcknowledgement, current.Intent.ID)
	}
	attempt := 1
	if current.Intent != nil {
		attempt = current.Intent.Attempt + 1
	}
	path := a.path()
	snapshot, err := os.ReadFile(path)
	existed := err == nil
	mode := os.FileMode(0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("snapshot %s profile: %w", a.Resource.Provider, err)
	}
	if existed {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		mode = info.Mode().Perm()
	}
	resourceJSON, err := json.Marshal(a.Resource)
	if err != nil {
		return nil, err
	}
	resourceSum := sha256.Sum256(resourceJSON)
	planSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%s:%s:%s", resourceSum, a.selected, path, a.rollbackTimeout)))
	now := a.now()
	idSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%d:%d", resourceSum, attempt, now.UnixNano())))
	_, err = store.Prepare(ctx, networkstate.Intent{
		ID: fmt.Sprintf("%x", idSum[:16]), Address: "networkProfile/" + a.Resource.Name,
		ArtifactDigest: fmt.Sprintf("sha256:%x", resourceSum), Attempt: attempt,
		Backend: a.Resource.Provider, Deadline: now.Add(a.rollbackTimeout), PlanHash: fmt.Sprintf("sha256:%x", planSum),
		Snapshot: snapshot, RestorePath: path, RestoreExisted: existed, RestoreMode: uint32(mode), Interface: a.selected,
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func (a *Applicator) mutate() error {
	path := a.path()
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s profile: %w", a.Resource.Provider, err)
		}
	} else if err := writeAtomic(path, a.render(a.selected), 0o600); err != nil {
		return fmt.Errorf("write %s profile: %w", a.Resource.Provider, err)
	}
	switch a.Resource.Provider {
	case models.NetworkProviderNetplan:
		if _, _, err := a.Runner.Run("netplan", "generate"); err != nil {
			return fmt.Errorf("validate netplan configuration: %w", err)
		}
		if _, _, err := a.Runner.Run("netplan", "apply"); err != nil {
			return fmt.Errorf("apply netplan configuration: %w", err)
		}
	case models.NetworkProviderSystemdNetworkd:
		if _, _, err := a.Runner.Run("networkctl", "reload"); err != nil {
			return fmt.Errorf("reload systemd-networkd configuration: %w", err)
		}
		if _, _, err := a.Runner.Run("networkctl", "reconfigure", a.selected); err != nil {
			return fmt.Errorf("reconfigure systemd-networkd interface: %w", err)
		}
	}
	return nil
}

func (a *Applicator) armRollbackWatchdog(store *networkstate.Store) {
	if store == nil || a.AfterFunc == nil {
		return
	}
	a.AfterFunc(a.rollbackTimeout, func() {
		_, _ = store.Reconcile(context.Background())
	})
}

func (a *Applicator) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func (a *Applicator) links() ([]linkState, error) {
	stdout, _, err := a.Runner.Run("ip", "-json", "link", "show")
	if err != nil {
		return nil, fmt.Errorf("enumerate network interfaces: %w", err)
	}
	var links []linkState
	if err := json.Unmarshal(stdout, &links); err != nil {
		return nil, fmt.Errorf("decode network interfaces: %w", err)
	}
	return links, nil
}

func (a *Applicator) configuredState(interfaceName string) (ProfileConfiguredState, error) {
	path := a.path()
	actual, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ProfileConfiguredState{Compliant: a.Resource.Lifecycle == models.LifecycleAbsent, Path: path}, nil
	}
	if err != nil {
		return ProfileConfiguredState{}, fmt.Errorf("read %s profile: %w", a.Resource.Provider, err)
	}
	desired := a.render(interfaceName)
	present := true
	compliant := a.Resource.Lifecycle != models.LifecycleAbsent && bytes.Equal(actual, desired)
	return ProfileConfiguredState{Compliant: compliant, Present: present, Path: path}, nil
}

func (a *Applicator) effectiveState(interfaceName string) (ProfileEffectiveState, error) {
	stdout, _, err := a.Runner.Run("ip", "-json", "address", "show", "dev", interfaceName)
	if err != nil {
		return ProfileEffectiveState{}, fmt.Errorf("observe effective network state for %s: %w", interfaceName, err)
	}
	var links []linkState
	if err := json.Unmarshal(stdout, &links); err != nil || len(links) != 1 {
		return ProfileEffectiveState{}, fmt.Errorf("decode effective network state for %s", interfaceName)
	}
	state := ProfileEffectiveState{Connected: strings.EqualFold(links[0].OperState, "up")}
	for _, address := range links[0].AddrInfo {
		parsed, err := netip.ParseAddr(address.Local)
		if err == nil && (address.Family == "inet" || address.Family == "inet6") {
			state.Addresses = append(state.Addresses, netip.PrefixFrom(parsed, address.PrefixLen).String())
		}
	}
	sort.Strings(state.Addresses)
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		state.Compliant = true
		return state, nil
	}
	state.Compliant = state.Connected
	if len(a.Resource.Addresses) != 0 {
		desired := append([]string(nil), a.Resource.Addresses...)
		sort.Strings(desired)
		state.Compliant = state.Compliant && slices.Equal(state.Addresses, desired)
	}
	return state, nil
}

func (a *Applicator) path() string {
	extension := ".yaml"
	if a.Resource.Provider == models.NetworkProviderSystemdNetworkd {
		extension = ".network"
	}
	return filepath.Join(a.ConfigDir, "90-remotr-"+a.Resource.Name+extension)
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-network-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (a *Applicator) render(interfaceName string) []byte {
	if a.Resource.Provider == models.NetworkProviderSystemdNetworkd {
		return renderNetworkd(a.Resource, interfaceName)
	}
	return renderNetplan(a.Resource, interfaceName)
}

func renderNetplan(resource models.NetworkProfileResource, interfaceName string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# managed by remotr: %s\nnetwork:\n  version: 2\n", resource.Name)
	section := "ethernets"
	if resource.ProfileType == models.NetworkProfileWiFi {
		section = "wifis"
	}
	fmt.Fprintf(&out, "  %s:\n    %s:\n      match:\n        name: %s\n", section, interfaceName, interfaceName)
	if resource.AutoConnect != nil && !*resource.AutoConnect {
		out.WriteString("      activation-mode: manual\n")
	}
	if resource.MTU != 0 {
		fmt.Fprintf(&out, "      mtu: %d\n", resource.MTU)
	}
	if resource.IPv4Method == "auto" {
		out.WriteString("      dhcp4: true\n")
	} else if resource.IPv4Method == "disabled" {
		out.WriteString("      dhcp4: false\n")
	}
	if resource.IPv6Method == "auto" {
		out.WriteString("      dhcp6: true\n")
	} else if resource.IPv6Method == "disabled" || resource.IPv6Method == "ignore" {
		out.WriteString("      dhcp6: false\n")
	}
	if len(resource.Addresses) != 0 {
		out.WriteString("      addresses:\n")
		for _, address := range resource.Addresses {
			fmt.Fprintf(&out, "        - %s\n", address)
		}
	}
	if resource.SSID != "" {
		fmt.Fprintf(&out, "      access-points:\n        %q: {}\n", resource.SSID)
	}
	return []byte(out.String())
}

func renderNetworkd(resource models.NetworkProfileResource, interfaceName string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# managed by remotr: %s\n[Match]\nName=%s\n", resource.Name, interfaceName)
	if resource.Selector.PermanentMAC != "" {
		fmt.Fprintf(&out, "PermanentMACAddress=%s\n", strings.ToLower(resource.Selector.PermanentMAC))
	}
	if resource.MTU != 0 || resource.AutoConnect != nil {
		out.WriteString("\n[Link]\n")
		if resource.AutoConnect != nil {
			policy := "manual"
			if *resource.AutoConnect {
				policy = "up"
			}
			fmt.Fprintf(&out, "ActivationPolicy=%s\n", policy)
		}
		if resource.MTU != 0 {
			fmt.Fprintf(&out, "MTUBytes=%d\n", resource.MTU)
		}
	}
	out.WriteString("\n[Network]\n")
	for _, address := range resource.Addresses {
		fmt.Fprintf(&out, "Address=%s\n", address)
	}
	dhcp := ""
	if resource.IPv4Method == "auto" && resource.IPv6Method == "auto" {
		dhcp = "yes"
	} else if resource.IPv4Method == "auto" {
		dhcp = "ipv4"
	} else if resource.IPv6Method == "auto" {
		dhcp = "ipv6"
	}
	if dhcp != "" {
		fmt.Fprintf(&out, "DHCP=%s\n", dhcp)
	}
	return []byte(out.String())
}

func selectLinks(links []linkState, selector models.NetworkInterfaceSelector) []linkState {
	var matches []linkState
	for _, link := range links {
		linkType := models.NetworkProfileEthernet
		if strings.HasPrefix(link.IfName, "wl") || link.LinkType == "wlan" {
			linkType = models.NetworkProfileWiFi
		}
		if selector.Name != "" && selector.Name != link.IfName {
			continue
		}
		permanentMAC := link.PermAddr
		if permanentMAC == "" {
			permanentMAC = link.Address
		}
		if selector.PermanentMAC != "" && !strings.EqualFold(selector.PermanentMAC, permanentMAC) {
			continue
		}
		if selector.Type != "" && selector.Type != linkType {
			continue
		}
		matches = append(matches, link)
	}
	return matches
}

func credentialFingerprint(reference string) string {
	sum := sha256.Sum256([]byte(reference))
	return fmt.Sprintf("sha256:%x", sum[:16])
}

func checkFailed(desired executor.RedactedSummary, reason executor.ReasonCode, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: reason, DesiredSummary: desired, Err: err}
}
