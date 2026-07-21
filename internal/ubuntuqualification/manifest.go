// Package ubuntuqualification validates the exact Ubuntu 24.04 applicator
// qualification inventory used by release and capability-advertisement gates.
package ubuntuqualification

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
	"gopkg.in/yaml.v3"
)

const maxManifestBytes = 4 << 20

// Manifest is the versioned Ubuntu qualification inventory.
type Manifest struct {
	Version                int                     `yaml:"version"`
	Change                 string                  `yaml:"change"`
	Platform               Platform                `yaml:"platform"`
	ExternalQualifications []ExternalQualification `yaml:"external_qualifications"`
	FutureRoadmap          []FutureRoadmapEntry    `yaml:"future_roadmap"`
	Rows                   []Row                   `yaml:"rows"`
}

// Platform fixes the only distribution tuple governed by this change.
type Platform struct {
	Distribution string `yaml:"distribution"`
	Release      string `yaml:"release"`
	Architecture string `yaml:"architecture"`
}

// ExternalQualification delegates contracts owned by another OpenSpec change.
type ExternalQualification struct {
	CapabilityIDs []string `yaml:"capability_ids"`
	Change        string   `yaml:"change"`
	Reason        string   `yaml:"reason"`
}

// FutureRoadmapEntry records an intentionally absent capability without
// allowing it into the exact registered-contract row namespace.
type FutureRoadmapEntry struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Disposition string `yaml:"disposition"`
	Reason      string `yaml:"reason"`
}

// Row is one exact capability/backend/revision/platform/environment contract.
type Row struct {
	CapabilityID     string    `yaml:"capability_id"`
	Backend          string    `yaml:"backend"`
	ContractRevision string    `yaml:"contract_revision"`
	Distribution     string    `yaml:"distribution"`
	Release          string    `yaml:"release"`
	Architecture     string    `yaml:"architecture"`
	Environment      string    `yaml:"environment"`
	Risk             string    `yaml:"risk"`
	AcceptedFields   []string  `yaml:"accepted_fields"`
	ComposedAddress  *string   `yaml:"composed_address"`
	GoverningIDs     []string  `yaml:"governing_ids"`
	Selectors        []string  `yaml:"selectors"`
	Disposition      string    `yaml:"disposition"`
	Reason           string    `yaml:"reason"`
	TDD              TDDRecord `yaml:"tdd"`
}

// TDDRecord is the per-row red-green evidence state machine. Planned rows do
// not authorize production changes; a committed red observation precedes a
// correction and verified rows retain the green and broader evidence.
type TDDRecord struct {
	GoverningID      string   `yaml:"governing_id"`
	PublicSeam       string   `yaml:"public_seam"`
	ExpectedResult   string   `yaml:"expected_result"`
	EvidenceLayers   []string `yaml:"evidence_layers"`
	Phase            string   `yaml:"phase"`
	RedFailure       *string  `yaml:"red_failure"`
	GreenResult      *string  `yaml:"green_result"`
	BroaderChecks    []string `yaml:"broader_checks"`
	FinalDisposition string   `yaml:"final_disposition"`
}

// Load reads and validates a qualification manifest against the live registry.
func Load(path string, registry *resourceregistry.Registry) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return Decode(data, registry)
}

// Decode strictly parses a bounded qualification manifest and validates it.
func Decode(data []byte, registry *resourceregistry.Registry) (Manifest, error) {
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("qualification manifest exceeds %d bytes", maxManifestBytes)
	}
	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode qualification manifest: %w", err)
	}
	if err := Validate(manifest, registry); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Clone returns a deep copy suitable for independent validation fixtures.
func (m Manifest) Clone() Manifest {
	clone := m
	clone.ExternalQualifications = make([]ExternalQualification, len(m.ExternalQualifications))
	for index, external := range m.ExternalQualifications {
		clone.ExternalQualifications[index] = external
		clone.ExternalQualifications[index].CapabilityIDs = slices.Clone(external.CapabilityIDs)
	}
	clone.FutureRoadmap = slices.Clone(m.FutureRoadmap)
	clone.Rows = make([]Row, len(m.Rows))
	for index, row := range m.Rows {
		clone.Rows[index] = row
		clone.Rows[index].AcceptedFields = slices.Clone(row.AcceptedFields)
		clone.Rows[index].GoverningIDs = slices.Clone(row.GoverningIDs)
		clone.Rows[index].Selectors = slices.Clone(row.Selectors)
		clone.Rows[index].TDD.EvidenceLayers = slices.Clone(row.TDD.EvidenceLayers)
		clone.Rows[index].TDD.BroaderChecks = slices.Clone(row.TDD.BroaderChecks)
		if row.TDD.RedFailure != nil {
			red := *row.TDD.RedFailure
			clone.Rows[index].TDD.RedFailure = &red
		}
		if row.TDD.GreenResult != nil {
			green := *row.TDD.GreenResult
			clone.Rows[index].TDD.GreenResult = &green
		}
		if row.ComposedAddress != nil {
			address := *row.ComposedAddress
			clone.Rows[index].ComposedAddress = &address
		}
	}
	return clone
}

