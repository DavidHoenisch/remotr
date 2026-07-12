package models

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestParseState_rejectsInvalidYAML(t *testing.T) {
	_, err := ParseState(strings.NewReader("configurations:\n  - name: [\n"))
	if err == nil {
		t.Fatal("expected error for invalid yaml")
	}
}

func TestParseState_rejectsEmpty(t *testing.T) {
	_, err := ParseState(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseState_parsesMinimalConfiguration(t *testing.T) {
	lastUpdated := time.Date(2026, 5, 9, 12, 30, 0, 0, time.UTC)
	input := `configurations:
  - name: base
    description: base packages
    lastUpdated: "2026-05-09T12:30:00Z"
    targetDistros:
      - Ubuntu
      - Arch
`
	want := State{Configurations: []Configuration{
		{
			Name:          "base",
			Description:   "base packages",
			LastUpdated:   lastUpdated,
			TargetDistros: []types.Distro{types.Ubuntu, types.Arch},
		},
	}}

	got, err := ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseState() = %#v, want %#v", got, want)
	}
}

func TestParseState_canonicalSchemaRejectsUnknownResourceFieldWithAddress(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: curl
        present: true
        presnt: false
`

	_, err := ParseState(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected unknown canonical resource field to be rejected")
	}
	for _, want := range []string{"base/curl", "presnt"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ParseState() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestParseState_schemaVersionBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantVersion int
		wantErr     string
	}{
		{
			name:        "unversioned is legacy schema zero",
			input:       "configurations:\n  - name: legacy\n",
			wantVersion: 0,
		},
		{
			name: "schema one canonical resource",
			input: `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: curl
        present: true
`,
			wantVersion: 1,
		},
		{
			name:    "future schema is rejected",
			input:   "schemaVersion: 2\nconfigurations: []\n",
			wantErr: "unsupported desired-state schemaVersion 2",
		},
		{
			name:    "canonical top-level field is rejected",
			input:   "schemaVersion: 1\nconfigurations: []\nsurprise: true\n",
			wantErr: "field surprise not found",
		},
		{
			name: "canonical configuration field is rejected",
			input: `schemaVersion: 1
configurations:
  - name: base
    resorces: []
`,
			wantErr: "field resorces not found",
		},
		{
			name: "unknown resource kind has stable address",
			input: `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: mystery
        name: example
`,
			wantErr: `resource "base/example": unknown resource kind "mystery"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseState(strings.NewReader(tt.input))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseState() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseState() error = %v", err)
			}
			if got.SchemaVersion != tt.wantVersion {
				t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, tt.wantVersion)
			}
		})
	}
}
