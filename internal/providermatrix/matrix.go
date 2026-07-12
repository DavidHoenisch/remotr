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
	Version int   `yaml:"version"`
	Rows    []Row `yaml:"rows"`
}

// Row identifies one provider/backend combination with sufficient evidence to
// claim support in a specified distribution environment.
type Row struct {
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
	Provider         string
	Distribution     string
	Release          string
	Architecture     string
	Backend          string
	ContractRevision string
	Environment      string
}

// Load reads and validates a provider evidence matrix.
func Load(path string) (Matrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Matrix{}, err
	}
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
	}
	return nil
}

// Advertised reports whether the matrix contains passing evidence for claim.
// Untested and failing rows are intentionally not product support promises.
func Advertised(matrix Matrix, claim Claim) bool {
	for _, row := range matrix.Rows {
		if row.Status != "passing" {
			continue
		}
		if row.Provider == claim.Provider &&
			row.Distribution == claim.Distribution &&
			row.Release == claim.Release &&
			row.Architecture == claim.Architecture &&
			row.Backend == claim.Backend &&
			row.ContractRevision == claim.ContractRevision &&
			row.Environment == claim.Environment {
			return true
		}
	}
	return false
}

func rowKey(row Row) string {
	values := []string{
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
