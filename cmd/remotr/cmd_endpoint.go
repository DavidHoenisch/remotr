package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
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
		fmt.Println("ID\tFLEET\tFINGERPRINT\tLABELS")
	}
	for _, ep := range eps {
		labels := ""
		if len(ep.Labels) > 0 {
			labels = formatLabels(ep.Labels)
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", ep.ID, ep.Fleet, ep.CertFingerprint, labels)
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

func formatSystemInfoSummary(report json.RawMessage) []string {
	if len(report) == 0 {
		return nil
	}
	var snap struct {
		CPU struct {
			ModelName string `json:"modelName"`
			CoreCount string `json:"coreCount"`
		} `json:"cpu"`
		RAM struct {
			MemTotal string `json:"memTotal"`
		} `json:"ram"`
		BlockDevices []struct {
			Name string `json:"name"`
		} `json:"blockDevices"`
		TPM struct {
			Version string `json:"version"`
		} `json:"tpm"`
	}
	if err := json.Unmarshal(report, &snap); err != nil {
		return []string{fmt.Sprintf("report: %s", string(report))}
	}
	var lines []string
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
	if n := len(snap.BlockDevices); n > 0 {
		lines = append(lines, fmt.Sprintf("block_devices: %d", n))
	}
	if snap.TPM.Version != "" {
		lines = append(lines, "tpm: present (version "+snap.TPM.Version+")")
	} else {
		lines = append(lines, "tpm: not reported")
	}
	return lines
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
