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
        lifecycle: present
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
        lifecycle: present
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

func TestParseState_canonicalSharedResourceMetadata(t *testing.T) {
	input := `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: curl
        lifecycle: present
        dependsOn: [base/repository]
        providerOptions:
          apt:
            refresh: true
        policy: report
        ownership: named
        validation:
          - command: [apt-cache, show, curl]
        notifications:
          - type: restart
            target: example.service
        risk: sensitive
        authorizationGroup: base-transition
        present: true
`

	state, err := ParseState(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseState() error = %v", err)
	}
	if len(state.Configurations) != 1 || len(state.Configurations[0].Packages) != 1 {
		t.Fatalf("ParseState() = %#v, want one package", state)
	}
	resource := state.Configurations[0].Packages[0]
	if resource.Kind != ResourceKindPackage || resource.Name != "curl" || resource.Lifecycle != LifecyclePresent {
		t.Fatalf("resource identity/lifecycle = %#v", resource)
	}
	if !reflect.DeepEqual(resource.DependsOn, []string{"base/repository"}) || resource.Policy != RemediationReport || resource.Ownership != OwnershipNamed {
		t.Fatalf("resource dependency/policy/ownership = %#v", resource.ResourceMeta)
	}
	if resource.ProviderOptions["apt"]["refresh"] != true {
		t.Fatalf("providerOptions = %#v", resource.ProviderOptions)
	}
	if !reflect.DeepEqual(resource.Validation, []ValidationRule{{Command: []string{"apt-cache", "show", "curl"}}}) {
		t.Fatalf("validation = %#v", resource.Validation)
	}
	if !reflect.DeepEqual(resource.Notifications, []Notification{{Type: NotificationRestart, Target: "example.service"}}) {
		t.Fatalf("notifications = %#v", resource.Notifications)
	}
	if resource.Risk != RiskSensitive || resource.AuthorizationGroup != "base-transition" {
		t.Fatalf("risk/authorization = %#v", resource.ResourceMeta)
	}
}

func TestParseState_packageLifecycleAndLegacyPresentCompatibility(t *testing.T) {
	t.Run("canonical absent", func(t *testing.T) {
		input := `schemaVersion: 1
configurations:
  - name: base
    resources:
      - kind: package
        name: curl
        lifecycle: absent
        packageManager: apt
`
		state, err := ParseState(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseState() error = %v", err)
		}
		pkg := state.Configurations[0].Packages[0]
		if pkg.Lifecycle != LifecycleAbsent || pkg.Present {
			t.Fatalf("package lifecycle = %q, present = %t; want absent/false", pkg.Lifecycle, pkg.Present)
		}
	})

	t.Run("legacy present maps to lifecycle", func(t *testing.T) {
		input := `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`
		state, err := ParseState(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ParseState() error = %v", err)
		}
		pkg := state.Configurations[0].Packages[0]
		if pkg.Lifecycle != LifecyclePresent || !pkg.Present {
			t.Fatalf("package lifecycle = %q, present = %t; want present/true", pkg.Lifecycle, pkg.Present)
		}
	})
}

func TestParseState_rejectsInvalidCanonicalSharedMetadata(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{"lifecycle", "lifecycle: maybe", `unknown lifecycle "maybe"`},
		{"policy", "policy: sometimes", `unknown remediation policy "sometimes"`},
		{"ownership", "ownership: everything", `unknown ownership mode "everything"`},
		{"risk", "risk: catastrophic", `unknown risk "catastrophic"`},
		{"empty validation argv", "validation:\n          - command: []", "requires non-empty command argv"},
		{"unknown notification", "notifications:\n          - type: bounce", `unknown type "bounce"`},
		{"restart target", "notifications:\n          - type: restart", `type "restart" requires target`},
		{"authorization whitespace", "authorizationGroup: ' transition '", "must not have surrounding whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: package\n        name: curl\n        present: true\n        " + tt.field + "\n"
			_, err := ParseState(strings.NewReader(input))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "base/curl") {
				t.Fatalf("ParseState() error = %v, want address and %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseStateWithDiagnostics_marksLegacySchemaDeprecated(t *testing.T) {
	state, diagnostics, err := ParseStateWithDiagnostics(strings.NewReader("configurations:\n  - name: legacy\n"))
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 0 || len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticLegacySchema {
		t.Fatalf("state/diagnostics = %#v / %#v", state, diagnostics)
	}
	if !strings.Contains(diagnostics[0].Message, "schema 0") || !strings.Contains(diagnostics[0].Message, "schemaVersion: 1") {
		t.Fatalf("diagnostic = %#v", diagnostics[0])
	}
}
