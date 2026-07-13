package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseStateCanonicalCoordinatedReboot(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: reboot
    name: kernel-maintenance
    generation: kernel-6.12.1
    onlyIfRequired: true
    delay: 2m
    timeout: 15m
    deadline: 2026-07-13T05:00:00Z
    maintenanceWindow:
      weekdays: [Sunday]
      start: "02:00"
      duration: 2h
    requireACPower: true
    userInhibition: defer
    workloadInhibition: defer
    enforce: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Reboots) != 1 {
		t.Fatalf("state = %+v", state)
	}
	got := state.Configurations[0].Reboots[0]
	if got.Generation != "kernel-6.12.1" || !got.OnlyIfRequired || got.Delay != "2m" || got.Timeout != "15m" || got.MaintenanceWindow == nil || len(got.MaintenanceWindow.Weekdays) != 1 || got.MaintenanceWindow.Start != "02:00" || got.MaintenanceWindow.Duration != "2h" || got.UserInhibition != models.InhibitionDefer || got.WorkloadInhibition != models.InhibitionDefer || got.Enforce == nil || !*got.Enforce {
		t.Fatalf("reboot = %+v", got)
	}
}

func TestParseStateRejectsUnsafeCoordinatedRebootAuthoring(t *testing.T) {
	tests := []struct {
		name, fields, want string
	}{
		{"missing generation", "timeout: 15m", "generation"},
		{"generation too long", "generation: " + strings.Repeat("g", 257) + "\ntimeout: 15m", "generation"},
		{"zero timeout", "generation: g1\ntimeout: 0s", "timeout"},
		{"timeout too long", "generation: g1\ntimeout: 61m", "timeout"},
		{"delay too long", "generation: g1\ndelay: 25h\ntimeout: 15m", "delay"},
		{"invalid deadline", "generation: g1\ntimeout: 15m\ndeadline: not-a-time", "deadline"},
		{"invalid inhibition", "generation: g1\ntimeout: 15m\nuserInhibition: force", "userInhibition"},
		{"invalid window", "generation: g1\ntimeout: 15m\nmaintenanceWindow: {weekdays: [Funday], start: '02:00', duration: 1h}", "weekdays"},
		{"risk downgrade", "generation: g1\ntimeout: 15m\nrisk: normal", "risk"},
		{"unsupported lifecycle", "generation: g1\ntimeout: 15m\nlifecycle: absent", "lifecycle"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := "schemaVersion: 1\nconfigurations:\n- name: base\n  resources:\n  - kind: reboot\n    name: maintenance\n    " + strings.ReplaceAll(tc.fields, "\n", "\n    ") + "\n"
			_, err := models.ParseState(strings.NewReader(yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseState() error = %v, want %q", err, tc.want)
			}
		})
	}
}
