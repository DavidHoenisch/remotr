package main

import (
	"encoding/json"
	"strings"
	"testing"
)

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
		"tpm": map[string]string{"version": "2.0"},
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