type backendPolicy struct {
	backend     string
	environment string
}

var ubuntuPolicies = map[models.ResourceKind][]backendPolicy{
	models.ResourceKindFile:             {{"posix", "container"}},
	models.ResourceKindDownload:         {{"https", "container"}},
	models.ResourceKindDirectory:        {{"posix", "container"}},
	models.ResourceKindLink:             {{"posix", "container"}},
	models.ResourceKindGroup:            {{"shadow", "vm-access"}},
	models.ResourceKindUser:             {{"shadow", "vm-access"}},
	models.ResourceKindAuthorizedKey:    {{"openssh", "vm-access"}},
	models.ResourceKindKnownHost:        {{"openssh", "container"}},
	models.ResourceKindSudo:             {{"sudoers", "vm-access"}},
	models.ResourceKindUserFile:         {{"posix", "vm-access"}},
	models.ResourceKindSysctl:           {{"procfs", "vm-system"}},
	models.ResourceKindKernelModule:     {{"kmod", "vm-system"}},
	models.ResourceKindHostname:         {{"systemd-hostnamed", "vm-system"}},
	models.ResourceKindHostLocale:       {{"systemd-localed", "vm-system"}},
	models.ResourceKindTimeSync:         {{"systemd-timesyncd", "vm-system"}},
	models.ResourceKindMount:            {{"util-linux", "vm-system"}},
	models.ResourceKindSwap:             {{"util-linux", "vm-system"}},
	models.ResourceKindEndpointSchedule: {{"cron", "container"}, {"systemd-timer", "vm-system"}},
	models.ResourceKindService:          {{"systemd", "vm-system"}, {"openrc", "vm-system"}, {"sysv", "vm-system"}},
	models.ResourceKindSystemd:          {{"systemd-legacy", "vm-system"}},
	models.ResourceKindSystemdUser:      {{"systemd-user-legacy", "vm-desktop"}},
	models.ResourceKindSystemdUnit:      {{"systemd", "vm-system"}},
	models.ResourceKindReboot:           {{"systemd", "vm-system"}},
	models.ResourceKindHostsEntry:       {{"hosts-file", "vm-network"}},
	models.ResourceKindDNSResolver:      {{"network-manager", "vm-network"}},
	models.ResourceKindRoute:            {{"network-manager", "vm-network"}},
	models.ResourceKindNetworkProfile:   {{"network-manager", "vm-network"}, {"netplan", "vm-network"}, {"systemd-networkd", "vm-network"}},
	models.ResourceKindFirewall:         {{"nftables-audit", "vm-network"}, {"nftables-enforcement", "vm-network"}, {"firewalld-audit", "vm-network"}, {"firewalld-enforcement", "vm-network"}},
	models.ResourceKindCertificate:      {{"pem-files", "vm-system"}},
	models.ResourceKindTrustAnchor:      {{"update-ca-certificates", "vm-system"}},
	models.ResourceKindAppArmorProfile:  {{"apparmor-parser", "vm-system"}},
	models.ResourceKindAuditRules:       {{"auditd", "vm-system"}},
	models.ResourceKindAccountLimit:     {{"pam-limits", "vm-access"}},
	models.ResourceKindLoginPolicy:      {{"pam-auth-update", "vm-access"}},
	models.ResourceKindJournald:         {{"systemd-journald", "vm-system"}},
	models.ResourceKindLogrotate:        {{"logrotate", "vm-system"}},
	models.ResourceKindDesktopSetting:   {{"dconf", "vm-desktop"}, {"gsettings", "vm-desktop"}},
	models.ResourceKindSessionPolicy:    {{"dconf", "vm-desktop"}, {"gsettings", "vm-desktop"}},
	models.ResourceKindBrowserPolicy:    {{"chromium", "vm-desktop"}, {"google-chrome", "vm-desktop"}, {"firefox", "vm-desktop"}},
	models.ResourceKindBootstrap:        {{"one-shot", "vm-system"}},
	models.ResourceKindAgentInstall:     {{"binary-install", "vm-system"}},
	models.ResourceKindCommand:          {{"argv", "vm-system"}},
}

