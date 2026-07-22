package artifactvariant

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
)

func TestSelectionUsesMostSpecificMatchingTargetWithoutFallback(t *testing.T) {
	portable := testVariant(t, nil, nil)
	ubuntu := testVariant(t, &artifactrequirements.TargetPredicate{Distros: []string{"ubuntu"}}, []artifactrequirements.Requirement{{ID: "provider:init/systemd", Revision: "1"}})
	ubuntuX86 := testVariant(t, &artifactrequirements.TargetPredicate{Distros: []string{"ubuntu"}, Architectures: []string{"x86"}}, []artifactrequirements.Requirement{{ID: "provider:package/apt", Revision: "1"}})
	document := testDocument(t, []capabilitydoc.Fact{{Key: "distro", Value: "ubuntu"}, {Key: "architecture", Value: "x86"}}, nil)

	selected, missing, ok := SelectHighestCompatible([]Variant{portable, ubuntu, ubuntuX86}, document)
	if ok || len(selected.Artifact) != 0 || !slices.Contains(missing, MissingRequirement{ID: "provider:package/apt", Revision: "1"}) {
		t.Fatalf("selection bypassed exact target: selected=%+v missing=%+v ok=%t", selected, missing, ok)
	}
}

func TestSelectionReportsNoMatchingTarget(t *testing.T) {
	ubuntu := testVariant(t, &artifactrequirements.TargetPredicate{Distros: []string{"ubuntu"}}, nil)
	document := testDocument(t, []capabilitydoc.Fact{{Key: "distro", Value: "arch"}}, nil)
	_, missing, ok := SelectHighestCompatible([]Variant{ubuntu}, document)
	if ok || !slices.Equal(missing, []MissingRequirement{{ID: "target:artifact", Revision: "1"}}) {
		t.Fatalf("no-match selection missing=%+v ok=%t", missing, ok)
	}
}

func FuzzSelectionDoesNotBypassSpecificTarget(f *testing.F) {
	f.Add(uint8(0), uint8(0), false)
	f.Add(uint8(2), uint8(1), true)
	f.Fuzz(func(t *testing.T, distroIndex, architectureIndex uint8, supportsExact bool) {
		distros := []string{"ubuntu", "debian", "arch"}
		architectures := []string{"x86", "arm"}
		distro := distros[int(distroIndex)%len(distros)]
		architecture := architectures[int(architectureIndex)%len(architectures)]
		exactRequirement := artifactrequirements.Requirement{ID: "provider:package/apt", Revision: "1"}
		variants := []Variant{
			testVariant(t, nil, nil),
			testVariant(t, &artifactrequirements.TargetPredicate{Distros: []string{distro}}, nil),
			testVariant(t, &artifactrequirements.TargetPredicate{Distros: []string{distro}, Architectures: []string{architecture}}, []artifactrequirements.Requirement{exactRequirement}),
		}
		var capabilities []capabilitydoc.Capability
		if supportsExact {
			capabilities = []capabilitydoc.Capability{{ID: exactRequirement.ID, Revision: exactRequirement.Revision}}
		}
		document := testDocument(t, []capabilitydoc.Fact{{Key: "distro", Value: distro}, {Key: "architecture", Value: architecture}}, capabilities)
		_, missing, ok := SelectHighestCompatible(variants, document)
		if ok != supportsExact {
			t.Fatalf("selection ok=%t, want %t, missing=%+v", ok, supportsExact, missing)
		}
		if !supportsExact && !slices.Contains(missing, MissingRequirement{ID: exactRequirement.ID, Revision: exactRequirement.Revision}) {
			t.Fatalf("specific target requirement bypassed: %+v", missing)
		}
	})
}

func BenchmarkSelectMixedTargetArtifactVariant(b *testing.B) {
	variants := []Variant{
		testVariant(b, nil, nil),
		testVariant(b, &artifactrequirements.TargetPredicate{Distros: []string{"ubuntu"}}, nil),
		testVariant(b, &artifactrequirements.TargetPredicate{Distros: []string{"ubuntu"}, Architectures: []string{"x86"}}, []artifactrequirements.Requirement{{ID: "provider:package/apt", Revision: "1"}}),
		testVariant(b, &artifactrequirements.TargetPredicate{Distros: []string{"arch"}, Architectures: []string{"x86"}}, []artifactrequirements.Requirement{{ID: "provider:package/pacman", Revision: "1"}}),
	}
	document := testDocument(b, []capabilitydoc.Fact{{Key: "distro", Value: "ubuntu"}, {Key: "architecture", Value: "x86"}}, []capabilitydoc.Capability{{ID: "provider:package/apt", Revision: "1"}})
	b.ReportAllocs()
	for b.Loop() {
		if _, _, ok := SelectHighestCompatible(variants, document); !ok {
			b.Fatal("compatible target was blocked")
		}
	}
}

type testingTB interface {
	Helper()
	Fatal(...any)
}

func testVariant(tb testingTB, target *artifactrequirements.TargetPredicate, providers []artifactrequirements.Requirement) Variant {
	tb.Helper()
	set := artifactrequirements.Set{
		Version: artifactrequirements.CurrentVersion, ArtifactSchemaVersion: 1,
		Target: target, ProviderCapabilities: providers,
	}
	digest, err := set.CanonicalDigest()
	if err != nil {
		tb.Fatal(err)
	}
	return Variant{Artifact: []byte("complete"), Digest: "sha256:artifact", SchemaVersion: 1, Requirements: set, RequirementDigest: digest}
}

func testDocument(tb testingTB, facts []capabilitydoc.Fact, capabilities []capabilitydoc.Capability) capabilitydoc.Document {
	tb.Helper()
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1",
		Facts: facts, Capabilities: capabilities,
	}).WithCanonicalDigest()
	if err != nil {
		tb.Fatal(err)
	}
	return document
}
