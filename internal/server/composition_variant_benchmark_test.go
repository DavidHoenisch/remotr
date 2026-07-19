package server

import (
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/artifactvariant"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
)

var benchmarkSelectedVariant artifactvariant.Variant

func BenchmarkCapabilityVariantSelection400Endpoints(b *testing.B) {
	requirement := artifactrequirements.Requirement{ID: "resource:package", Revision: "package-v1"}
	provider := artifactrequirements.Requirement{ID: "provider:package/apt", Revision: "1"}
	variants := make([]artifactvariant.Variant, 0, 2)
	for _, schemaVersion := range []int{1, 0} {
		set := artifactrequirements.Set{
			Version: artifactrequirements.CurrentVersion, ArtifactSchemaVersion: schemaVersion,
			ResourceCapabilities: []artifactrequirements.Requirement{requirement},
			ProviderCapabilities: []artifactrequirements.Requirement{provider},
		}
		digest, err := set.CanonicalDigest()
		if err != nil {
			b.Fatal(err)
		}
		variants = append(variants, artifactvariant.Variant{SchemaVersion: schemaVersion, Requirements: set, RequirementDigest: digest})
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{
			{ID: requirement.ID, Revision: requirement.Revision},
			{ID: provider.ID, Revision: provider.Revision},
		},
		Facts: []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for endpoint := 0; endpoint < 400; endpoint++ {
			selected, _, ok := artifactvariant.SelectHighestCompatible(variants, document)
			if !ok {
				b.Fatal("compatible document was blocked")
			}
			benchmarkSelectedVariant = selected
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(variants)), "variant_count")
	b.ReportMetric(400, "endpoint_population")
}
