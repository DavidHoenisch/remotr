package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func TestEndpointOutputSeparatesTargetOfferedAndActiveRelease(t *testing.T) {
	offeredSchema, activeSchema := 1, 0
	endpoint := admin.Endpoint{
		ID: "endpoint-1", Fleet: "engineering",
		TargetReleaseRef: "release-target", OfferedReleaseRef: "release-offered", OfferedDigest: "digest-offered", OfferedSchemaVersion: &offeredSchema,
		ActiveReleaseRef: "release-active", ActiveDigest: "digest-active", ActiveSchemaVersion: &activeSchema,
		CapabilityDigest: "sha256:capability", CapabilityBlockedTargetRef: "release-target",
		MissingRequirements: []admin.MissingRequirement{{ID: "provider:package/apt", Revision: "1"}},
	}
	output := captureStdout(t, func() { printEndpoint(endpoint) })
	for _, want := range []string{
		"target_release_ref: release-target", "offered_release_ref: release-offered", "offered_digest: digest-offered", "offered_schema_version: 1",
		"active_release_ref: release-active", "active_digest: digest-active", "active_schema_version: 0",
		"capability_digest: sha256:capability", "capability_blocked_target_ref: release-target", "provider:package/apt@1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("endpoint output missing %q:\n%s", want, output)
		}
	}
}

func TestFormatSystemInfoSummary(t *testing.T) {
	report, err := json.Marshal(map[string]any{
		"osRelease": map[string]string{
			"prettyName": "Arch Linux",
		},
		"cpu": map[string]string{
			"modelName": "Example CPU",
			"coreCount": "8",
		},
		"ram": map[string]string{
			"memTotal": "16384000 kB",
		},
		"networks": []map[string]any{
			{
				"name":       "wlan0",
				"macAddress": "aa:bb:cc:dd:ee:ff",
				"ipv4":       []string{"192.168.1.10"},
				"statistics": map[string]string{"operstate": "up"},
			},
		},
		"batteries": []map[string]string{
			{
				"name":     "BAT0",
				"status":   "Discharging",
				"capacity": "42",
			},
		},
		"blockDevices": []map[string]any{
			{
				"name":           "nvme0n1",
				"encrypted":      true,
				"encryptionType": "LUKS2",
			},
			{
				"name":      "sda",
				"encrypted": false,
			},
		},
		"kernel": map[string]string{"version": "6.9.3-arch1-1"},
		"tpm":    map[string]string{"version": "2.0"},
	})
	if err != nil {
		t.Fatal(err)
	}

	lines := formatSystemInfoSummary(report)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"os: Arch Linux",
		"cpu: Example CPU (8 cores)",
		"ram: 16384000 kB",
		"network wlan0: mac=aa:bb:cc:dd:ee:ff ipv4=192.168.1.10 operstate=up",
		"battery BAT0: 42% (Discharging)",
		"block_device nvme0n1: encrypted (LUKS2)",
		"block_device sda: not encrypted",
		"disk_encryption: 1/2 devices encrypted",
		"kernel: 6.9.3-arch1-1",
		"tpm: present (version 2.0)",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("summary missing %q:\n%s", want, joined)
		}
	}
}

func TestFormatOSReleaseLine(t *testing.T) {
	if got := formatOSReleaseLine("Arch Linux", "", ""); got != "os: Arch Linux" {
		t.Fatalf("got %q", got)
	}
	if got := formatOSReleaseLine("", "Debian", "12"); got != "os: Debian 12" {
		t.Fatalf("got %q", got)
	}
}
