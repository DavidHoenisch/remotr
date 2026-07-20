// Package providermatrix loads and validates real-environment provider
// evidence. An empty valid matrix intentionally advertises no support.
package providermatrix

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Matrix is versioned so support evidence can evolve without reinterpreting
// existing distribution, backend, or environment claims.
type Matrix struct {
	Version      int             `yaml:"version"`
	Dependencies DependencyGates `yaml:"dependencies"`
	Rows         []Row           `yaml:"rows"`
}

// DependencyGates records the shared workstreams accepted by the release.
// Provider evidence remains in Rows when a gate is false, but it cannot be
// converted into an endpoint support claim.
type DependencyGates struct {
	ExecutionContract  bool `yaml:"execution_contract"`
	CapabilityDelivery bool `yaml:"capability_delivery"`
	TestingFoundation  bool `yaml:"testing_foundation"`
	PackageProviders   bool `yaml:"package_providers"`
}

// AcceptedDependencyGates returns the fully accepted release state used by
// focused fixtures that are testing row qualification rather than dependency
// blocking.
func AcceptedDependencyGates() DependencyGates {
	return DependencyGates{
		ExecutionContract:  true,
		CapabilityDelivery: true,
		TestingFoundation:  true,
		PackageProviders:   true,
	}
}

// Row identifies one provider/backend combination with sufficient evidence to
// claim support in a specified distribution environment.
type Row struct {
	CapabilityID     string   `yaml:"capability_id"`
	Provider         string   `yaml:"provider"`
	Distribution     string   `yaml:"distribution"`
	Release          string   `yaml:"release"`
	Architecture     string   `yaml:"architecture"`
	Backend          string   `yaml:"backend"`
	ContractRevision string   `yaml:"contract_revision"`
	Environment      string   `yaml:"environment"`
	Status           string   `yaml:"status"`
	Selectors        []string `yaml:"selectors"`
}

// Claim identifies the exact provider environment a capability would advertise.
// A claim is available only when its matching matrix row has passing evidence.
type Claim struct {
	CapabilityID     string
	Provider         string
	Distribution     string
	Release          string
	Architecture     string
	Backend          string
	ContractRevision string
	Environment      string
}

// SelectorRunner resolves and executes one evidence selector. A successful
// return means that selector's complete evidence target passed now; a label in
// the matrix is never treated as execution evidence by itself.
type SelectorRunner func(selector string) error

// Load reads and validates a provider evidence matrix.
func Load(path string) (Matrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Matrix{}, err
	}
	return Decode(data)
}

// Decode parses and validates provider evidence from a bounded caller-owned
// source such as the repository's embedded default matrix.
func Decode(data []byte) (Matrix, error) {
	var matrix Matrix
	if err := yaml.Unmarshal(data, &matrix); err != nil {
		return Matrix{}, fmt.Errorf("decode provider matrix: %w", err)
	}
	if err := Validate(matrix); err != nil {
		return Matrix{}, err
	}
	return matrix, nil
}

