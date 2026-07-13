package rebootstate

import (
	"strings"
	"testing"
)

func TestParseStateRejectsInvalidCoordinatedRebootState(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "schema one cannot contain coordination fields",
			raw:  `{"schemaVersion":1,"required":false,"attemptGeneration":1}`,
		},
		{
			name: "unknown intent phase",
			raw:  `{"schemaVersion":2,"required":false,"intent":{"generation":"g1","phase":"retrying","priorBootId":"boot-1","preparedAt":"2026-07-13T02:00:00Z","notBefore":"2026-07-13T02:00:00Z","timeout":60000000000}}`,
		},
		{
			name: "attempt exceeds durable generation",
			raw:  `{"schemaVersion":2,"required":false,"attemptGeneration":1,"intent":{"generation":"g1","phase":"attempting","priorBootId":"boot-1","preparedAt":"2026-07-13T02:00:00Z","notBefore":"2026-07-13T02:00:00Z","timeout":60000000000,"attemptedAt":"2026-07-13T02:00:00Z","attemptDeadline":"2026-07-13T02:01:00Z","attemptGeneration":2}}`,
		},
		{
			name: "completion lacks changed boot identity",
			raw:  `{"schemaVersion":2,"required":false,"attemptGeneration":1,"completion":{"generation":"g1","bootId":"","attemptGeneration":1,"completedAt":"2026-07-13T02:01:00Z"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseState([]byte(tt.raw)); err == nil || !strings.Contains(err.Error(), "parse reboot state") {
				t.Fatalf("parseState() error = %v", err)
			}
		})
	}
}

func TestParseStateMigratesSchemaOneRequirement(t *testing.T) {
	status, err := parseState([]byte(`{"schemaVersion":1,"required":true,"sources":[{"address":"base/kernel","provider":"apt"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !status.Required || len(status.Sources) != 1 || status.Intent != nil || status.Completion != nil || status.AttemptGeneration != 0 {
		t.Fatalf("migrated status = %+v", status)
	}
}
