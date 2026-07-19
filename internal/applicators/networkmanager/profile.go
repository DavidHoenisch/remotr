// Package networkmanager implements the audit-first NetworkManager profile
// provider through nmcli's non-secret observation surface.
package networkmanager

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type ProfileApplicator struct {
	Resource        models.NetworkProfileResource
	Runner          executil.Runner
	StateDir        string
	Now             func() time.Time
	AfterFunc       func(time.Duration, func())
	selectedDevice  Device
	devicePath      string
	rollbackTimeout time.Duration
}

type Device struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	PermanentMAC string `json:"permanentMAC"`
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
	CredentialDrift       bool                   `json:"credentialDrift,omitempty"`
	Acknowledged          bool                   `json:"acknowledged"`
	RollbackOutcome       string                 `json:"rollbackOutcome,omitempty"`
}

type ProfileConfiguredState struct {
	Compliant   bool     `json:"compliant"`
	Present     bool     `json:"present"`
	ProfileType string   `json:"profileType,omitempty"`
	AutoConnect bool     `json:"autoConnect,omitempty"`
	MTU         int      `json:"mtu,omitempty"`
	IPv4Method  string   `json:"ipv4Method,omitempty"`
	IPv6Method  string   `json:"ipv6Method,omitempty"`
	Addresses   []string `json:"addresses,omitempty"`
	SSID        string   `json:"ssid,omitempty"`
}

type ProfileEffectiveState struct {
	Compliant bool     `json:"compliant"`
	Connected bool     `json:"connected"`
	Addresses []string `json:"addresses,omitempty"`
}

func NewProfile(resource models.NetworkProfileResource, runner executil.Runner) *ProfileApplicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &ProfileApplicator{
		Resource: resource, Runner: runner, Now: time.Now,
		AfterFunc: func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) },
	}
}

