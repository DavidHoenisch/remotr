package ubuntuqualification_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

// OS-AEC-098: every qualified Ubuntu desktop/session/browser row retains the
// executable VM selector and its recorded logged-in/logged-out recovery result.
// Static file output cannot replace any part of that evidence set.
func TestQualifiedDesktopRowsRequireLiveAndLoggedOutVMEvidence(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}

	const selector = "make:provider-matrix-vm-desktop-session"
	rowIndexes := make(map[string]int)
	for index, row := range manifest.Rows {
		switch row.CapabilityID {
		case "desktopSetting", "sessionPolicy", "browserPolicy":
			if row.Disposition != "qualified" {
				continue
			}
			key := row.CapabilityID + "/" + row.Backend
			rowIndexes[key] = index
			if !slices.Equal(row.Selectors, []string{selector}) ||
				!slices.Contains(row.TDD.EvidenceLayers, "ubuntu-24.04-vm") ||
				!slices.Contains(row.TDD.EvidenceLayers, "desktop-session") ||
				!strings.Contains(row.Reason+" "+*row.TDD.GreenResult, "logged-in") ||
				!strings.Contains(row.Reason+" "+*row.TDD.GreenResult, "logged-out") ||
				!slices.ContainsFunc(row.TDD.BroaderChecks, func(check string) bool { return strings.Contains(check, selector) }) {
				t.Errorf("%s has incomplete live/logged-out desktop VM evidence: %+v", key, row)
			}
		}
	}
	if len(rowIndexes) != 7 {
		t.Fatalf("qualified desktop rows = %d, want 7: %v", len(rowIndexes), rowIndexes)
	}

	for name, mutate := range map[string]func(*ubuntuqualification.Row){
		"static selector": func(row *ubuntuqualification.Row) {
			row.Selectors = []string{"go-test:./internal/providermatrix:^TestDesktopSessionFixtureDeclaresRequiredEvidence$"}
		},
		"missing desktop evidence layer": func(row *ubuntuqualification.Row) {
			row.TDD.EvidenceLayers = slices.DeleteFunc(row.TDD.EvidenceLayers, func(layer string) bool { return layer == "desktop-session" })
		},
		"missing VM broader check": func(row *ubuntuqualification.Row) {
			row.TDD.BroaderChecks = slices.DeleteFunc(row.TDD.BroaderChecks, func(check string) bool { return strings.Contains(check, selector) })
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := manifest.Clone()
			mutate(&candidate.Rows[rowIndexes["desktopSetting/dconf"]])
			err := ubuntuqualification.Validate(candidate, registry)
			if err == nil || !strings.Contains(err.Error(), "logged-in/logged-out desktop VM evidence") {
				t.Fatalf("Validate() error = %v, want logged-in/logged-out desktop VM evidence rejection", err)
			}
		})
	}
}
