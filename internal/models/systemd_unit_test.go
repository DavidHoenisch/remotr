package models_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestParseStateCanonicalSystemdUnitAndDropIn(t *testing.T) {
	state, err := models.ParseState(strings.NewReader(`
schemaVersion: 1
configurations:
- name: base
  resources:
  - kind: systemdUnit
    name: telemetry-unit
    lifecycle: present
    unit: telemetry.service
    content: |
      [Service]
      ExecStart=/usr/local/bin/telemetry
    mode: [420]
    owner: root
    group: root
  - kind: systemdUnit
    name: telemetry-limits
    lifecycle: absent
    unit: telemetry.service
    dropIn: 20-limits.conf
`))
	if err != nil {
		t.Fatal(err)
	}
	units := state.Configurations[0].SystemdUnits
	if len(units) != 2 || units[0].Unit != "telemetry.service" || units[0].Content == "" || units[1].DropIn != "20-limits.conf" || units[1].Lifecycle != models.LifecycleAbsent {
		t.Fatalf("systemd units = %+v", units)
	}
}

func TestParseStateSystemdUnitRejectsUnsafeAuthoring(t *testing.T) {
	tests := []struct{ name, fields, want string }{
		{"path unit", "unit: ../ssh.service\n    content: '[Service]'", "unit identity"},
		{"unsafe drop-in", "unit: ssh.service\n    dropIn: ../override.conf\n    content: '[Service]'", "dropIn"},
		{"missing content", "unit: ssh.service", "content is required"},
		{"absent content", "lifecycle: absent\n    unit: ssh.service\n    content: '[Service]'", "absent systemd unit"},
		{"invalid mode", "unit: ssh.service\n    content: '[Service]'\n    mode: [1024]", "mode"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := models.ParseState(strings.NewReader("schemaVersion: 1\nconfigurations:\n- name: base\n  resources:\n  - kind: systemdUnit\n    name: example\n    " + test.fields + "\n"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseState() error = %v, want %q", err, test.want)
			}
		})
	}
}
