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

// Row is one exact capability/backend/revision/platform/environment contract.
type Row struct {
	CapabilityID     string   `yaml:"capability_id"`
	Backend          string   `yaml:"backend"`
	ContractRevision string   `yaml:"contract_revision"`
	Distribution     string   `yaml:"distribution"`
	Release          string   `yaml:"release"`
	Architecture     string   `yaml:"architecture"`
	Environment      string   `yaml:"environment"`
	Risk             string   `yaml:"risk"`
	AcceptedFields   []string `yaml:"accepted_fields"`
	ComposedAddress  *string  `yaml:"composed_address"`
	GoverningIDs     []string `yaml:"governing_ids"`
	Selectors        []string `yaml:"selectors"`
	Disposition      string   `yaml:"disposition"`
	Reason           string   `yaml:"reason"`
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
	clone.Rows = make([]Row, len(m.Rows))
	for index, row := range m.Rows {
		clone.Rows[index] = row
		clone.Rows[index].AcceptedFields = slices.Clone(row.AcceptedFields)
		clone.Rows[index].GoverningIDs = slices.Clone(row.GoverningIDs)
		clone.Rows[index].Selectors = slices.Clone(row.Selectors)
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
		if strings.TrimSpace(row.Risk) == "" || len(row.GoverningIDs) == 0 || len(row.Selectors) == 0 || strings.TrimSpace(row.Reason) == "" {
			return fmt.Errorf("%s: risk, governing IDs, selectors, and reason are required", location)
		}
		if row.Disposition != "unadvertised" && (row.ComposedAddress == nil || strings.TrimSpace(*row.ComposedAddress) == "") {
			return fmt.Errorf("%s: composed address is required for %s", location, row.Disposition)
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
