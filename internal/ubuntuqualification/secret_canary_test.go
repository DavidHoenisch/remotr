package ubuntuqualification_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

var requiredSecretCanarySelectors = []string{
	"go-test:./internal/resourceregistry:^TestResourceSafeSummaryProjectsClassifiedFieldsAndOmitsSecretCanary$",
	"go-test:./cmd/remotr:^TestSecretUploadReadsProtectedInputFileAndNeverAcceptsArgvMaterial$",
	"make:test-e2e",
}

// OS-AEC-099: provider-local canaries are necessary but not sufficient for a
// qualified secret-bearing row. Every such row must also retain the shared
// desired-state, argv, agent, Sync, persistence, API, CLI, rollback, and
// cleanup evidence selectors, and removing any part must invalidate the
// qualification manifest before publication can consume it.
func TestSecretCanaryBlocksQualification(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}

	wantRows := map[string]bool{
		"certificate/pem-files":              true,
		"trustAnchor/update-ca-certificates": true,
		"appArmorProfile/apparmor-parser":    true,
		"auditRules/auditd":                  true,
		"accountLimit/pam-limits":            true,
		"loginPolicy/pam-auth-update":        true,
		"journald/systemd-journald":          true,
		"logrotate/logrotate":                true,
	}
	secretRows := make([]int, 0, len(wantRows))
	for index, row := range manifest.Rows {
		if row.Disposition != "qualified" || !slices.Contains(row.TDD.EvidenceLayers, "secret-canary") {
			continue
		}
		key := row.CapabilityID + "/" + row.Backend
		if !wantRows[key] {
			t.Errorf("unexpected qualified secret-canary row %s", key)
			continue
		}
		delete(wantRows, key)
		secretRows = append(secretRows, index)
		if !slices.Contains(row.GoverningIDs, "OS-AEC-099") {
			t.Errorf("%s does not govern qualification with OS-AEC-099", key)
		}
		for _, selector := range requiredSecretCanarySelectors {
			if !slices.Contains(row.Selectors, selector) {
				t.Errorf("%s is missing shared secret-canary selector %q", key, selector)
			}
		}
	}
	for key := range wantRows {
		t.Errorf("missing qualified secret-canary row %s", key)
	}
	if len(secretRows) != 8 {
		t.Fatalf("qualified secret-canary row count = %d, want 8", len(secretRows))
	}

	mutations := []struct {
		name   string
		mutate func(*ubuntuqualification.Row)
	}{
		{name: "governing ID", mutate: func(row *ubuntuqualification.Row) {
			row.GoverningIDs = slices.DeleteFunc(row.GoverningIDs, func(value string) bool { return value == "OS-AEC-099" })
		}},
		{name: "evidence layer", mutate: func(row *ubuntuqualification.Row) {
			row.TDD.EvidenceLayers = slices.DeleteFunc(row.TDD.EvidenceLayers, func(value string) bool { return value == "secret-canary" })
		}},
	}
	for _, selector := range requiredSecretCanarySelectors {
		selector := selector
		mutations = append(mutations, struct {
			name   string
			mutate func(*ubuntuqualification.Row)
		}{name: selector, mutate: func(row *ubuntuqualification.Row) {
			row.Selectors = slices.DeleteFunc(row.Selectors, func(value string) bool { return value == selector })
		}})
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := manifest.Clone()
			mutation.mutate(&candidate.Rows[secretRows[0]])
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), "OS-AEC-099 secret-canary evidence") {
				t.Fatalf("Validate() error = %v, want OS-AEC-099 secret-canary rejection", err)
			}
		})
	}
}
