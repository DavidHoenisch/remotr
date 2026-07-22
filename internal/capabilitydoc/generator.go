package capabilitydoc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/providerregistry"
	"github.com/DavidHoenisch/remotr/internal/releasecatalog"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"github.com/DavidHoenisch/remotr/internal/types"
	"github.com/DavidHoenisch/remotr/internal/ubuntuproqualification"
)

// Generator derives capability evidence from the same immutable registries
// used to parse resources and choose runtime providers.
type Generator struct {
	resources              *resourceregistry.Registry
	providers              *providerregistry.Registry
	providerMatrix         *providermatrix.Matrix
	ubuntuProQualification *ubuntuproqualification.Manifest
	artifactSchemaVersions []int
}

// NewDefaultGeneratorWithUbuntuProQualification constructs a generator with
// an explicit frozen Ubuntu Pro qualification inventory. Injection keeps
// tests and publication tooling able to promote one exact evidence row without
// changing the checked-in untested inventory.
func NewDefaultGeneratorWithUbuntuProQualification(artifactSchemaVersions []int, matrix providermatrix.Matrix, qualification ubuntuproqualification.Manifest) (*Generator, error) {
	generator, err := NewDefaultGeneratorWithProviderMatrix(artifactSchemaVersions, matrix)
	if err != nil {
		return nil, err
	}
	if err := ubuntuproqualification.Validate(qualification); err != nil {
		return nil, fmt.Errorf("Ubuntu Pro qualification: %w", err)
	}
	clone := qualification.Clone()
	generator.ubuntuProQualification = &clone
	return generator, nil
}

// NewDefaultGeneratorWithProviderMatrix constructs a generator whose resource,
// native package, repository, trust, and AUR declarations are bounded by exact
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
	ubuntuProQualification, err := releasecatalog.UbuntuProQualification()
	if err != nil {
		return nil, fmt.Errorf("Ubuntu Pro release catalog: %w", err)
	}
	return &Generator{
		resources:              resources,
		providers:              providers,
		providerMatrix:         &providerMatrix,
		ubuntuProQualification: &ubuntuProQualification,
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
		Capabilities:           make([]Capability, 0),
		AgentVersion:           agentVersion,
	}
	document.Capabilities = append(document.Capabilities, g.qualifiedResourceCapabilities(endpoint)...)
	document.Capabilities = append(document.Capabilities, g.qualifiedApplicatorProviderCapabilities(endpoint)...)
	document.Capabilities = append(document.Capabilities, g.qualifiedPackageCapabilities(endpoint)...)
	document.Capabilities = append(document.Capabilities, g.qualifiedPortablePackageCapabilities(endpoint)...)
	document.Capabilities = append(document.Capabilities, g.qualifiedUbuntuProCapabilities(endpoint)...)
	document.Capabilities = append(document.Capabilities, Capability{ID: "provider:package/remotr", Revision: "1"})
	document.Facts = normalizedFacts(endpoint)
	sort.Slice(document.Capabilities, func(i, j int) bool {
		if document.Capabilities[i].ID == document.Capabilities[j].ID {
			return document.Capabilities[i].Revision < document.Capabilities[j].Revision
		}
		return document.Capabilities[i].ID < document.Capabilities[j].ID
	})
	document.Capabilities = deduplicateCapabilities(document.Capabilities)
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

func (g *Generator) qualifiedUbuntuProCapabilities(endpoint facts.Facts) []Capability {
	if g.ubuntuProQualification == nil || !endpoint.ExactUbuntu() {
		return nil
	}
	target := ubuntuproqualification.Target{
		Distribution: "ubuntu",
		Release:      strings.TrimSpace(endpoint.DistroVersion),
		Architecture: matrixArchitecture(endpoint.Arch),
		APIRevision:  "ubuntu-pro-api-v32",
	}
	advertised := g.ubuntuProQualification.AdvertisedCapabilities(target)
	capabilities := make([]Capability, 0, len(advertised))
	for _, id := range advertised {
		revision := "1"
		if id == "resource:ubuntu-pro" {
			revision = "ubuntu-pro-v1"
		}
		capabilities = append(capabilities, Capability{ID: id, Revision: revision})
	}
	return capabilities
}