func (a *ProfileApplicator) Name() string        { return "network-profile:" + a.Resource.Name }
func (a *ProfileApplicator) Description() string { return "network profile " + a.Resource.ProfileName }
func (a *ProfileApplicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

// Preflight requires both resource-level enforcement authorization and the
// durable state needed by the guarded activation transaction.
func (a *ProfileApplicator) Preflight(context.Context) error {
	if a.Resource.IsAudit() {
		return nil
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
	devices, err := a.devices()
	if err != nil {
		return err
	}
	matches := selectDevices(devices, a.Resource.Selector)
	if len(matches) != 1 {
		return fmt.Errorf("networkProfile %q selector matched %d interfaces", a.Resource.Name, len(matches))
	}
	stdout, _, err := a.Runner.Run("nmcli", "-g", "GENERAL.DBUS-PATH", "device", "show", matches[0].Name)
	if err != nil {
		return fmt.Errorf("resolve NetworkManager device object for %s: %w", matches[0].Name, err)
	}
	path := strings.TrimSpace(string(stdout))
	if !strings.HasPrefix(path, "/org/freedesktop/NetworkManager/Devices/") || strings.ContainsAny(path, " \t\r\n") {
		return fmt.Errorf("networkProfile %q received invalid NetworkManager device object", a.Resource.Name)
	}
	a.selectedDevice = matches[0]
	a.devicePath = path
	a.rollbackTimeout = timeout
	return nil
}

func (a *ProfileApplicator) PreflightRollback(ctx context.Context) error {
	if a.Resource.IsAudit() {
		return nil
	}
	if a.devicePath == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	store, err := networkstate.New(networkstate.Options{Root: a.StateDir, Runner: a.Runner, Now: a.now})
	if err != nil {
		return err
	}
	current, err := store.Status()
	if err != nil {
		return err
	}
	attempt := 1
	if current.Intent != nil {
		attempt = current.Intent.Attempt + 1
	}
	resourceJSON, err := json.Marshal(a.Resource)
	if err != nil {
		return err
	}
	resourceSum := sha256.Sum256(resourceJSON)
	planSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%s:%s:%s", resourceSum, a.selectedDevice.Name, a.devicePath, a.rollbackTimeout)))
	now := a.now()
	idSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%d:%d", resourceSum, attempt, now.UnixNano())))
	return store.Preflight(ctx, networkstate.Intent{
		ID: fmt.Sprintf("%x", idSum[:16]), Address: "networkProfile/" + a.Resource.Name,
		ArtifactDigest: fmt.Sprintf("sha256:%x", resourceSum), Attempt: attempt,
		Backend: "network-manager", Deadline: now.Add(a.rollbackTimeout),
		Checkpoint: "/org/freedesktop/NetworkManager/Checkpoint/preflight",
		PlanHash:   fmt.Sprintf("sha256:%x", planSum),
	})
}

func (a *ProfileApplicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("network profile " + a.Resource.Name)
	if err := ctx.Err(); err != nil {
		return profileFailed(desired, executor.ReasonProbeFailed, err)
	}
	if err := a.Resource.Validate(); err != nil {
		return profileFailed(desired, executor.ReasonProbeFailed, err)
	}
	devices, err := a.devices()
	if err != nil {
		return profileFailed(desired, executor.ReasonProbeFailed, err)
	}
	matches := selectDevices(devices, a.Resource.Selector)
	if len(matches) != 1 {
		reason := executor.ReasonCode("interface_not_found")
		if len(matches) > 1 {
			reason = "ambiguous_interface"
		}
		return profileFailed(desired, reason, fmt.Errorf("networkProfile %q selector matched %d interfaces", a.Resource.Name, len(matches)))
	}
	mode := "enforce"
	if a.Resource.IsAudit() {
		mode = "audit"
	}
	report := ProfileReport{Backend: a.Resource.Provider, Mode: mode, Interface: matches[0].Name, ProfileName: a.Resource.ProfileName}
	active, err := a.activeConnection(matches[0].Name)
	if err != nil {
		return profileFailed(desired, executor.ReasonProbeFailed, err)
	}
	report.Configured, report.CredentialFingerprint, err = a.configuredState()
	if err != nil {
		return profileFailed(desired, executor.ReasonProbeFailed, err)
	}
	report.Effective, err = a.effectiveState(matches[0].Name, active)
	if err != nil {
		return profileFailed(desired, executor.ReasonProbeFailed, err)
	}
	if a.Resource.CredentialRef != "" {
		report.CredentialReference = a.Resource.CredentialRef
		desiredFingerprint := credentialFingerprint(a.Resource.CredentialRef)
		report.CredentialDrift = report.CredentialFingerprint != desiredFingerprint
		report.CredentialFingerprint = desiredFingerprint
	}
	if report.Configured.Compliant && report.Effective.Compliant && !report.CredentialDrift {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, Actual: report}
	}
	reason := executor.ReasonCode(executor.ReasonStateDrift)
	observed := executor.RedactedSummary("configured, effective, or credential fingerprint state differs")
	if a.Resource.IsAudit() {
		reason = "audit_plan"
		observed = "audit-only; NetworkManager profile unchanged"
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: reason, DesiredSummary: desired, ObservedSummary: observed, Actual: report}
}

func (a *ProfileApplicator) Apply(ctx context.Context) error {
	if a.Resource.IsAudit() {
		return fmt.Errorf("networkProfile %q is audit-only; guarded enforcement is not enabled", a.Resource.Name)
	}
	_, met := a.State(ctx)
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.devicePath == "" {
		if err := a.Preflight(ctx); err != nil {
			return err
		}
	}
	store, err := a.prepareTransaction(ctx)
	if err != nil {
		return err
	}
	if err := a.mutateProfile(); err != nil {
		if _, rollbackErr := store.Rollback(ctx, "apply_failed"); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("NetworkManager checkpoint rollback failed: %w", rollbackErr))
		}
		return err
	}
	a.armRollbackWatchdog(store)
	return nil
}

