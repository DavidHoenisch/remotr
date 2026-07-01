package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/endpointlabel"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/urfave/cli/v3"
)

func actionEndpointList(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint list: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint list: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if !opcreds.Present(settings.StateDir) {
		return exitErr(2, "endpoint list: operator credentials missing in %s (run remotr bootstrap first)", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint list: %v", err)
	}

	eps, err := client.ListEndpoints()
	if err != nil {
		return apiErr(c, "endpoint list", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(eps)
	}

	if len(eps) == 0 {
		writeInfoLine("no endpoints enrolled")
		return nil
	}

	if resolveFormat(c) == formatTable && !c.Bool("no-headers") {
		fmt.Println("ID\tFLEET\tUSERNAMES\tFINGERPRINT\tLABELS")
	}
	for _, ep := range eps {
		labels := ""
		if len(ep.Labels) > 0 {
			labels = formatLabels(ep.Labels)
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", ep.ID, ep.Fleet, formatUsernames(ep.Usernames), ep.CertFingerprint, labels)
	}
	return nil
}

func actionEndpointShow(_ context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "endpoint show")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint show: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint show: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if !opcreds.Present(settings.StateDir) {
		return errCredentialsMissing("endpoint show", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint show: %v", err)
	}

	ep, err := client.GetEndpoint(endpointID)
	if err != nil {
		return apiErr(c, "endpoint show", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(ep)
	}

	fmt.Printf("id: %s\n", ep.ID)
	fmt.Printf("fleet: %s\n", ep.Fleet)
	if len(ep.Usernames) > 0 {
		fmt.Printf("usernames: %s\n", formatUsernames(ep.Usernames))
	}
	if ep.CertFingerprint != "" {
		fmt.Printf("cert_fingerprint: %s\n", ep.CertFingerprint)
	}
	if len(ep.Labels) > 0 {
		fmt.Printf("labels: %s\n", formatLabels(ep.Labels))
	} else {
		fmt.Println("labels: (none)")
	}
	if ep.DesiredAgentVersion != "" {
		fmt.Printf("desired_agent_version: %s\n", ep.DesiredAgentVersion)
	}
	if ep.ReportedAgentVersion != "" {
		fmt.Printf("reported_agent_version: %s\n", ep.ReportedAgentVersion)
	}
	if ep.AgentUpgrade != nil {
		fmt.Printf("agent_upgrade:\n")
		if ep.AgentUpgrade.Desired != "" {
			fmt.Printf("  desired: %s\n", ep.AgentUpgrade.Desired)
		}
		if ep.AgentUpgrade.Phase != "" {
			fmt.Printf("  phase: %s\n", ep.AgentUpgrade.Phase)
		}
		if ep.AgentUpgrade.Message != "" {
			fmt.Printf("  message: %s\n", ep.AgentUpgrade.Message)
		}
		if !ep.AgentUpgrade.ReportedAt.IsZero() {
			fmt.Printf("  reported_at: %s\n", ep.AgentUpgrade.ReportedAt.UTC().Format(time.RFC3339))
		}
	}
	if ep.LastCheckIn != nil {
		fmt.Printf("last_check_in:\n")
		fmt.Printf("  release_ref: %s\n", ep.LastCheckIn.ReleaseRef)
		fmt.Printf("  digest: %s\n", ep.LastCheckIn.Digest)
		fmt.Printf("  at: %s\n", ep.LastCheckIn.At.UTC().Format(time.RFC3339))
	} else {
		fmt.Println("last_check_in: (none)")
	}
	if ep.LastDrift != nil {
		fmt.Printf("last_drift:\n")
		fmt.Printf("  release_ref: %s\n", ep.LastDrift.ReleaseRef)
		fmt.Printf("  digest: %s\n", ep.LastDrift.Digest)
		fmt.Printf("  reported_at: %s\n", ep.LastDrift.ReportedAt.UTC().Format(time.RFC3339))
	} else {
		fmt.Println("last_drift: (none)")
	}
	if ep.LastApplyFailure != nil {
		fmt.Printf("last_apply_failure:\n")
		fmt.Printf("  release_ref: %s\n", ep.LastApplyFailure.ReleaseRef)
		fmt.Printf("  resource_address: %s\n", ep.LastApplyFailure.ResourceAddress)
		fmt.Printf("  message: %s\n", ep.LastApplyFailure.Message)
		fmt.Printf("  reported_at: %s\n", ep.LastApplyFailure.ReportedAt.UTC().Format(time.RFC3339))
	} else {
		fmt.Println("last_apply_failure: (none)")
	}
	if ep.SystemInfo != nil {
		fmt.Printf("system_info:\n")
		fmt.Printf("  reported_at: %s\n", ep.SystemInfo.ReportedAt.UTC().Format(time.RFC3339))
		if ep.SystemInfo.Digest != "" {
			fmt.Printf("  digest: %s\n", ep.SystemInfo.Digest)
		}
		for _, line := range formatSystemInfoSummary(ep.SystemInfo.Report) {
			fmt.Printf("  %s\n", line)
		}
	} else {
		fmt.Println("system_info: (none)")
	}
	return nil
}

func actionEndpointRemove(_ context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "endpoint remove")
	if err != nil {
		return err
	}
	if err := requireConfirm(c, "endpoint remove", endpointID); err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint remove: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint remove: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if !opcreds.Present(settings.StateDir) {
		return exitErr(2, "endpoint remove: operator credentials missing in %s (run remotr bootstrap first)", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint remove: %v", err)
	}
	if err := client.RemoveEndpoint(endpointID); err != nil {
		return apiErr(c, "endpoint remove", err)
	}
	fmt.Printf("removed endpoint %s\n", endpointID)
	return nil
}

func actionEndpointAgentUpgrade(_ context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "endpoint agent upgrade")
	if err != nil {
		return err
	}
	ver, err := resolveVersion(c, "endpoint agent upgrade")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint agent upgrade: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint agent upgrade: server URL is required")
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint agent upgrade: %v", err)
	}
	if err := client.RequestEndpointAgentUpgrade(endpointID, ver); err != nil {
		return apiErr(c, "endpoint agent upgrade", err)
	}
	fmt.Printf("upgrade requested for %s to %s (applies on next sync)\n", endpointID, ver)
	return nil
}

type systemInfoNetworkSummary struct {
	Name       string
	MACAddress string
	IPv4       []string
	IPv6       []string
	Operstate  string
}

type systemInfoBatterySummary struct {
	Name          string
	Status        string
	Capacity      string
	CapacityLevel string
}

type systemInfoBlockDeviceSummary struct {
	Name           string
	Encrypted      bool
	EncryptionType string
}

func formatSystemInfoSummary(report json.RawMessage) []string {
	if len(report) == 0 {
		return nil
	}
	var snap struct {
		OSRelease struct {
			PrettyName string `json:"prettyName"`
			Name       string `json:"name"`
			VersionID  string `json:"versionId"`
		} `json:"osRelease"`
		CPU struct {
			ModelName string `json:"modelName"`
			CoreCount string `json:"coreCount"`
		} `json:"cpu"`
		RAM struct {
			MemTotal string `json:"memTotal"`
		} `json:"ram"`
		Networks []struct {
			Name       string   `json:"name"`
			MACAddress string   `json:"macAddress"`
			IPv4       []string `json:"ipv4"`
			IPv6       []string `json:"ipv6"`
			Statistics *struct {
				Operstate string `json:"operstate"`
			} `json:"statistics"`
		} `json:"networks"`
		Batteries []struct {
			Name          string `json:"name"`
			Status        string `json:"status"`
			Capacity      string `json:"capacity"`
			CapacityLevel string `json:"capacityLevel"`
		} `json:"batteries"`
		BlockDevices []struct {
			Name           string `json:"name"`
			Encrypted      bool   `json:"encrypted"`
			EncryptionType string `json:"encryptionType"`
		} `json:"blockDevices"`
		TPM struct {
			Version string `json:"version"`
		} `json:"tpm"`
	}
	if err := json.Unmarshal(report, &snap); err != nil {
		return []string{fmt.Sprintf("report: %s", string(report))}
	}
	var lines []string
	if osLine := formatOSReleaseLine(snap.OSRelease.PrettyName, snap.OSRelease.Name, snap.OSRelease.VersionID); osLine != "" {
		lines = append(lines, osLine)
	}
	if snap.CPU.ModelName != "" {
		line := "cpu: " + snap.CPU.ModelName
		if snap.CPU.CoreCount != "" {
			line += " (" + snap.CPU.CoreCount + " cores)"
		}
		lines = append(lines, line)
	}
	if snap.RAM.MemTotal != "" {
		lines = append(lines, "ram: "+snap.RAM.MemTotal)
	}
	for _, net := range snap.Networks {
		if net.Name == "lo" {
			continue
		}
		if net.MACAddress == "" && len(net.IPv4) == 0 && len(net.IPv6) == 0 {
			continue
		}
		summary := systemInfoNetworkSummary{
			Name:       net.Name,
			MACAddress: net.MACAddress,
			IPv4:       net.IPv4,
			IPv6:       net.IPv6,
		}
		if net.Statistics != nil {
			summary.Operstate = net.Statistics.Operstate
		}
		if line := formatNetworkSummaryLine(summary); line != "" {
			lines = append(lines, line)
		}
	}
	for _, bat := range snap.Batteries {
		if line := formatBatterySummaryLine(systemInfoBatterySummary{
			Name:          bat.Name,
			Status:        bat.Status,
			Capacity:      bat.Capacity,
			CapacityLevel: bat.CapacityLevel,
		}); line != "" {
			lines = append(lines, line)
		}
	}
	if len(snap.BlockDevices) > 0 {
		sort.Slice(snap.BlockDevices, func(i, j int) bool {
			return snap.BlockDevices[i].Name < snap.BlockDevices[j].Name
		})
		encrypted := 0
		for _, dev := range snap.BlockDevices {
			if dev.Encrypted {
				encrypted++
			}
			if line := formatBlockDeviceSummaryLine(systemInfoBlockDeviceSummary{
				Name:           dev.Name,
				Encrypted:      dev.Encrypted,
				EncryptionType: dev.EncryptionType,
			}); line != "" {
				lines = append(lines, line)
			}
		}
		lines = append(lines, fmt.Sprintf("disk_encryption: %d/%d devices encrypted", encrypted, len(snap.BlockDevices)))
	}
	if snap.TPM.Version != "" {
		lines = append(lines, "tpm: present (version "+snap.TPM.Version+")")
	} else {
		lines = append(lines, "tpm: not reported")
	}
	return lines
}

func formatOSReleaseLine(prettyName, name, versionID string) string {
	switch {
	case prettyName != "":
		return "os: " + prettyName
	case name != "" && versionID != "":
		return "os: " + name + " " + versionID
	case name != "":
		return "os: " + name
	default:
		return ""
	}
}

func formatNetworkSummaryLine(net systemInfoNetworkSummary) string {
	parts := []string{"network " + net.Name + ":"}
	if net.MACAddress != "" {
		parts = append(parts, "mac="+net.MACAddress)
	}
	if len(net.IPv4) > 0 {
		parts = append(parts, "ipv4="+strings.Join(net.IPv4, ","))
	}
	if len(net.IPv6) > 0 {
		parts = append(parts, "ipv6="+strings.Join(net.IPv6, ","))
	}
	if net.Operstate != "" {
		parts = append(parts, "operstate="+net.Operstate)
	}
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts, " ")
}

func formatBlockDeviceSummaryLine(dev systemInfoBlockDeviceSummary) string {
	if dev.Name == "" {
		return ""
	}
	if dev.Encrypted {
		if dev.EncryptionType != "" {
			return fmt.Sprintf("block_device %s: encrypted (%s)", dev.Name, dev.EncryptionType)
		}
		return fmt.Sprintf("block_device %s: encrypted", dev.Name)
	}
	return fmt.Sprintf("block_device %s: not encrypted", dev.Name)
}

func formatBatterySummaryLine(bat systemInfoBatterySummary) string {
	if bat.Name == "" {
		return ""
	}
	parts := []string{"battery " + bat.Name + ":"}
	if bat.Capacity != "" {
		parts = append(parts, bat.Capacity+"%")
	} else if bat.CapacityLevel != "" {
		parts = append(parts, bat.CapacityLevel)
	}
	if bat.Status != "" {
		parts = append(parts, "("+bat.Status+")")
	}
	return strings.Join(parts, " ")
}

func formatUsernames(usernames []string) string {
	return strings.Join(usernames, ",")
}

func formatLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}

