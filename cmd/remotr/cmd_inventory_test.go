package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func TestBuildInventoryRow(t *testing.T) {
	report, err := json.Marshal(map[string]any{
		"osRelease": map[string]string{"prettyName": "Ubuntu 22.04"},
		"cpu":       map[string]string{"modelName": "Intel(R) Core(TM) i7", "coreCount": "8"},
		"ram":       map[string]string{"memTotal": "32GiB"},
		"networks": []map[string]any{
			{"name": "eth0", "macAddress": "aa:bb:cc:dd:ee:ff", "ipv4": []string{"10.0.0.5"}},
		},
		"blockDevices": []map[string]any{
			{"name": "nvme0n1", "encrypted": true},
		},
		"kernel": map[string]string{"version": "5.15"},
		"tpm":    map[string]string{"version": "2.0"},
	})
	if err != nil {
		t.Fatal(err)
	}

	ep := admin.Endpoint{
		ID:                   "ep-123",
		Fleet:                "prod",
		ReportedAgentVersion: "v1.0.0",
		LastCheckIn: &admin.CheckInSummary{
			At: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		SystemInfo: &admin.SystemInfoSummary{
			Report: report,
		},
	}

	row := buildInventoryRow(ep)
	if row.EndpointID != "ep-123" {
		t.Errorf("id = %q, want ep-123", row.EndpointID)
	}
	if row.Fleet != "prod" {
		t.Errorf("fleet = %q, want prod", row.Fleet)
	}
	if row.OS != "Ubuntu 22.04" {
		t.Errorf("os = %q, want Ubuntu 22.04", row.OS)
	}
	if row.CPU != "Intel(R) Core(TM) i7 (8 cores)" {
		t.Errorf("cpu = %q", row.CPU)
	}
	if row.RAM != "32GiB" {
		t.Errorf("ram = %q", row.RAM)
	}
	if row.Kernel != "5.15" {
		t.Errorf("kernel = %q", row.Kernel)
	}
	if row.PrimaryIP != "10.0.0.5" {
		t.Errorf("primary_ip = %q", row.PrimaryIP)
	}
	if row.MACAddress != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("mac = %q", row.MACAddress)
	}
	if row.DiskEncryption != "1/1" {
		t.Errorf("disk_enc = %q", row.DiskEncryption)
	}
	if row.TPM != "present (version 2.0)" {
		t.Errorf("tpm = %q", row.TPM)
	}
	if row.AgentVersion != "v1.0.0" {
		t.Errorf("agent_version = %q", row.AgentVersion)
	}
	if row.LastCheckIn != "2024-01-01T00:00:00Z" {
		t.Errorf("last_checkin = %q", row.LastCheckIn)
	}
}

func TestBuildInventoryRowNoSystemInfo(t *testing.T) {
	ep := admin.Endpoint{
		ID:    "ep-456",
		Fleet: "dev",
	}
	row := buildInventoryRow(ep)
	if row.EndpointID != "ep-456" {
		t.Errorf("id = %q", row.EndpointID)
	}
	if row.Fleet != "dev" {
		t.Errorf("fleet = %q", row.Fleet)
	}
	if row.OS != "" {
		t.Errorf("os = %q, want empty", row.OS)
	}
	if row.CPU != "" {
		t.Errorf("cpu = %q, want empty", row.CPU)
	}
	if row.TPM != "" {
		t.Errorf("tpm = %q, want empty", row.TPM)
	}
}

func TestInventoryOS(t *testing.T) {
	if got := inventoryOS("Arch Linux", "", ""); got != "Arch Linux" {
		t.Errorf("got %q", got)
	}
	if got := inventoryOS("", "Debian", "12"); got != "Debian 12" {
		t.Errorf("got %q", got)
	}
	if got := inventoryOS("", "Debian", ""); got != "Debian" {
		t.Errorf("got %q", got)
	}
	if got := inventoryOS("", "", ""); got != "" {
		t.Errorf("got %q", got)
	}
}
