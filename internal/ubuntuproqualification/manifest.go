// Package ubuntuproqualification validates the exact Ubuntu Pro qualification
// inventory. Inventory rows are evidence obligations, not runtime discovery:
// only an independently passing exact row can advertise its capability.
package ubuntuproqualification

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"
)

const maxManifestBytes = 4 << 20

type Manifest struct {
	Version        int             `yaml:"version"`
	Change         string          `yaml:"change"`
	BaseRows       []BaseRow       `yaml:"base_rows"`
	CapabilityRows []CapabilityRow `yaml:"capability_rows"`
	NegativeCases  []NegativeCase  `yaml:"negative_cases"`
	NonClaims      []NonClaim      `yaml:"non_claims"`
}

type BaseRow struct {
	Distribution      string   `yaml:"distribution"`
	Release           string   `yaml:"release"`
	Architecture      string   `yaml:"architecture"`
	APIRevision       string   `yaml:"api_revision"`
	Status            string   `yaml:"status"`
	RequiredSelectors []string `yaml:"required_selectors"`
}

type CapabilityRow struct {
	ID                string   `yaml:"id"`
	Capability        string   `yaml:"capability"`
	Kind              string   `yaml:"kind"`
	Service           string   `yaml:"service"`
	Value             string   `yaml:"value,omitempty"`
	Distribution      string   `yaml:"distribution"`
	Release           string   `yaml:"release"`
	Architecture      string   `yaml:"architecture"`
	APIRevision       string   `yaml:"api_revision"`
	Status            string   `yaml:"status"`
	RequiredSelectors []string `yaml:"required_selectors"`
}

type NegativeCase struct {
	ID                string   `yaml:"id"`
	Reason            string   `yaml:"reason"`
	RequiredSelectors []string `yaml:"required_selectors"`
}

type NonClaim struct {
	ID     string `yaml:"id"`
	Reason string `yaml:"reason"`
}

type Target struct {
	Distribution string
	Release      string
	Architecture string
	APIRevision  string
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-selected local qualification manifest; content is bounded and strictly decoded below.
	if err != nil {
		return Manifest{}, err
	}
	return Decode(data)
}

func Decode(data []byte) (Manifest, error) {
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("Ubuntu Pro qualification manifest exceeds %d bytes", maxManifestBytes)
	}
	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode Ubuntu Pro qualification manifest: %w", err)
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func Validate(manifest Manifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("Ubuntu Pro qualification version = %d, want 1", manifest.Version)
	}
	if manifest.Change != "add-ubuntu-pro-management" {
		return fmt.Errorf("Ubuntu Pro qualification change = %q", manifest.Change)
	}
	baseKeys := make(map[string]bool, len(manifest.BaseRows))
	for _, row := range manifest.BaseRows {
		if row.Distribution != "ubuntu" || row.Architecture != "amd64" || row.APIRevision != "ubuntu-pro-api-v32" {
			return fmt.Errorf("inexact Ubuntu Pro base row: %+v", row)
		}
		if !validStatus(row.Status) {
			return fmt.Errorf("base row %s has invalid status %q", row.Release, row.Status)
		}
		key := targetKey(row.Distribution, row.Release, row.Architecture, row.APIRevision)
		if baseKeys[key] {
			return fmt.Errorf("duplicate Ubuntu Pro base row %q", key)
		}
		baseKeys[key] = true
		if len(row.RequiredSelectors) == 0 {
			return fmt.Errorf("base row %q has no evidence selectors", key)
		}
	}
	capabilityIDs := make(map[string]bool, len(manifest.CapabilityRows))
	for _, row := range manifest.CapabilityRows {
		if row.ID == "" || row.Capability == "" || row.Service == "" || !validCapabilityKind(row.Kind) {
			return fmt.Errorf("invalid Ubuntu Pro capability row: %+v", row)
		}
		if expected := capabilityID(row); row.Capability != expected {
			return fmt.Errorf("capability row %q names %q, want canonical %q", row.ID, row.Capability, expected)
		}
		if row.Distribution != "ubuntu" || row.Architecture != "amd64" || row.APIRevision != "ubuntu-pro-api-v32" {
			return fmt.Errorf("inexact Ubuntu Pro capability row %q", row.ID)
		}
		if !validStatus(row.Status) {
			return fmt.Errorf("capability row %q has invalid status %q", row.ID, row.Status)
		}
		if capabilityIDs[row.ID] {
			return fmt.Errorf("duplicate Ubuntu Pro capability row %q", row.ID)
		}
		capabilityIDs[row.ID] = true
		if len(row.RequiredSelectors) == 0 {
			return fmt.Errorf("capability row %q has no evidence selectors", row.ID)
		}
	}
	if err := validateNamedInventory("negative case", negativeIDs(manifest.NegativeCases)); err != nil {
		return err
	}
	if err := validateNamedInventory("non-claim", nonClaimIDs(manifest.NonClaims)); err != nil {
		return err
	}
	return nil
}

