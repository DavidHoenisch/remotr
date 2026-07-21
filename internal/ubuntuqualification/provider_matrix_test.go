package ubuntuqualification_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/traceability"
	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

// Task 11.5: qualification, executable matrix evidence, and governing
// traceability are one release state. Exact rows may advance; family rows may
// not stand in for them.
func TestQualifiedRowsStayCoherentAcrossReleaseEvidence(t *testing.T) {
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := providermatrix.Load(filepath.Join("..", "..", "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	trace, err := traceability.LoadManifest(filepath.Join("..", "..", "test", "traceability.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, row := range manifest.Rows {
		environment := row.Environment
		if strings.HasPrefix(environment, "vm-") {
			environment = "vm"
		}
		var matches []providermatrix.Row
		for _, evidence := range matrix.Rows {
			if evidence.CapabilityID == row.CapabilityID && evidence.Backend == row.Backend &&
				evidence.ContractRevision == row.ContractRevision && evidence.Distribution == row.Distribution &&
				evidence.Release == row.Release && evidence.Architecture == row.Architecture && evidence.Environment == environment {
				matches = append(matches, evidence)
			}
		}

		switch row.Disposition {
		case "qualified":
			if len(matches) != 1 || matches[0].Status != "passing" {
				t.Errorf("qualified %s/%s evidence = %+v, want one passing row", row.CapabilityID, row.Backend, matches)
			} else {
				for _, selector := range matches[0].Selectors {
					if !slices.Contains(row.Selectors, selector) {
						t.Errorf("qualified %s/%s matrix selector %q is absent from qualification evidence %v", row.CapabilityID, row.Backend, selector, row.Selectors)
					}
				}
			}
			for _, id := range row.GoverningIDs {
				entry, ok := trace.Scenarios[id]
				if !ok || entry.Lifecycle != "verified" {
					t.Errorf("qualified %s/%s governing %s lifecycle = %q, want verified", row.CapabilityID, row.Backend, id, entry.Lifecycle)
				}
			}
		case "unadvertised":
			for _, evidence := range matches {
				if evidence.Status == "passing" {
					t.Errorf("unadvertised %s/%s has passing evidence: %+v", row.CapabilityID, row.Backend, evidence)
				}
			}
		default:
			t.Errorf("repository row %s/%s retains non-final disposition %q", row.CapabilityID, row.Backend, row.Disposition)
		}
	}

	for _, evidence := range matrix.Rows {
		if evidence.Distribution == manifest.Platform.Distribution && evidence.Release == manifest.Platform.Release &&
			evidence.Architecture == manifest.Platform.Architecture &&
			slices.Contains([]string{"desktop", "filesystem", "identity", "network", "security"}, evidence.CapabilityID) {
			t.Errorf("broad Ubuntu family row reached release evidence: %+v", evidence)
		}
	}
}

func TestBlockedContractsHaveExactUntestedRows(t *testing.T) {
	// Task 3.4 focused red observed: the matrix contained only the broad Ubuntu
	// filesystem, identity, and service rows and none of these 44 exact rows.
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ubuntuqualification.Load(filepath.Join("..", "..", "test", "qualification", "ubuntu-2404-applicators.yaml"), registry)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := providermatrix.Load(filepath.Join("..", "..", "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, broad := range []struct {
		capabilityID string
		revision     string
	}{{"filesystem", "v1"}, {"identity", "v1"}, {"service", "v1"}} {
		for _, row := range matrix.Rows {
			if row.Distribution == manifest.Platform.Distribution && row.Release == manifest.Platform.Release &&
				row.Architecture == manifest.Platform.Architecture && row.CapabilityID == broad.capabilityID &&
				row.ContractRevision == broad.revision {
				t.Errorf("legacy broad discovery row remains in the Ubuntu matrix: %+v", row)
			}
		}
	}

	for _, expected := range manifest.Rows {
		if expected.Disposition != "blocked" {
			continue
		}
		environment := expected.Environment
		if strings.HasPrefix(environment, "vm-") {
			environment = "vm"
		}
		var matches []providermatrix.Row
		for _, row := range matrix.Rows {
			if row.CapabilityID == expected.CapabilityID && row.Backend == expected.Backend &&
				row.ContractRevision == expected.ContractRevision && row.Distribution == expected.Distribution &&
				row.Release == expected.Release && row.Architecture == expected.Architecture && row.Environment == environment {
				matches = append(matches, row)
			}
		}
		if len(matches) != 1 {
			t.Errorf("%s/%s exact matrix rows = %d, want 1", expected.CapabilityID, expected.Backend, len(matches))
			continue
		}
		if matches[0].Status != "untested" {
			t.Errorf("%s/%s status = %q, want untested", expected.CapabilityID, expected.Backend, matches[0].Status)
		}
		if !slices.Equal(matches[0].Selectors, expected.Selectors) {
			t.Errorf("%s/%s selectors = %v, want %v", expected.CapabilityID, expected.Backend, matches[0].Selectors, expected.Selectors)
		}
	}
}

func TestExactMatrixSelectorsAreRunnableFromCleanEnvironment(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	matrix, err := providermatrix.Load(filepath.Join(repositoryRoot, "test", "provider-matrix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, row := range matrix.Rows {
		for _, selector := range row.Selectors {
			if seen[selector] {
				continue
			}
			seen[selector] = true
			name, args, err := providermatrix.ResolveSelector(selector)
			if err != nil {
				t.Errorf("%s: %v", selector, err)
				continue
			}
			switch name {
			case "make":
				args = []string{"-n", "--no-print-directory", args[0]}
			case "go":
				args = []string{"test", "-mod=vendor", args[2], "-list", args[4]}
			}
			command := exec.Command(name, args...)
			command.Dir = repositoryRoot
			command.Env = []string{
				"PATH=" + os.Getenv("PATH"),
				"GOCACHE=" + t.TempDir(),
				"GOTMPDIR=" + t.TempDir(),
			}
			if output, err := command.CombinedOutput(); err != nil {
				t.Errorf("selector %q is not runnable from a clean environment: %v\n%s", selector, err, output)
			}
		}
	}
}