var delegatedCapabilities = map[models.ResourceKind]bool{
	models.ResourceKindPackage:       true,
	models.ResourceKindAPTSigningKey: true,
	models.ResourceKindAPTRepository: true,
}

var outOfScopeCapabilities = map[models.ResourceKind]bool{
	models.ResourceKindPacmanSigningKey: true,
	models.ResourceKindPacmanRepository: true,
}

var broadFamilies = map[string]bool{
	"desktop":    true,
	"filesystem": true,
	"identity":   true,
	"network":    true,
	"repository": true,
	"security":   true,
}

var requiredUnadvertised = map[string]bool{
	"agentInstall/binary-install":     true,
	"bootstrap/one-shot":              true,
	"command/argv":                    true,
	"firewall/firewalld-enforcement":  true,
	"networkProfile/netplan":          true,
	"networkProfile/systemd-networkd": true,
	"service/openrc":                  true,
	"service/sysv":                    true,
	"systemd/systemd-legacy":          true,
	"systemdUser/systemd-user-legacy": true,
}

var requiredSecretCanaryRows = map[string]bool{
	"certificate/pem-files":              true,
	"trustAnchor/update-ca-certificates": true,
	"appArmorProfile/apparmor-parser":    true,
	"auditRules/auditd":                  true,
	"accountLimit/pam-limits":            true,
	"loginPolicy/pam-auth-update":        true,
	"journald/systemd-journald":          true,
	"logrotate/logrotate":                true,
}

var requiredSecretCanarySelectors = []string{
	"go-test:./internal/resourceregistry:^TestResourceSafeSummaryProjectsClassifiedFieldsAndOmitsSecretCanary$",
	"go-test:./cmd/remotr:^TestSecretUploadReadsProtectedInputFileAndNeverAcceptsArgvMaterial$",
	"make:test-e2e",
}

var requiredFutureRoadmap = []string{
	"UHF-000", "UHF-001", "UHF-002",
	"UHF-100", "UHF-101", "UHF-102", "UHF-103", "UHF-104", "UHF-105", "UHF-106", "UHF-107",
	"UHF-200", "UHF-201", "UHF-202", "UHF-203", "UHF-204", "UHF-205", "UHF-206", "UHF-207", "UHF-208",
	"UHF-300", "UHF-301", "UHF-302", "UHF-303", "UHF-304",
}

