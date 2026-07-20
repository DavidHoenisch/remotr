package capabilitydoc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/providerregistry"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// Generator derives capability evidence from the same immutable registries
// used to parse resources and choose runtime providers.
type Generator struct {
	resources              *resourceregistry.Registry
	providers              *providerregistry.Registry
	providerMatrix         *providermatrix.Matrix
	artifactSchemaVersions []int
}

// NewDefaultGeneratorWithProviderMatrix constructs a generator whose native
// package, repository, trust, and AUR declarations are bounded by exact
// passing evidence rows. The matrix is validated before it can influence a
// capability document.
func NewDefaultGeneratorWithProviderMatrix(artifactSchemaVersions []int, matrix providermatrix.Matrix) (*Generator, error) {
	generator, err := NewDefaultGenerator(artifactSchemaVersions)
	if err != nil {
		return nil, err
	}
	if err := providermatrix.Validate(matrix); err != nil {
		return nil, fmt.Errorf("provider capability matrix: %w", err)
	}
	generator.providerMatrix = &matrix
	return generator, nil
}

// NewDefaultGenerator constructs the modern agent generator from all currently
// registered resource and provider contracts.
func NewDefaultGenerator(artifactSchemaVersions []int) (*Generator, error) {
	resources, err := resourceregistry.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("resource capability registry: %w", err)
	}
	providers, err := providerregistry.NewDefault()
	if err != nil {
		return nil, fmt.Errorf("provider capability registry: %w", err)
	}
	providerMatrix, err := providermatrix.Default()
	if err != nil {
		return nil, fmt.Errorf("provider capability matrix: %w", err)
	}
	return &Generator{
		resources:              resources,
		providers:              providers,
		providerMatrix:         &providerMatrix,
		artifactSchemaVersions: append([]int(nil), artifactSchemaVersions...),
	}, nil
}