func (a *ProfileApplicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if a.Resource.IsAudit() {
		if errors.Is(err, appErr.ErrStateAlreadyMet) {
			return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
		}
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

func (a *ProfileApplicator) Revert(ctx context.Context) error {
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

func (a *ProfileApplicator) prepareTransaction(ctx context.Context) (*networkstate.Store, error) {
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
	stdout, _, err := a.Runner.Run(
		"busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager",
		"org.freedesktop.NetworkManager", "CheckpointCreate", "aou", "1", a.devicePath,
		strconv.FormatInt(int64(a.rollbackTimeout/time.Second), 10), "0",
	)
	if err != nil {
		return nil, fmt.Errorf("create NetworkManager checkpoint: %w", err)
	}
	checkpoint := parseCheckpointObject(stdout)
	if checkpoint == "" {
		return nil, errors.New("create NetworkManager checkpoint: invalid object path")
	}
	resourceJSON, err := json.Marshal(a.Resource)
	if err != nil {
		return nil, err
	}
	resourceSum := sha256.Sum256(resourceJSON)
	planSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%s:%s:%s", resourceSum, a.selectedDevice.Name, a.devicePath, a.rollbackTimeout)))
	now := a.now()
	idSum := sha256.Sum256([]byte(fmt.Sprintf("%x:%d:%d", resourceSum, attempt, now.UnixNano())))
	_, err = store.Prepare(ctx, networkstate.Intent{
		ID: fmt.Sprintf("%x", idSum[:16]), Address: "networkProfile/" + a.Resource.Name,
		ArtifactDigest: fmt.Sprintf("sha256:%x", resourceSum), Attempt: attempt,
		Backend: "network-manager", Deadline: now.Add(a.rollbackTimeout), Checkpoint: checkpoint,
		PlanHash: fmt.Sprintf("sha256:%x", planSum),
	})
	if err != nil {
		_, _, _ = a.Runner.Run("busctl", "call", "org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager", "org.freedesktop.NetworkManager", "CheckpointDestroy", "o", checkpoint)
		return nil, err
	}
	return store, nil
}

func (a *ProfileApplicator) mutateProfile() error {
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if _, _, err := a.Runner.Run("nmcli", "connection", "delete", a.Resource.ProfileName); err != nil {
			return fmt.Errorf("delete NetworkManager profile %s: %w", a.Resource.ProfileName, err)
		}
		return nil
	}
	args := []string{"connection", "modify", a.Resource.ProfileName, "connection.interface-name", a.selectedDevice.Name}
	if a.Resource.AutoConnect != nil {
		autoConnect := "no"
		if *a.Resource.AutoConnect {
			autoConnect = "yes"
		}
		args = append(args, "connection.autoconnect", autoConnect)
	}
	if a.Resource.MTU != 0 {
		field := "802-3-ethernet.mtu"
		if a.Resource.ProfileType == models.NetworkProfileWiFi {
			field = "802-11-wireless.mtu"
		}
		args = append(args, field, strconv.Itoa(a.Resource.MTU))
	}
	if a.Resource.IPv4Method != "" {
		args = append(args, "ipv4.method", a.Resource.IPv4Method)
	}
	if a.Resource.IPv6Method != "" {
		args = append(args, "ipv6.method", a.Resource.IPv6Method)
	}
	var ipv4, ipv6 []string
	for _, address := range a.Resource.Addresses {
		prefix, _ := netip.ParsePrefix(address)
		if prefix.Addr().Is4() {
			ipv4 = append(ipv4, address)
		} else {
			ipv6 = append(ipv6, address)
		}
	}
	if len(a.Resource.Addresses) != 0 {
		args = append(args, "ipv4.addresses", strings.Join(ipv4, ","), "ipv6.addresses", strings.Join(ipv6, ","))
	}
	if a.Resource.SSID != "" {
		args = append(args, "802-11-wireless.ssid", a.Resource.SSID)
	}
	if a.Resource.CredentialRef != "" {
		args = append(args, "user.data", "remotr.credential="+credentialFingerprint(a.Resource.CredentialRef))
	}
	if _, _, err := a.Runner.Run("nmcli", args...); err != nil {
		return fmt.Errorf("modify NetworkManager profile %s: %w", a.Resource.ProfileName, err)
	}
	if _, _, err := a.Runner.Run("nmcli", "connection", "up", a.Resource.ProfileName, "ifname", a.selectedDevice.Name); err != nil {
		return fmt.Errorf("activate NetworkManager profile %s: %w", a.Resource.ProfileName, err)
	}
	return nil
}

func (a *ProfileApplicator) armRollbackWatchdog(store *networkstate.Store) {
	if store == nil || a.AfterFunc == nil {
		return
	}
	a.AfterFunc(a.rollbackTimeout, func() {
		_, _ = store.Reconcile(context.Background())
	})
}

func (a *ProfileApplicator) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now().UTC()
}

func parseCheckpointObject(raw []byte) string {
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

func (a *ProfileApplicator) devices() ([]Device, error) {
	stdout, _, err := a.Runner.Run("nmcli", "-t", "-f", "GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR", "device", "show")
	if err != nil {
		return nil, fmt.Errorf("enumerate NetworkManager interfaces: %w", err)
	}
	var devices []Device
	current := Device{}
	flush := func() {
		if current.Name != "" {
			devices = append(devices, current)
		}
		current = Device{}
	}
	for _, line := range strings.Split(string(stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch key {
		case "GENERAL.DEVICE":
			if current.Name != "" {
				flush()
			}
			current.Name = value
		case "GENERAL.TYPE":
			current.Type = normalizeDeviceType(value)
		case "GENERAL.HWADDR":
			current.PermanentMAC = strings.ToLower(value)
		}
	}
	flush()
	return devices, nil
}

func (a *ProfileApplicator) activeConnection(interfaceName string) (string, error) {
	stdout, _, err := a.Runner.Run("nmcli", "-t", "-f", "GENERAL.CONNECTION", "device", "show", interfaceName)
	if err != nil {
		return "", fmt.Errorf("observe active NetworkManager connection for %s: %w", interfaceName, err)
	}
	for _, line := range strings.Split(string(stdout), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && key == "GENERAL.CONNECTION" {
			return value, nil
		}
	}
	return "", nil
}

const profileFields = "connection.id,connection.type,connection.autoconnect,802-3-ethernet.mtu,802-11-wireless.mtu,ipv4.method,ipv6.method,ipv4.addresses,ipv6.addresses,802-11-wireless.ssid,user.data"

func (a *ProfileApplicator) configuredState() (ProfileConfiguredState, string, error) {
	stdout, _, err := a.Runner.Run("nmcli", "-t", "-f", profileFields, "connection", "show", a.Resource.ProfileName)
	if err != nil {
		if a.Resource.Lifecycle == models.LifecycleAbsent {
			return ProfileConfiguredState{Compliant: true}, "", nil
		}
		return ProfileConfiguredState{}, "", fmt.Errorf("observe NetworkManager profile %s: %w", a.Resource.ProfileName, err)
	}
	properties := safeProfileProperties(stdout)
	state := ProfileConfiguredState{
		Present: true, ProfileType: normalizeDeviceType(properties["connection.type"]),
		AutoConnect: properties["connection.autoconnect"] == "yes",
		IPv4Method:  properties["ipv4.method"], IPv6Method: properties["ipv6.method"],
		SSID: properties["802-11-wireless.ssid"],
	}
	state.MTU, _ = strconv.Atoi(firstNonEmpty(properties["802-3-ethernet.mtu"], properties["802-11-wireless.mtu"]))
	state.Addresses = normalizedProfileValues(append(csvValues(properties["ipv4.addresses"]), csvValues(properties["ipv6.addresses"])...))
	wantPresent := a.Resource.Lifecycle != models.LifecycleAbsent
	state.Compliant = state.Present == wantPresent
	if wantPresent {
		state.Compliant = properties["connection.id"] == a.Resource.ProfileName && state.ProfileType == a.Resource.ProfileType
		if a.Resource.AutoConnect != nil {
			state.Compliant = state.Compliant && state.AutoConnect == *a.Resource.AutoConnect
		}
		if a.Resource.MTU != 0 {
			state.Compliant = state.Compliant && state.MTU == a.Resource.MTU
		}
		if a.Resource.IPv4Method != "" {
			state.Compliant = state.Compliant && state.IPv4Method == a.Resource.IPv4Method
		}
		if a.Resource.IPv6Method != "" {
			state.Compliant = state.Compliant && state.IPv6Method == a.Resource.IPv6Method
		}
		if len(a.Resource.Addresses) != 0 {
			state.Compliant = state.Compliant && slices.Equal(state.Addresses, normalizedProfileValues(a.Resource.Addresses))
		}
		if a.Resource.SSID != "" {
			state.Compliant = state.Compliant && state.SSID == a.Resource.SSID
		}
	}
	return state, credentialMetadata(properties["user.data"]), nil
}

func (a *ProfileApplicator) effectiveState(interfaceName, activeConnection string) (ProfileEffectiveState, error) {
	stdout, _, err := a.Runner.Run("nmcli", "-t", "-f", "GENERAL.STATE,IP4.ADDRESS,IP6.ADDRESS", "device", "show", interfaceName)
	if err != nil {
		return ProfileEffectiveState{}, fmt.Errorf("observe effective NetworkManager profile state for %s: %w", interfaceName, err)
	}
	properties := safeProfileProperties(stdout)
	state := ProfileEffectiveState{
		Connected: strings.HasPrefix(properties["GENERAL.STATE"], "100"),
		Addresses: normalizedProfileValues(append(propertySeries(stdout, "IP4.ADDRESS"), propertySeries(stdout, "IP6.ADDRESS")...)),
	}
	wantPresent := a.Resource.Lifecycle != models.LifecycleAbsent
	if !wantPresent {
		state.Compliant = activeConnection != a.Resource.ProfileName
		return state, nil
	}
	state.Compliant = activeConnection == a.Resource.ProfileName && state.Connected
	if len(a.Resource.Addresses) != 0 {
		state.Compliant = state.Compliant && slices.Equal(state.Addresses, normalizedProfileValues(a.Resource.Addresses))
	}
	return state, nil
}

// safeProfileProperties allowlists non-secret fields. Unexpected fields such
// as a backend accidentally returning a PSK are deliberately discarded.
func safeProfileProperties(raw []byte) map[string]string {
	allowed := map[string]bool{
		"connection.id": true, "connection.type": true, "connection.autoconnect": true,
		"802-3-ethernet.mtu": true, "802-11-wireless.mtu": true,
		"ipv4.method": true, "ipv6.method": true, "ipv4.addresses": true, "ipv6.addresses": true,
		"802-11-wireless.ssid": true, "user.data": true, "GENERAL.STATE": true,
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && allowed[key] {
			values[key] = value
		}
	}
	return values
}

func propertySeries(raw []byte, prefix string) []string {
	var values []string
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.HasPrefix(key, prefix+"[") {
			values = append(values, value)
		}
	}
	return values
}

func credentialFingerprint(reference string) string {
	sum := sha256.Sum256([]byte(reference))
	return fmt.Sprintf("sha256:%x", sum[:16])
}

func credentialMetadata(userData string) string {
	for _, item := range strings.Split(userData, ",") {
		if value, found := strings.CutPrefix(strings.TrimSpace(item), "remotr.credential="); found {
			return value
		}
	}
	return ""
}

func csvValues(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" && value != "--" {
			values = append(values, value)
		}
	}
	return values
}

func normalizedProfileValues(values []string) []string {
	values = append([]string(nil), values...)
	sort.Strings(values)
	return slices.Compact(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" && value != "0" {
			return value
		}
	}
	return ""
}

func selectDevices(devices []Device, selector models.NetworkInterfaceSelector) []Device {
	var matches []Device
	for _, device := range devices {
		if selector.Name != "" && selector.Name != device.Name {
			continue
		}
		if selector.Type != "" && selector.Type != device.Type {
			continue
		}
		if selector.PermanentMAC != "" && !strings.EqualFold(selector.PermanentMAC, device.PermanentMAC) {
			continue
		}
		matches = append(matches, device)
	}
	return matches
}

func normalizeDeviceType(value string) string {
	switch strings.ToLower(value) {
	case "802-3-ethernet", "ethernet":
		return models.NetworkProfileEthernet
	case "802-11-wireless", "wifi", "wireless":
		return models.NetworkProfileWiFi
	default:
		return strings.ToLower(value)
	}
}

func profileFailed(desired executor.RedactedSummary, reason executor.ReasonCode, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: reason, DesiredSummary: desired, Err: err}
}