// Validate rejects incomplete, duplicate, broad-only, stale, and unknown rows.
func Validate(manifest Manifest, registry *resourceregistry.Registry) error {
	if registry == nil {
		return fmt.Errorf("resource registry is required")
	}
	if manifest.Version != 1 {
		return fmt.Errorf("qualification manifest version = %d, want 1", manifest.Version)
	}
	if manifest.Change != "qualify-ubuntu-2404-applicators" {
		return fmt.Errorf("qualification manifest change = %q", manifest.Change)
	}
	if manifest.Platform != (Platform{Distribution: "ubuntu", Release: "24.04", Architecture: "amd64"}) {
		return fmt.Errorf("qualification platform = %+v, want ubuntu/24.04/amd64", manifest.Platform)
	}
	if err := validateExternalQualifications(manifest.ExternalQualifications); err != nil {
		return err
	}
	if err := validateFutureRoadmap(manifest.FutureRoadmap); err != nil {
		return err
	}

	definitions := make(map[models.ResourceKind]resourceregistry.Definition)
	expected := make(map[string]resourceregistry.Definition)
	for _, definition := range registry.Definitions() {
		definitions[definition.Kind] = definition
		if delegatedCapabilities[definition.Kind] || outOfScopeCapabilities[definition.Kind] {
			continue
		}
		policies, exists := ubuntuPolicies[definition.Kind]
		if !exists || len(policies) == 0 {
			return fmt.Errorf("registered contract %q has no Ubuntu qualification policy", definition.Kind)
		}
		for _, policy := range policies {
			key := exactKey(string(definition.Kind), policy.backend, definition.ProviderContractRevision, policy.environment)
			expected[key] = definition
		}
	}
	for kind := range ubuntuPolicies {
		if _, exists := definitions[kind]; !exists {
			return fmt.Errorf("Ubuntu qualification policy refers to unknown registered contract %q", kind)
		}
	}

	seen := make(map[string]struct{}, len(manifest.Rows))
	for index, row := range manifest.Rows {
		location := fmt.Sprintf("qualification row %d", index+1)
		if broadFamilies[row.CapabilityID] || row.Backend == "*" {
			return fmt.Errorf("%s: broad family capability %q cannot qualify an exact contract", location, row.CapabilityID)
		}
		definition, exists := definitions[models.ResourceKind(row.CapabilityID)]
		if !exists || delegatedCapabilities[definition.Kind] || outOfScopeCapabilities[definition.Kind] {
			return fmt.Errorf("%s: unknown qualification row %s/%s", location, row.CapabilityID, row.Backend)
		}
		if row.ContractRevision != definition.ProviderContractRevision {
			return fmt.Errorf("%s: stale contract revision %q for %s, want %q", location, row.ContractRevision, row.CapabilityID, definition.ProviderContractRevision)
		}
		if row.Distribution != "ubuntu" || row.Release != "24.04" || row.Architecture != "amd64" {
			return fmt.Errorf("%s: row platform must be ubuntu/24.04/amd64", location)
		}
		key := exactKey(row.CapabilityID, row.Backend, row.ContractRevision, row.Environment)
		if _, exists := expected[key]; !exists {
			return fmt.Errorf("%s: unknown qualification row %s", location, key)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s: duplicate exact qualification row %s", location, key)
		}
		seen[key] = struct{}{}
		wantFields := acceptedFields(definition)
		if !slices.Equal(row.AcceptedFields, wantFields) {
			return fmt.Errorf("%s: accepted fields for %s are stale", location, key)
		}
		if row.Disposition != "qualified" && row.Disposition != "blocked" && row.Disposition != "unadvertised" {
			return fmt.Errorf("%s: invalid disposition %q", location, row.Disposition)
		}
		nonQualificationKey := row.CapabilityID + "/" + row.Backend
		if requiredUnadvertised[nonQualificationKey] {
			if row.Disposition != "unadvertised" {
				return fmt.Errorf("%s: %s must remain unadvertised", location, nonQualificationKey)
			}
			if strings.TrimSpace(row.Reason) == "" {
				return fmt.Errorf("%s: %s requires an explicit non-qualification reason", location, nonQualificationKey)
			}
		}
		if strings.TrimSpace(row.Risk) == "" || len(row.GoverningIDs) == 0 || len(row.Selectors) == 0 || strings.TrimSpace(row.Reason) == "" {
			return fmt.Errorf("%s: risk, governing IDs, selectors, and reason are required", location)
		}
		if err := validateSecretCanaryQualification(row); err != nil {
			return fmt.Errorf("%s: %w", location, err)
		}
		if row.Disposition != "unadvertised" && (row.ComposedAddress == nil || strings.TrimSpace(*row.ComposedAddress) == "") {
			return fmt.Errorf("%s: composed address is required for %s", location, row.Disposition)
		}
		if err := validateTDDRecord(row); err != nil {
			return fmt.Errorf("%s: %w", location, err)
		}
	}

	missing := make([]string, 0)
	for key := range expected {
		if _, exists := seen[key]; !exists {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing exact qualification row %s", missing[0])
	}
	return nil
}

func validateSecretCanaryQualification(row Row) error {
	if row.Disposition != "qualified" || !requiredSecretCanaryRows[row.CapabilityID+"/"+row.Backend] {
		return nil
	}
	if !slices.Contains(row.GoverningIDs, "OS-AEC-099") || !slices.Contains(row.TDD.EvidenceLayers, "secret-canary") {
		return fmt.Errorf("OS-AEC-099 secret-canary evidence is required for qualified %s/%s", row.CapabilityID, row.Backend)
	}
	for _, selector := range requiredSecretCanarySelectors {
		if !slices.Contains(row.Selectors, selector) {
			return fmt.Errorf("OS-AEC-099 secret-canary evidence for qualified %s/%s requires selector %q", row.CapabilityID, row.Backend, selector)
		}
	}
	return nil
}

var approvedPublicSeams = map[string]bool{
	"authenticated-sync":       true,
	"composed-agent-execution": true,
	"configuration-cli":        true,
	"observable-performance":   true,
	"operator-cli-admin-api":   true,
	"provider-contract":        true,
	"system-safety-recovery":   true,
}

func validateTDDRecord(row Row) error {
	record := row.TDD
	if !slices.Contains(row.GoverningIDs, record.GoverningID) {
		return fmt.Errorf("TDD governing ID %q is not assigned to the row", record.GoverningID)
	}
	if !approvedPublicSeams[record.PublicSeam] {
		return fmt.Errorf("TDD public seam %q is not approved", record.PublicSeam)
	}
	if strings.TrimSpace(record.ExpectedResult) == "" || len(record.EvidenceLayers) == 0 {
		return fmt.Errorf("TDD expected result and evidence layers are required")
	}
	if record.FinalDisposition != row.Disposition {
		return fmt.Errorf("TDD final disposition %q does not match row disposition %q", record.FinalDisposition, row.Disposition)
	}
	if row.Disposition == "qualified" && record.Phase != "verified" {
		return fmt.Errorf("qualified row requires verified TDD evidence")
	}
	redPresent := record.RedFailure != nil && strings.TrimSpace(*record.RedFailure) != ""
	greenPresent := record.GreenResult != nil && strings.TrimSpace(*record.GreenResult) != ""
	switch record.Phase {
	case "planned":
		if redPresent || greenPresent || len(record.BroaderChecks) != 0 || row.Disposition == "qualified" {
			return fmt.Errorf("planned TDD phase cannot contain results or qualify a row")
		}
	case "red-observed":
		if !redPresent {
			return fmt.Errorf("red-observed TDD phase requires an observed red failure")
		}
		if greenPresent || len(record.BroaderChecks) != 0 || row.Disposition == "qualified" {
			return fmt.Errorf("red-observed TDD phase cannot contain green or broader evidence")
		}
	case "green":
		if !redPresent || !greenPresent {
			return fmt.Errorf("green TDD phase requires red and green results")
		}
		if row.Disposition == "qualified" {
			return fmt.Errorf("green TDD phase cannot qualify a row before broader checks")
		}
	case "verified":
		if !redPresent || !greenPresent || len(record.BroaderChecks) == 0 {
			return fmt.Errorf("verified TDD phase requires broader checks after red and green results")
		}
	case "not-applicable":
		if row.Disposition != "unadvertised" || redPresent || greenPresent || len(record.BroaderChecks) != 0 {
			return fmt.Errorf("not-applicable TDD phase is only valid for untouched unadvertised rows")
		}
	default:
		return fmt.Errorf("unknown TDD phase %q", record.Phase)
	}
	return nil
}

func validateFutureRoadmap(entries []FutureRoadmapEntry) error {
	expected := make(map[string]struct{}, len(requiredFutureRoadmap))
	for _, id := range requiredFutureRoadmap {
		expected[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if _, exists := expected[entry.ID]; !exists {
			return fmt.Errorf("future-roadmap item %q is unknown", entry.ID)
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return fmt.Errorf("duplicate future-roadmap item %s", entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if entry.Disposition != "future-roadmap" {
			return fmt.Errorf("future-roadmap item %s has disposition %q", entry.ID, entry.Disposition)
		}
		if strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.Reason) == "" {
			return fmt.Errorf("future-roadmap item %s requires a title and reason (entry %d)", entry.ID, index+1)
		}
	}
	for _, id := range requiredFutureRoadmap {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("missing future-roadmap item %s", id)
		}
	}
	return nil
}

func validateExternalQualifications(external []ExternalQualification) error {
	if len(external) != 1 {
		return fmt.Errorf("exactly one external package qualification reference is required")
	}
	entry := external[0]
	want := []string{"aptRepository", "aptSigningKey", "package"}
	got := slices.Clone(entry.CapabilityIDs)
	sort.Strings(got)
	if !slices.Equal(got, want) || entry.Change != "complete-core-package-providers" || strings.TrimSpace(entry.Reason) == "" {
		return fmt.Errorf("APT package contracts must reference complete-core-package-providers")
	}
	return nil
}

func acceptedFields(definition resourceregistry.Definition) []string {
	fields := make([]string, 0, len(definition.FieldDescriptors))
	for field := range definition.FieldDescriptors {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func exactKey(capability, backend, revision, environment string) string {
	return strings.Join([]string{capability, backend, revision, "ubuntu", "24.04", "amd64", environment}, "/")
}
