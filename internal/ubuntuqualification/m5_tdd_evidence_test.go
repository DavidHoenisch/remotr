package ubuntuqualification_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

func TestM5CorrectionsRequireRecordedUbuntuVMEvidence(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]string{
		"certificate/pem-files":              "make:provider-matrix-vm-system-safety",
		"trustAnchor/update-ca-certificates": "make:provider-matrix-vm-system-safety",
		"appArmorProfile/apparmor-parser":    "make:provider-matrix-vm-system-safety",
		"auditRules/auditd":                  "make:provider-matrix-vm-system-safety",
		"accountLimit/pam-limits":            "make:provider-matrix-vm-user-safety",
		"loginPolicy/pam-auth-update":        "make:provider-matrix-vm-login-policy-safety",
		"journald/systemd-journald":          "make:provider-matrix-vm-system-safety",
		"logrotate/logrotate":                "make:provider-matrix-vm-system-safety",
	}
	rows := make(map[string]int, len(required))
	for index, row := range manifest.Rows {
		key := row.CapabilityID + "/" + row.Backend
		selector, required := required[key]
		if !required {
			continue
		}
		rows[key] = index
		if row.Disposition != "qualified" || row.TDD.Phase != "verified" || row.TDD.PublicSeam != "system-safety-recovery" ||
			!slices.Contains(row.GoverningIDs, row.TDD.GoverningID) || !slices.Contains(row.TDD.EvidenceLayers, "ubuntu-24.04-vm") ||
			!slices.Contains(row.Selectors, selector) || row.TDD.RedFailure == nil || row.TDD.GreenResult == nil ||
			!slices.ContainsFunc(row.TDD.BroaderChecks, func(check string) bool { return strings.Contains(check, selector) }) {
			t.Errorf("%s has incomplete task 9.10 evidence: %+v", key, row.TDD)
		}
	}
	if len(rows) != len(required) {
		t.Fatalf("task 9.10 rows = %d, want %d: %v", len(rows), len(required), rows)
	}

	for key, index := range rows {
		t.Run(key+"/vm-selector", func(t *testing.T) {
			candidate := manifest.Clone()
			selector := required[key]
			candidate.Rows[index].Selectors = slices.DeleteFunc(candidate.Rows[index].Selectors, func(value string) bool { return value == selector })
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), "task 9.10 Ubuntu VM evidence") {
				t.Fatalf("Validate() error = %v, want task 9.10 Ubuntu VM evidence", err)
			}
		})

		t.Run(key+"/vm-layer", func(t *testing.T) {
			candidate := manifest.Clone()
			candidate.Rows[index].TDD.EvidenceLayers = slices.DeleteFunc(candidate.Rows[index].TDD.EvidenceLayers, func(value string) bool { return value == "ubuntu-24.04-vm" })
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), "task 9.10 Ubuntu VM evidence") {
				t.Fatalf("Validate() error = %v, want task 9.10 Ubuntu VM evidence", err)
			}
		})

		t.Run(key+"/broader-result", func(t *testing.T) {
			candidate := manifest.Clone()
			selector := required[key]
			candidate.Rows[index].TDD.BroaderChecks = slices.DeleteFunc(candidate.Rows[index].TDD.BroaderChecks, func(value string) bool { return strings.Contains(value, selector) })
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), "task 9.10 Ubuntu VM evidence") {
				t.Fatalf("Validate() error = %v, want task 9.10 Ubuntu VM evidence", err)
			}
		})
	}
}