func parseLabelPair(s string) (key, value string, ok bool) {
	i := strings.Index(s, "=")
	if i <= 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func resolveEndpointLabelPair(c *cli.Command, cmd string) (key, value string, err error) {
	key = strings.TrimSpace(c.String("key"))
	value = c.String("value")
	keyFromFlag := key != ""
	valueFromFlag := c.IsSet("value")

	if !keyFromFlag {
		if extra := endpointLabelArgs(c); len(extra) > 0 {
			var ok bool
			key, value, ok = parseLabelPair(extra[0])
			if !ok {
				return "", "", exitErr(2, "%s: expected key=value label (got %q)", cmd, extra[0])
			}
			return key, value, nil
		}
	}

	if keyFromFlag && valueFromFlag {
		return key, value, nil
	}

	if isInteractive() {
		if err := promptEndpointLabelPairFields(&key, &value, keyFromFlag, valueFromFlag); err != nil {
			return "", "", exitErr(2, "%s: %v", cmd, err)
		}
		key = strings.TrimSpace(key)
		if err := endpointlabel.ValidateKey(key); err != nil {
			return "", "", exitErr(2, "%s: %v", cmd, err)
		}
		if err := endpointlabel.ValidateValue(value); err != nil {
			return "", "", exitErr(2, "%s: %v", cmd, err)
		}
		return key, value, nil
	}

	if !keyFromFlag {
		return "", "", exitErr(2, "%s: provide key=value or --key and --value", cmd)
	}
	return key, value, nil
}

func resolveEndpointLabelKey(c *cli.Command, cmd string, existingLabels map[string]string) (string, error) {
	key := strings.TrimSpace(c.String("key"))
	if key != "" {
		return key, nil
	}
	if extra := endpointLabelArgs(c); len(extra) > 0 {
		return extra[0], nil
	}
	if isInteractive() {
		labelPicker := len(existingLabels) > 0
		if err := promptEndpointLabelKey(&key, existingLabels); err != nil {
			return "", exitErr(2, "%s: %v", cmd, err)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return "", exitErr(2, "%s: label key required", cmd)
		}
		if labelPicker {
			replayActivate(c)
			replayAddPositional(key)
		}
		return key, nil
	}
	return "", exitErr(2, "%s: label key required (--key or positional argument)", cmd)
}

func actionEndpointLabelSet(_ context.Context, c *cli.Command) error {
	endpointIDs, err := resolveEndpointIDs(c, "endpoint label set")
	if err != nil {
		return err
	}
	key, value, err := resolveEndpointLabelPair(c, "endpoint label set")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint label set: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint label set: server URL is required")
	}
	if !opcreds.Present(settings.StateDir) {
		return errCredentialsMissing("endpoint label set", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint label set: %v", err)
	}
	for _, endpointID := range endpointIDs {
		resp, err := client.SetEndpointLabel(endpointID, key, value)
		if err != nil {
			return apiErr(c, "endpoint label set", err)
		}
		fmt.Printf("set %s on %s to %s\n", key, endpointID, value)
		if len(resp.Labels) > 0 {
			fmt.Printf("  labels: %s\n", formatLabels(resp.Labels))
		}
	}
	return nil
}

func actionEndpointLabelUnset(_ context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "endpoint label unset")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint label unset: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint label unset: server URL is required")
	}
	if !opcreds.Present(settings.StateDir) {
		return errCredentialsMissing("endpoint label unset", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint label unset: %v", err)
	}
	ep, err := client.GetEndpoint(endpointID)
	if err != nil {
		return apiErr(c, "endpoint label unset", err)
	}
	if len(ep.Labels) == 0 {
		return exitErr(1, "endpoint label unset: no labels on %s", endpointID)
	}

	key, err := resolveEndpointLabelKey(c, "endpoint label unset", ep.Labels)
	if err != nil {
		return err
	}

	if err := client.DeleteEndpointLabel(endpointID, key); err != nil {
		return apiErr(c, "endpoint label unset", err)
	}
	fmt.Printf("removed label %s from %s\n", key, endpointID)
	return nil
}

func actionEndpointLabelList(_ context.Context, c *cli.Command) error {
	endpointID, err := resolveEndpointID(c, "endpoint label list")
	if err != nil {
		return err
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint label list: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint label list: server URL is required")
	}
	if !opcreds.Present(settings.StateDir) {
		return errCredentialsMissing("endpoint label list", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint label list: %v", err)
	}
	ep, err := client.GetEndpoint(endpointID)
	if err != nil {
		return apiErr(c, "endpoint label list", err)
	}

	if resolveFormat(c) == formatJSON {
		return encodeJSON(ep.Labels)
	}
	if len(ep.Labels) == 0 {
		writeInfoLine("no labels")
		return nil
	}
	if resolveFormat(c) == formatTable && !c.Bool("no-headers") {
		fmt.Println("KEY\tVALUE")
	}
	keys := make([]string, 0, len(ep.Labels))
	for k := range ep.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s\t%s\n", k, ep.Labels[k])
	}
	return nil
}