func (g *Generator) qualifiedResourceCapabilities(endpoint facts.Facts) []Capability {
	if g.providerMatrix == nil {
		return nil
	}
	distribution := strings.ToLower(string(endpoint.Distro))
	release := strings.TrimSpace(endpoint.DistroVersion)
	architecture := matrixArchitecture(endpoint.Arch)
	if distribution == "" || release == "" || architecture == "" {
		return nil
	}

	var capabilities []Capability
	for _, definition := range g.resources.Definitions() {
		capabilityID := string(definition.Kind)
		contractRevision := definition.ProviderContractRevision
		if claim, ok := packageResourceClaim(capabilityID, endpoint); ok {
			claim.Distribution = distribution
			claim.Release = release
			claim.Architecture = architecture
			if providermatrix.AdvertisedForPublication(*g.providerMatrix, claim) {
				capabilities = append(capabilities, Capability{
					ID:       ResourceCapabilityID(capabilityID),
					Revision: contractRevision,
				})
			}
			continue
		}

		for _, row := range g.providerMatrix.Rows {
			if row.CapabilityID != capabilityID || row.ContractRevision != contractRevision ||
				row.Distribution != distribution || row.Release != release || row.Architecture != architecture {
				continue
			}
			claim := providermatrix.Claim{
				CapabilityID: row.CapabilityID, Provider: row.Provider,
				Distribution: row.Distribution, Release: row.Release, Architecture: row.Architecture,
				Backend: row.Backend, ContractRevision: row.ContractRevision, Environment: row.Environment,
			}
			if providermatrix.AdvertisedForPublication(*g.providerMatrix, claim) && g.rowAppliesToEndpoint(row, endpoint) {
				capabilities = append(capabilities, Capability{
					ID:       ResourceCapabilityID(capabilityID),
					Revision: contractRevision,
				})
				break
			}
		}
	}
	return capabilities
}

func (g *Generator) qualifiedApplicatorProviderCapabilities(endpoint facts.Facts) []Capability {
	if g.providerMatrix == nil {
		return nil
	}
	distribution := strings.ToLower(string(endpoint.Distro))
	release := strings.TrimSpace(endpoint.DistroVersion)
	architecture := matrixArchitecture(endpoint.Arch)
	registered := make(map[string]string)
	for _, definition := range g.resources.Definitions() {
		registered[string(definition.Kind)] = definition.ProviderContractRevision
	}

	var capabilities []Capability
	for _, row := range g.providerMatrix.Rows {
		if registered[row.CapabilityID] != row.ContractRevision || row.Distribution != distribution ||
			row.Release != release || row.Architecture != architecture || !g.rowAppliesToEndpoint(row, endpoint) {
			continue
		}
		claim := providermatrix.Claim{
			CapabilityID: row.CapabilityID, Provider: row.Provider,
			Distribution: row.Distribution, Release: row.Release, Architecture: row.Architecture,
			Backend: row.Backend, ContractRevision: row.ContractRevision, Environment: row.Environment,
		}
		if !providermatrix.AdvertisedForPublication(*g.providerMatrix, claim) {
			continue
		}
		for _, id := range providerCapabilityIDs(row) {
			capabilities = append(capabilities, Capability{ID: id, Revision: "1"})
		}
	}
	return capabilities
}

func (g *Generator) rowAppliesToEndpoint(row providermatrix.Row, endpoint facts.Facts) bool {
	observed := make(map[string]bool)
	for _, definition := range g.providers.Definitions(endpoint) {
		observed["provider:"+string(definition.Capability)+"/"+definition.ID] = true
	}
	for _, capabilityID := range providerCapabilityIDs(row) {
		if providerCapabilityRequiresObservedFact(capabilityID) && !observed[capabilityID] {
			return false
		}
	}
	return true
}

func providerCapabilityRequiresObservedFact(capabilityID string) bool {
	for _, prefix := range []string{
		"provider:init/", "provider:firewall/", "provider:network/",
		"provider:security/", "provider:desktop/", "provider:browser/",
	} {
		if strings.HasPrefix(capabilityID, prefix) {
			return true
		}
	}
	return false
}

