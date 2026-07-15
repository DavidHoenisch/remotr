package models

import (
	"fmt"
	"strings"
	"testing"
)

// OS-ESM-004: endpoint schedules preserve argv boundaries and are decoded as
// persistent local resources, not server-dispatched CronJob API objects.
func TestParseStateEndpointSchedulePreservesStructuredInputs(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: endpointSchedule
        name: nightly-backup
        lifecycle: present
        backend: cron
        schedule: "0 3 * * *"
        user: root
        argv: [/usr/local/bin/backup, "daily archive"]
        workingDirectory: /var/lib/backup
        environment:
          - name: BACKUP_BUCKET
            value: archive
          - name: BACKUP_TOKEN
            secretRef: remotr:schedules/backup-token
        timeout: 30m
        overlap: forbid
`

	state, err := ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].EndpointSchedules) != 1 {
		t.Fatalf("ParseState() = %#v, want one endpoint schedule", state)
	}
	got := state.Configurations[0].EndpointSchedules[0]
	if got.Backend != ScheduleBackendCron || len(got.Argv) != 2 || got.Argv[1] != "daily archive" {
		t.Fatalf("endpoint schedule = %#v, want cron and preserved argv", got)
	}
}

// OS-ESM-002 and OS-ESM-005: author-time validation rejects incompatible
// backend fields and malformed cron expressions at the stable resource address.
func TestParseStateEndpointScheduleRejectsInvalidAuthoring(t *testing.T) {
	tests := []struct {
		name    string
		fields  string
		message string
	}{
		{
			name: "systemd persistence on cron",
			fields: `        backend: cron
        schedule: "0 3 * * *"
        user: root
        argv: [/usr/bin/true]
        persistent: true
`,
			message: "persistent is supported only by systemd-timer",
		},
		{
			name: "invalid cron expression",
			fields: `        backend: cron
        schedule: "60 3 * * *"
        user: root
        argv: [/usr/bin/true]
`,
			message: "schedule minute",
		},
		{
			name: "ambiguous executable form",
			fields: `        backend: cron
        schedule: "0 3 * * *"
        user: root
        argv: [/usr/bin/true]
        shell: echo unsafe
`,
			message: "exactly one of argv or shell",
		},
		{
			name: "systemd missed-run policy omitted",
			fields: `        backend: systemd-timer
        schedule: daily
        user: root
        argv: [/usr/bin/true]
`,
			message: "explicit persistent missed-run policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: endpointSchedule\n        name: nightly-backup\n        lifecycle: present\n" + tt.fields
			_, err := ParseState(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), "base/nightly-backup") || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("ParseState() error = %v, want address and %q", err, tt.message)
			}
		})
	}
}

// FuzzEndpointScheduleAuthoring keeps schema input bounded while asserting
// that any accepted cron expression produces exactly one typed local schedule.
func FuzzEndpointScheduleAuthoring(f *testing.F) {
	f.Add("0 3 * * *")
	f.Add("60 3 * * *")
	f.Add("@daily")
	f.Fuzz(func(t *testing.T, expression string) {
		if len(expression) > 256 || strings.ContainsRune(expression, '\x00') {
			return
		}
		input := "schemaVersion: 1\nconfigurations:\n- name: base\n  resources:\n  - kind: endpointSchedule\n    name: fuzz\n    backend: cron\n    schedule: " + fmt.Sprintf("%q", expression) + "\n    user: root\n    argv: [/usr/bin/true]\n"
		state, err := ParseState(strings.NewReader(input))
		if err != nil {
			return
		}
		if len(state.Configurations) != 1 || len(state.Configurations[0].EndpointSchedules) != 1 {
			t.Fatalf("accepted endpoint schedule was dropped: %#v", state)
		}
	})
}