// Generate captures current normalized endpoint facts and the contracts that
// those facts make available. The resulting document includes its canonical
// digest and is ready for local validation.
func (g *Generator) Generate(endpoint facts.Facts, agentVersion string) (Document, error) {
	if g == nil || g.resources == nil || g.providers == nil {
		return Document{}, fmt.Errorf("capability document generator is not configured")
	}
	endpoint = endpoint.Normalized()
	document := Document{
		DocumentVersion:        CurrentDocumentVersion,
		ArtifactSchemaVersions: append([]int(nil), g.artifactSchemaVersions...),
		AgentVersion:           agentVersion,
	}
	for _, definition := range g.resources.Definitions() {
		document.Capabilities = append(document.Capabilities, Capability{
			ID:       ResourceCapabilityID(string(definition.Kind)),
			Revision: definition.ProviderContractRevision,
		})
	}
	for _, definition := range g.providers.Definitions(endpoint) {
		if definition.Capability == providerregistry.CapabilityPackage && (definition.ID == "apt" || definition.ID == "pacman") {
			continue
		}
		document.Capabilities = append(document.Capabilities, Capability{
			ID:       "provider:" + string(definition.Capability) + "/" + definition.ID,
			Revision: definition.ContractRevision,
		})
	}
	document.Capabilities = append(document.Capabilities, g.qualifiedPackageCapabilities(endpoint)...)
	document.Facts = normalizedFacts(endpoint)
	sort.Slice(document.Capabilities, func(i, j int) bool {
		if document.Capabilities[i].ID == document.Capabilities[j].ID {
			return document.Capabilities[i].Revision < document.Capabilities[j].Revision
		}
		return document.Capabilities[i].ID < document.Capabilities[j].ID
	})
	sort.Slice(document.Facts, func(i, j int) bool {
		if document.Facts[i].Key == document.Facts[j].Key {
			return document.Facts[i].Value < document.Facts[j].Value
		}
		return document.Facts[i].Key < document.Facts[j].Key
	})
	document.Facts = deduplicateFacts(document.Facts)
	document, err := document.WithCanonicalDigest()
	if err != nil {
		return Document{}, err
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (g *Generator) qualifiedPackageCapabilities(endpoint facts.Facts) []Capability {
	if g.providerMatrix == nil {
		return nil
	}
	architecture := ""
	if endpoint.Arch == types.X86 {
		architecture = "amd64"
	}
	if architecture == "" {
		return nil
	}
	base := providermatrix.Claim{
		Distribution:     strings.ToLower(string(endpoint.Distro)),
		Release:          strings.TrimSpace(endpoint.DistroVersion),
		Architecture:     architecture,
		ContractRevision: "v1",
		Environment:      "container",
	}
	var declarations []Capability
	appendIfQualified := func(provider, backend, id string, features []string) {
		claim := base
		claim.Provider = provider
		claim.Backend = backend
		if providermatrix.Advertised(*g.providerMatrix, claim) {
			declarations = append(declarations, Capability{ID: id, Revision: "1", Features: append([]string(nil), features...)})
		}
	}
	switch endpoint.Distro {
	case types.Debian, types.Ubuntu:
		appendIfQualified("package", "apt", "provider:package/apt", aptPackageFeatures)
		before := len(declarations)
		appendIfQualified("repository", "apt", "provider:repository/apt", aptRepositoryFeatures)
		if len(declarations) > before {
			declarations = append(declarations, Capability{ID: "provider:trust/apt", Revision: "1", Features: append([]string(nil), aptTrustFeatures...)})
		}
	case types.Arch:
		appendIfQualified("package", "pacman", "provider:package/pacman", pacmanPackageFeatures)
		appendIfQualified("package", "yay", "provider:package/yay", aurPackageFeatures)
		before := len(declarations)
		appendIfQualified("repository", "pacman", "provider:repository/pacman", pacmanRepositoryFeatures)
		if len(declarations) > before {
			declarations = append(declarations, Capability{ID: "provider:trust/pacman", Revision: "1", Features: append([]string(nil), pacmanTrustFeatures...)})
		}
	}
	return declarations
}

var (
	aptPackageFeatures = []string{
		"lifecycle:absent", "lifecycle:present", "lifecycle:purged", "policy:downgrade", "policy:hold",
		"policy:noninteractive", "policy:refresh-cache", "policy:remove-dependencies", "policy:upgrade", "version:exact",
	}
	pacmanPackageFeatures = []string{
		"lifecycle:absent", "lifecycle:present", "policy:downgrade", "policy:noninteractive",
		"policy:refresh-cache", "policy:remove-dependencies", "policy:upgrade", "version:exact",
	}
	aurPackageFeatures = []string{
		"aur:artifact-digest", "aur:exact-artifact-install", "aur:source-identity", "aur:unprivileged-build",
		"lifecycle:absent", "lifecycle:present", "policy:downgrade", "policy:noninteractive", "policy:upgrade", "version:exact",
	}
	aptRepositoryFeatures = []string{
		"repository:absent", "repository:architecture", "repository:disabled", "repository:present",
		"repository:priority", "repository:scoped-credentials", "repository:signature-policy",
	}
	aptTrustFeatures         = []string{"trust:full-fingerprint", "trust:scoped-keyring"}
	pacmanRepositoryFeatures = []string{
		"repository:absent", "repository:architecture", "repository:disabled", "repository:present", "repository:signature-policy",
	}
	pacmanTrustFeatures = []string{"trust:full-fingerprint", "trust:provider-native-local-sign"}
)

// ResourceCapabilityID maps the canonical camel-case YAML resource kind to a
// lowercase protocol identifier without maintaining a second resource list.
func ResourceCapabilityID(kind string) string {
	var identifier strings.Builder
	identifier.WriteString("resource:")
	for index, character := range strings.TrimSpace(kind) {
		if character >= 'A' && character <= 'Z' {
			if index > 0 {
				identifier.WriteByte('-')
			}
			character += 'a' - 'A'
		}
		identifier.WriteRune(character)
	}
	return identifier.String()
}

func normalizedFacts(endpoint facts.Facts) []Fact {
	values := []Fact{
		{Key: "distro", Value: lower(endpoint.Distro)},
		{Key: "distro-family", Value: lower(endpoint.DistroFamily)},
		{Key: "distro-version", Value: lower(endpoint.DistroVersion)},
		{Key: "architecture", Value: lower(endpoint.Arch)},
		{Key: "init", Value: lower(endpoint.Init)},
		{Key: "package", Value: lower(endpoint.Package)},
		{Key: "firewall", Value: lower(endpoint.Firewall)},
		{Key: "network", Value: lower(endpoint.Network)},
		{Key: "security", Value: lower(endpoint.Security)},
	}
	for _, backend := range endpoint.Desktop {
		values = append(values, Fact{Key: "desktop", Value: lower(backend)})
	}
	for _, browser := range endpoint.Browser {
		values = append(values, Fact{Key: "browser", Value: lower(browser)})
	}
	out := values[:0]
	for _, fact := range values {
		if fact.Value != "" {
			out = append(out, fact)
		}
	}
	return out
}

func lower(value any) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
}

func deduplicateFacts(input []Fact) []Fact {
	if len(input) < 2 {
		return input
	}
	output := input[:1]
	for _, fact := range input[1:] {
		if fact != output[len(output)-1] {
			output = append(output, fact)
		}
	}
	return output
}