func providerCapabilityIDs(row providermatrix.Row) []string {
	switch row.CapabilityID {
	case "sysctl":
		return []string{"provider:kernel/sysctl"}
	case "kernelModule":
		return []string{"provider:kernel/modules"}
	case "hostname":
		return []string{"provider:host/hostnamectl"}
	case "hostLocale":
		return []string{"provider:host/localectl"}
	case "timeSync":
		return []string{"provider:time-sync/" + row.Backend}
	case "mount":
		return []string{"provider:storage/mount"}
	case "swap":
		return []string{"provider:storage/swap"}
	case "endpointSchedule":
		capabilities := []string{"provider:schedule/" + row.Backend}
		if row.Backend == "systemd-timer" {
			capabilities = append(capabilities, "provider:init/systemd")
		}
		return capabilities
	case "service":
		return []string{"provider:init/" + row.Backend}
	case "systemd", "systemdUnit", "journald":
		return []string{"provider:init/systemd"}
	case "dnsResolver", "route", "networkProfile":
		return []string{"provider:network/" + row.Backend}
	case "firewall":
		backend := strings.TrimSuffix(strings.TrimSuffix(row.Backend, "-enforcement"), "-audit")
		return []string{"provider:firewall/" + backend}
	case "appArmorProfile":
		return []string{"provider:security/apparmor"}
	case "loginPolicy":
		return []string{"provider:authentication/" + row.Backend}
	case "logrotate":
		return []string{"provider:logging/logrotate"}
	case "desktopSetting", "sessionPolicy":
		return []string{"provider:desktop/" + row.Backend}
	case "browserPolicy":
		return []string{"provider:browser/" + row.Backend}
	default:
		return nil
	}
}

func packageResourceClaim(capabilityID string, endpoint facts.Facts) (providermatrix.Claim, bool) {
	claim := providermatrix.Claim{ContractRevision: "v1", Environment: "container"}
	switch capabilityID {
	case "package":
		claim.CapabilityID = "package"
		claim.Provider = "package"
		claim.Backend = strings.ToLower(string(endpoint.Package))
	case "aptSigningKey", "aptRepository":
		if endpoint.Package != types.Apt {
			return providermatrix.Claim{}, false
		}
		claim.CapabilityID = "repository"
		claim.Provider = "repository"
		claim.Backend = "apt"
	case "pacmanSigningKey", "pacmanRepository":
		if endpoint.Package != types.Pacman {
			return providermatrix.Claim{}, false
		}
		claim.CapabilityID = "repository"
		claim.Provider = "repository"
		claim.Backend = "pacman"
	default:
		return providermatrix.Claim{}, false
	}
	return claim, claim.Backend != ""
}

func matrixArchitecture(architecture types.Architecture) string {
	if architecture == types.X86 {
		return "amd64"
	}
	return ""
}

func (g *Generator) qualifiedPackageCapabilities(endpoint facts.Facts) []Capability {
	if g.providerMatrix == nil {
		return nil
	}
	architecture := matrixArchitecture(endpoint.Arch)
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
		claim.CapabilityID = provider
		claim.Provider = provider
		claim.Backend = backend
		if providermatrix.AdvertisedForPublication(*g.providerMatrix, claim) {
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

func (g *Generator) qualifiedPortablePackageCapabilities(endpoint facts.Facts) []Capability {
	if g.providerMatrix == nil {
		return nil
	}
	base := providermatrix.Claim{
		Distribution: strings.ToLower(string(endpoint.Distro)), Release: strings.TrimSpace(endpoint.DistroVersion),
		Architecture: matrixArchitecture(endpoint.Arch), ContractRevision: "v1", Environment: "vm",
	}
	if base.Distribution == "" || base.Release == "" || base.Architecture == "" {
		return nil
	}
	var capabilities []Capability
	for _, provider := range endpoint.UniversalPackage {
		if provider != types.Flatpak {
			continue
		}
		claim := base
		claim.CapabilityID, claim.Provider, claim.Backend = "flatpak", "flatpak", "flatpak"
		if providermatrix.AdvertisedForPublication(*g.providerMatrix, claim) {
			capabilities = append(capabilities, Capability{ID: "provider:package/flatpak", Revision: "1"})
		}
	}
	for _, browser := range endpoint.Browser {
		if browser != facts.BrowserChromium && browser != facts.BrowserGoogleChrome {
			continue
		}
		claim := base
		claim.CapabilityID, claim.Provider, claim.Backend = "pwa", "pwa", string(browser)
		if providermatrix.AdvertisedForPublication(*g.providerMatrix, claim) {
			capabilities = append(capabilities, Capability{ID: "provider:package/pwa", Revision: "1"})
		}
	}
	return capabilities
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
	for _, provider := range endpoint.UniversalPackage {
		values = append(values, Fact{Key: "package-universal", Value: lower(provider)})
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

func deduplicateCapabilities(input []Capability) []Capability {
	if len(input) < 2 {
		return input
	}
	output := input[:1]
	for _, capability := range input[1:] {
		if capability.ID == output[len(output)-1].ID && capability.Revision == output[len(output)-1].Revision {
			continue
		}
		output = append(output, capability)
	}
	return output
}
