package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/urfave/cli/v3"
)

type inventoryRow struct {
	EndpointID     string `json:"endpoint_id"`
	Fleet          string `json:"fleet"`
	OS             string `json:"os"`
	CPU            string `json:"cpu"`
	RAM            string `json:"ram"`
	Kernel         string `json:"kernel"`
	PrimaryIP      string `json:"primary_ip"`
	MACAddress     string `json:"mac_address"`
	DiskEncryption string `json:"disk_encryption"`
	TPM            string `json:"tpm"`
	AgentVersion   string `json:"agent_version"`
	LastCheckIn    string `json:"last_check_in"`
}

func inventoryCommand() *cli.Command {
	return &cli.Command{
		Name:     "inventory",
		Category: catInventory,
		Usage:    "extract asset inventory of all enrolled machines",
		Description: withExamples("",
			"remotr inventory",
			"remotr inventory --json",
			"remotr inventory --save",
			"remotr inventory --json --save"),
		Action: actionInventory,
		Flags: append(outputFlags(),
			&cli.BoolFlag{Name: "save", Usage: "save inventory to a datetime-stamped file"},
		),
	}
}

func actionInventory(_ context.Context, c *cli.Command) error {
	settings, err := resolveSettings(c)
	if err != nil {
		return exitErr(2, "inventory: %v", err)
	}
	if settings.ServerURL == "" {
		return exitErr(2, "inventory: server URL is required (config, REMOTR_SERVER_URL, or --server-url)")
	}
	if !opcreds.Present(settings.StateDir) {
		return exitErr(2, "inventory: operator credentials missing in %s (run remotr bootstrap first)", settings.StateDir)
	}

	client, err := admin.NewClientFromState(strings.TrimRight(settings.ServerURL, "/"), settings.StateDir)
	if err != nil {
		return exitErr(1, "inventory: %v", err)
	}

	eps, err := client.ListEndpoints()
	if err != nil {
		return apiErr(c, "inventory", err)
	}

	if len(eps) == 0 {
		writeInfoLine("no endpoints enrolled")
		return nil
	}

	rows := make([]inventoryRow, len(eps))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for i, ep := range eps {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			epDetail, err := client.GetEndpoint(id)
			if err != nil {
				writeInfoLine("inventory: warning: could not get endpoint %s: %v", id, err)
				return
			}
			row := buildInventoryRow(epDetail)
			mu.Lock()
			rows[idx] = row
			mu.Unlock()
		}(i, ep.ID)
	}
	wg.Wait()

	var filled []inventoryRow
	for _, r := range rows {
		if r.EndpointID != "" {
			filled = append(filled, r)
		}
	}

	if resolveFormat(c) == formatJSON {
		out, err := json.MarshalIndent(filled, "", "  ")
		if err != nil {
			return exitErr(1, "inventory: %v", err)
		}
		out = append(out, '\n')
		if c.Bool("save") {
			path, err := saveInventory(out, "json")
			if err != nil {
				return exitErr(1, "inventory: %v", err)
			}
			fmt.Print(string(out))
			writeInfoLine("saved inventory to %s", path)
			return nil
		}
		fmt.Print(string(out))
		return nil
	}

	var sb strings.Builder
	if !c.Bool("no-headers") {
		sb.WriteString("ID\tFLEET\tOS\tCPU\tRAM\tPRIMARY_IP\tDISK_ENC\tTPM\tAGENT_VERSION\tLAST_CHECKIN\n")
	}
	for _, r := range filled {
		fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.EndpointID, r.Fleet, r.OS, r.CPU, r.RAM, r.PrimaryIP,
			r.DiskEncryption, r.TPM, r.AgentVersion, r.LastCheckIn)
	}

	output := sb.String()
	if c.Bool("save") {
		path, err := saveInventory([]byte(output), "txt")
		if err != nil {
			return exitErr(1, "inventory: %v", err)
		}
		fmt.Print(output)
		writeInfoLine("saved inventory to %s", path)
		return nil
	}
	fmt.Print(output)
	return nil
}

func buildInventoryRow(ep admin.Endpoint) inventoryRow {
	row := inventoryRow{
		EndpointID:   ep.ID,
		Fleet:        ep.Fleet,
		AgentVersion: ep.ReportedAgentVersion,
	}
	if ep.LastCheckIn != nil && !ep.LastCheckIn.At.IsZero() {
		row.LastCheckIn = ep.LastCheckIn.At.UTC().Format(time.RFC3339)
	}
	if ep.SystemInfo != nil && len(ep.SystemInfo.Report) > 0 {
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
			} `json:"networks"`
			BlockDevices []struct {
				Name           string `json:"name"`
				Encrypted      bool   `json:"encrypted"`
				EncryptionType string `json:"encryptionType"`
			} `json:"blockDevices"`
			Kernel struct {
				Version string `json:"version"`
			} `json:"kernel"`
			TPM struct {
				Version string `json:"version"`
			} `json:"tpm"`
		}
		if err := json.Unmarshal(ep.SystemInfo.Report, &snap); err == nil {
			row.OS = inventoryOS(snap.OSRelease.PrettyName, snap.OSRelease.Name, snap.OSRelease.VersionID)
			if snap.CPU.ModelName != "" {
				row.CPU = snap.CPU.ModelName
				if snap.CPU.CoreCount != "" {
					row.CPU += " (" + snap.CPU.CoreCount + " cores)"
				}
			}
			row.RAM = snap.RAM.MemTotal
			row.Kernel = snap.Kernel.Version
			for _, net := range snap.Networks {
				if net.Name == "lo" {
					continue
				}
				if net.MACAddress != "" && row.MACAddress == "" {
					row.MACAddress = net.MACAddress
				}
				if len(net.IPv4) > 0 && row.PrimaryIP == "" {
					row.PrimaryIP = net.IPv4[0]
				}
			}
			if row.PrimaryIP == "" {
				for _, net := range snap.Networks {
					if net.Name == "lo" {
						continue
					}
					if len(net.IPv6) > 0 {
						row.PrimaryIP = net.IPv6[0]
						break
					}
				}
			}
			if len(snap.BlockDevices) > 0 {
				enc := 0
				for _, dev := range snap.BlockDevices {
					if dev.Encrypted {
						enc++
					}
				}
				row.DiskEncryption = fmt.Sprintf("%d/%d", enc, len(snap.BlockDevices))
			}
			if snap.TPM.Version != "" {
				row.TPM = "present (version " + snap.TPM.Version + ")"
			} else {
				row.TPM = "not reported"
			}
		}
	}
	return row
}

func inventoryOS(pretty, name, version string) string {
	switch {
	case pretty != "":
		return pretty
	case name != "" && version != "":
		return name + " " + version
	case name != "":
		return name
	default:
		return ""
	}
}

func saveInventory(data []byte, ext string) (string, error) {
	ts := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("remotr-inventory-%s.%s", ts, ext)
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filename)
	if err != nil {
		return filename, nil
	}
	return abs, nil
}
