package artifactvariant

import (
	"fmt"
	"sort"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
)

// MissingRequirement is one bounded schema or contract requirement absent
// from current authenticated endpoint evidence.
type MissingRequirement struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

// SelectHighestCompatible returns the complete highest-schema variant
// satisfied by the current document. It never transforms artifact bytes or
// manufactures a capability-specific variant.
func SelectHighestCompatible(variants []Variant, document capabilitydoc.Document) (Variant, []MissingRequirement, bool) {
	if err := document.Validate(); err != nil {
		return Variant{}, nil, false
	}
	candidates := append([]Variant(nil), variants...)
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].SchemaVersion > candidates[j].SchemaVersion
	})
	supportedSchemas := make(map[int]bool, len(document.ArtifactSchemaVersions))
	for _, schemaVersion := range document.ArtifactSchemaVersions {
		supportedSchemas[schemaVersion] = true
	}
	supportedContracts := make(map[string]string, len(document.Capabilities))
	for _, capability := range document.Capabilities {
		supportedContracts[capability.ID] = capability.Revision
	}

	var bestMissing []MissingRequirement
	for _, candidate := range candidates {
		missing, valid := missingRequirements(candidate, supportedSchemas, supportedContracts)
		if !valid {
			continue
		}
		if len(missing) == 0 {
			return cloneVariant(candidate), nil, true
		}
		if bestMissing == nil || len(missing) < len(bestMissing) {
			bestMissing = missing
		}
	}
	return Variant{}, bestMissing, false
}

func missingRequirements(variant Variant, schemas map[int]bool, contracts map[string]string) ([]MissingRequirement, bool) {
	if variant.SchemaVersion != variant.Requirements.ArtifactSchemaVersion || variant.Requirements.Validate() != nil {
		return nil, false
	}
	actualDigest, err := variant.Requirements.CanonicalDigest()
	if err != nil || actualDigest != variant.RequirementDigest {
		return nil, false
	}
	var missing []MissingRequirement
	if !schemas[variant.SchemaVersion] {
		missing = append(missing, MissingRequirement{ID: fmt.Sprintf("schema:%d", variant.SchemaVersion), Revision: "1"})
	}
	appendMissing := func(requirements []artifactrequirements.Requirement) {
		for _, requirement := range requirements {
			if contracts[requirement.ID] != requirement.Revision {
				missing = append(missing, MissingRequirement{ID: requirement.ID, Revision: requirement.Revision})
			}
		}
	}
	appendMissing(variant.Requirements.ResourceCapabilities)
	appendMissing(variant.Requirements.ProviderCapabilities)
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].ID == missing[j].ID {
			return missing[i].Revision < missing[j].Revision
		}
		return missing[i].ID < missing[j].ID
	})
	return missing, true
}

func cloneVariant(variant Variant) Variant {
	variant.Artifact = append([]byte(nil), variant.Artifact...)
	variant.Requirements.ResourceCapabilities = append([]artifactrequirements.Requirement(nil), variant.Requirements.ResourceCapabilities...)
	variant.Requirements.ProviderCapabilities = append([]artifactrequirements.Requirement(nil), variant.Requirements.ProviderCapabilities...)
	return variant
}
