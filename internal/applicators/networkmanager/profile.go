// Package networkmanager implements the audit-first NetworkManager profile
// provider through nmcli's non-secret observation surface.
package networkmanager

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

type ProfileApplicator struct {
	Resource models.NetworkProfileResource
	Runner   executil.Runner
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
	return &ProfileApplicator{Resource: resource, Runner: runner}
}

func (a *ProfileApplicator) Name() string        { return "network-profile:" + a.Resource.Name }
func (a *ProfileApplicator) Description() string { return "network profile " + a.Resource.ProfileName }
func (a *ProfileApplicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
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

func (a *ProfileApplicator) Apply(context.Context) error {
	if a.Resource.IsAudit() {
		return fmt.Errorf("networkProfile %q is audit-only; guarded enforcement is not enabled", a.Resource.Name)
	}
	return fmt.Errorf("networkProfile %q guarded enforcement is not implemented", a.Resource.Name)
}

func (a *ProfileApplicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired}
	}
	return executor.ApplyResult{Status: executor.Failed, RollbackClass: executor.RollbackNone, RebootRequired: executor.RebootNotRequired, Err: err}
}

func (a *ProfileApplicator) Revert(context.Context) error { return appErr.ErrNoOp }

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