func (manifest Manifest) Clone() Manifest {
	clone := manifest
	clone.BaseRows = slices.Clone(manifest.BaseRows)
	for index := range clone.BaseRows {
		clone.BaseRows[index].RequiredSelectors = slices.Clone(manifest.BaseRows[index].RequiredSelectors)
	}
	clone.CapabilityRows = slices.Clone(manifest.CapabilityRows)
	for index := range clone.CapabilityRows {
		clone.CapabilityRows[index].RequiredSelectors = slices.Clone(manifest.CapabilityRows[index].RequiredSelectors)
	}
	clone.NegativeCases = slices.Clone(manifest.NegativeCases)
	for index := range clone.NegativeCases {
		clone.NegativeCases[index].RequiredSelectors = slices.Clone(manifest.NegativeCases[index].RequiredSelectors)
	}
	clone.NonClaims = slices.Clone(manifest.NonClaims)
	return clone
}

func (manifest Manifest) BaseRow(release, architecture, apiRevision string) (BaseRow, bool) {
	for _, row := range manifest.BaseRows {
		if row.Release == release && row.Architecture == architecture && row.APIRevision == apiRevision {
			return row, true
		}
	}
	return BaseRow{}, false
}

func (manifest Manifest) HasNegativeCase(id string) bool {
	return slices.Contains(negativeIDs(manifest.NegativeCases), id)
}

func (manifest Manifest) HasNonClaim(id string) bool {
	return slices.Contains(nonClaimIDs(manifest.NonClaims), id)
}

func (manifest Manifest) HasCapabilityTuple(kind, service, value, release, architecture, apiRevision string) bool {
	for _, row := range manifest.CapabilityRows {
		if row.Kind == kind && row.Service == service && row.Value == value && row.Release == release && row.Architecture == architecture && row.APIRevision == apiRevision {
			return true
		}
	}
	return false
}

func (manifest Manifest) AdvertisedCapabilities(target Target) []string {
	capabilities := make([]string, 0)
	for _, row := range manifest.BaseRows {
		if row.Status == "passing" && matches(target, row.Distribution, row.Release, row.Architecture, row.APIRevision) {
			capabilities = append(capabilities, "resource:ubuntu-pro")
		}
	}
	for _, row := range manifest.CapabilityRows {
		if row.Status == "passing" && matches(target, row.Distribution, row.Release, row.Architecture, row.APIRevision) {
			capabilities = append(capabilities, row.Capability)
		}
	}
	sort.Strings(capabilities)
	return capabilities
}

func matches(target Target, distribution, release, architecture, apiRevision string) bool {
	return target.Distribution == distribution && target.Release == release && target.Architecture == architecture && target.APIRevision == apiRevision
}

func targetKey(distribution, release, architecture, apiRevision string) string {
	return distribution + "/" + release + "/" + architecture + "/" + apiRevision
}

func validStatus(status string) bool {
	return status == "untested" || status == "passing" || status == "unsupported"
}

func validCapabilityKind(kind string) bool {
	switch kind {
	case "service", "enable-mode", "variant", "disable-behavior":
		return true
	default:
		return false
	}
}

func capabilityID(row CapabilityRow) string {
	switch row.Kind {
	case "service":
		return "provider:ubuntu-pro-service/" + row.Service
	case "enable-mode":
		return "provider:ubuntu-pro-option/" + row.Service + "/" + row.Value
	case "variant":
		return "provider:ubuntu-pro-variant/" + row.Service + "/" + row.Value
	case "disable-behavior":
		return "provider:ubuntu-pro-disable/" + row.Service + "/" + row.Value
	default:
		return ""
	}
}

func negativeIDs(cases []NegativeCase) []string {
	ids := make([]string, 0, len(cases))
	for _, item := range cases {
		ids = append(ids, item.ID)
	}
	return ids
}

func nonClaimIDs(claims []NonClaim) []string {
	ids := make([]string, 0, len(claims))
	for _, item := range claims {
		ids = append(ids, item.ID)
	}
	return ids
}

func validateNamedInventory(kind string, ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("%s has empty ID", kind)
		}
		if seen[id] {
			return fmt.Errorf("duplicate %s %q", kind, id)
		}
		seen[id] = true
	}
	return nil
}