// Validate rejects incomplete, duplicate, or unknown-environment evidence
// rows. A version-1 matrix may be empty until real environment evidence exists.
func Validate(matrix Matrix) error {
	if matrix.Version != 1 {
		return fmt.Errorf("provider matrix version = %d, want 1", matrix.Version)
	}
	seen := make(map[string]struct{}, len(matrix.Rows))
	for index, row := range matrix.Rows {
		if err := validateRow(row); err != nil {
			return fmt.Errorf("provider matrix row %d: %w", index+1, err)
		}
		key := rowKey(row)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("provider matrix row %d: duplicate evidence row %s", index+1, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateRow(row Row) error {
	for field, value := range map[string]string{
		"capability_id":     row.CapabilityID,
		"provider":          row.Provider,
		"distribution":      row.Distribution,
		"release":           row.Release,
		"architecture":      row.Architecture,
		"backend":           row.Backend,
		"contract_revision": row.ContractRevision,
		"environment":       row.Environment,
		"status":            row.Status,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", field)
		}
	}
	if row.Environment != "container" && row.Environment != "vm" {
		return fmt.Errorf("environment %q is not supported (want container or vm)", row.Environment)
	}
	if row.Status != "passing" && row.Status != "untested" && row.Status != "failing" {
		return fmt.Errorf("status %q is not supported (want passing, untested, or failing)", row.Status)
	}
	if len(row.Selectors) == 0 {
		return fmt.Errorf("at least one selector is required")
	}
	for _, selector := range row.Selectors {
		if strings.TrimSpace(selector) == "" {
			return fmt.Errorf("selectors must not be empty")
		}
		if _, _, err := ResolveSelector(selector); err != nil {
			return err
		}
	}
	if row.Status == "passing" {
		if err := validatePassingEvidenceSet(row); err != nil {
			return err
		}
	}
	return nil
}

// VerifyClaim validates the matrix, finds the exact passing row for claim, and
// runs every selector in that row. Capability publication must use this seam so
// stale labels or superficially passing tests cannot become support claims.
func VerifyClaim(matrix Matrix, claim Claim, run SelectorRunner) error {
	if err := Validate(matrix); err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("selector runner is required")
	}
	for _, row := range matrix.Rows {
		if row.Status != "passing" || !matches(row, claim) {
			continue
		}
		for _, selector := range row.Selectors {
			if err := run(selector); err != nil {
				return fmt.Errorf("provider evidence %s selector %q: %w", rowKey(row), selector, err)
			}
		}
		return nil
	}
	return fmt.Errorf("no matching passing provider-matrix evidence")
}

// Advertised reports whether the matrix contains passing evidence for claim.
// Untested and failing rows are intentionally not product support promises.
func Advertised(matrix Matrix, claim Claim) bool {
	if Validate(matrix) != nil {
		return false
	}
	for _, row := range matrix.Rows {
		if row.Status != "passing" {
			continue
		}
		if validatePassingEvidenceSet(row) == nil && matches(row, claim) {
			return true
		}
	}
	return false
}

// AdvertisedForPublication reports whether recorded passing evidence may be
// emitted as an endpoint capability after the shared release dependencies are
// accepted. Package and repository claims additionally depend on the package
// provider workstream.
func AdvertisedForPublication(matrix Matrix, claim Claim) bool {
	dependencies := matrix.Dependencies
	if !dependencies.ExecutionContract || !dependencies.CapabilityDelivery || !dependencies.TestingFoundation {
		return false
	}
	if (claim.CapabilityID == "package" || claim.CapabilityID == "repository") && !dependencies.PackageProviders {
		return false
	}
	return Advertised(matrix, claim)
}

func matches(row Row, claim Claim) bool {
	return row.CapabilityID == claim.CapabilityID &&
		row.Provider == claim.Provider &&
		row.Distribution == claim.Distribution &&
		row.Release == claim.Release &&
		row.Architecture == claim.Architecture &&
		row.Backend == claim.Backend &&
		row.ContractRevision == claim.ContractRevision &&
		row.Environment == claim.Environment
}

func validatePassingEvidenceSet(row Row) error {
	want, qualified := corePackageEvidenceSelector(row)
	if qualified {
		if len(row.Selectors) != 1 || row.Selectors[0] != want {
			return fmt.Errorf("passing core provider row does not contain the complete evidence set for its qualified provider identity (want selector %q)", want)
		}
		return nil
	}
	if (row.Provider == "package" || row.Provider == "repository") &&
		isNativePackageBackend(row.Backend) {
		return fmt.Errorf("passing core provider row has no qualified provider identity and complete evidence set")
	}
	return nil
}

func isNativePackageBackend(backend string) bool {
	switch backend {
	case "apt", "pacman", "yay", "dnf", "dnf4", "dnf5", "rpm", "rpm-ostree", "apk", "zypper", "snap":
		return true
	default:
		return false
	}
}

func corePackageEvidenceSelector(row Row) (string, bool) {
	if row.Architecture != "amd64" || row.ContractRevision != "v1" || row.Environment != "container" {
		return "", false
	}
	type identity struct {
		capabilityID string
		provider     string
		distribution string
		release      string
		backend      string
	}
	selectors := map[identity]string{
		{"package", "package", "debian", "12", "apt"}:                "make:provider-matrix-apt-debian-12",
		{"repository", "repository", "debian", "12", "apt"}:          "make:provider-matrix-apt-repository-debian-12",
		{"package", "package", "ubuntu", "24.04", "apt"}:             "make:provider-matrix-apt-ubuntu-24-04",
		{"repository", "repository", "ubuntu", "24.04", "apt"}:       "make:provider-matrix-apt-repository-ubuntu-24-04",
		{"package", "package", "arch", "2026-07-06", "pacman"}:       "make:provider-matrix-pacman-arch-2026-07-06",
		{"package", "package", "arch", "2026-07-06", "yay"}:          "make:provider-matrix-aur-arch-2026-07-06",
		{"repository", "repository", "arch", "2026-07-06", "pacman"}: "make:provider-matrix-pacman-repository-arch-2026-07-06",
	}
	selector, ok := selectors[identity{row.CapabilityID, row.Provider, row.Distribution, row.Release, row.Backend}]
	return selector, ok
}

func rowKey(row Row) string {
	values := []string{
		row.CapabilityID,
		row.Provider,
		row.Distribution,
		row.Release,
		row.Architecture,
		row.Backend,
		row.ContractRevision,
		row.Environment,
	}
	return strings.Join(values, "/")
}
