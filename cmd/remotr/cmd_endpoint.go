package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/urfave/cli/v2"
)

func actionEndpointList(c *cli.Context) error {
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
		return exitErr(1, "endpoint list: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(eps); err != nil {
			return exitErr(1, "endpoint list: %v", err)
		}
		return nil
	}

	if len(eps) == 0 {
		fmt.Println("no endpoints enrolled")
		return nil
	}
	for _, ep := range eps {
		fmt.Printf("%s\tfleet=%s", ep.ID, ep.Fleet)
		if ep.CertFingerprint != "" {
			fmt.Printf("\tfp=%s", ep.CertFingerprint)
		}
		if len(ep.Labels) > 0 {
			fmt.Printf("\tlabels=%s", formatLabels(ep.Labels))
		}
		fmt.Println()
	}
	return nil
}

func actionEndpointShow(c *cli.Context) error {
	endpointID := strings.TrimSpace(c.Args().First())
	if endpointID == "" {
		return exitErr(2, "endpoint show: endpoint id required")
	}

	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "endpoint show: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "endpoint show: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if !opcreds.Present(settings.StateDir) {
		return exitErr(2, "endpoint show: operator credentials missing in %s (run remotr bootstrap first)", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "endpoint show: %v", err)
	}

	ep, err := client.GetEndpoint(endpointID)
	if err != nil {
		return exitErr(1, "endpoint show: %v", err)
	}

	if c.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(ep); err != nil {
			return exitErr(1, "endpoint show: %v", err)
		}
		return nil
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
	return nil
}

func actionEndpointRemove(c *cli.Context) error {
	endpointID := strings.TrimSpace(c.Args().First())
	if endpointID == "" {
		return exitErr(2, "endpoint remove: endpoint id required")
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
		return exitErr(1, "endpoint remove: %v", err)
	}
	fmt.Printf("removed endpoint %s\n", endpointID)
	return nil
}

func actionEndpointAgentUpgrade(c *cli.Context) error {
	endpointID := strings.TrimSpace(c.Args().First())
	if endpointID == "" {
		return exitErr(2, "endpoint agent upgrade: endpoint id required")
	}
	ver := strings.TrimSpace(c.String("version"))
	if ver == "" {
		return exitErr(2, "endpoint agent upgrade: --version is required")
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
		return exitErr(1, "endpoint agent upgrade: %v", err)
	}
	fmt.Printf("upgrade requested for %s to %s (applies on next sync)\n", endpointID, ver)
	return nil
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
