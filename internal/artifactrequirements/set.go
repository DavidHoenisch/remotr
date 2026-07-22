// Package artifactrequirements models the bounded contracts required to
// process one canonical artifact variant.
package artifactrequirements

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const (
	CurrentVersion  = 1
	MaxRequirements = 256
	MaxTargetValues = 16
)

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._:/-][a-z0-9]+)*$`)
	revisionPattern   = regexp.MustCompile(`^(?:0|[1-9][0-9]*(?:\.(?:0|[1-9][0-9]*)){0,2}|[A-Za-z][A-Za-z0-9]*(?:[._-][A-Za-z0-9]+)*)$`)
)

// Requirement is one exact resource or provider contract needed by a
// compiled artifact.
type Requirement struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

// TargetPredicate limits one requirement projection to normalized endpoint
// facts. Omitted dimensions are wildcards. It never changes artifact bytes.
type TargetPredicate struct {
	Distros       []string `json:"distros,omitempty"`
	Architectures []string `json:"architectures,omitempty"`
}

// Set is the versioned requirement evidence attached to one artifact variant.
// Schema support remains explicit rather than being represented as a pseudo
// capability so selection can compare it to the document's schema set.
type Set struct {
	Version               int              `json:"version"`
	ArtifactSchemaVersion int              `json:"artifactSchemaVersion"`
	Target                *TargetPredicate `json:"target,omitempty"`
	ResourceCapabilities  []Requirement    `json:"resourceCapabilities"`
	ProviderCapabilities  []Requirement    `json:"providerCapabilities"`
}

// Validate rejects unsupported versions, schemas, malformed contracts, and
// duplicate or conflicting requirement IDs.
func (s Set) Validate() error {
	if s.Version != CurrentVersion {
		return fmt.Errorf("unsupported requirement-set version %d", s.Version)
	}
	if s.ArtifactSchemaVersion != 0 && s.ArtifactSchemaVersion != 1 {
		return fmt.Errorf("unsupported artifact schema version %d", s.ArtifactSchemaVersion)
	}
	if len(s.ResourceCapabilities)+len(s.ProviderCapabilities) > MaxRequirements {
		return fmt.Errorf("requirement count exceeds %d", MaxRequirements)
	}
	if err := validateTarget(s.Target); err != nil {
		return err
	}
	seen := make(map[string]string, len(s.ResourceCapabilities)+len(s.ProviderCapabilities))
	if err := validateRequirements(s.ResourceCapabilities, "resource:", seen); err != nil {
		return err
	}
	if err := validateRequirements(s.ProviderCapabilities, "provider:", seen); err != nil {
		return err
	}
	return nil
}

// CanonicalBody returns compact deterministic JSON without mutating the
// caller-owned requirement slices.
func (s Set) CanonicalBody() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	canonical := Set{
		Version:               s.Version,
		ArtifactSchemaVersion: s.ArtifactSchemaVersion,
		Target:                cloneTarget(s.Target),
		ResourceCapabilities:  append([]Requirement(nil), s.ResourceCapabilities...),
		ProviderCapabilities:  append([]Requirement(nil), s.ProviderCapabilities...),
	}
	sortRequirements(canonical.ResourceCapabilities)
	sortRequirements(canonical.ProviderCapabilities)
	if canonical.Target != nil {
		sort.Strings(canonical.Target.Distros)
		sort.Strings(canonical.Target.Architectures)
	}
	return json.Marshal(canonical)
}

// CanonicalDigest returns the stable digest used in artifact variant keys.
func (s Set) CanonicalDigest() (string, error) {
	body, err := s.CanonicalBody()
	if err != nil {
		return "", fmt.Errorf("canonical artifact requirement set: %w", err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// DecodeCanonical restores strictly encoded persisted requirement evidence and
// verifies both its canonical bytes and separately indexed digest.
func DecodeCanonical(canonical []byte, digest string) (Set, error) {
	return decode(canonical, digest, true)
}

// DecodePersisted restores requirement evidence from a JSON store such as
// Postgres JSONB, which may normalize insignificant object-key ordering. It
// remains strict about fields, bounds, trailing input, and the separately
// indexed digest of the canonical semantic body.
func DecodePersisted(stored []byte, digest string) (Set, error) {
	return decode(stored, digest, false)
}

func decode(raw []byte, digest string, requireCanonicalBytes bool) (Set, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var set Set
	if err := decoder.Decode(&set); err != nil {
		return Set{}, fmt.Errorf("decode artifact requirement set: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Set{}, err
	}
	body, err := set.CanonicalBody()
	if err != nil {
		return Set{}, err
	}
	if requireCanonicalBytes && !bytes.Equal(body, raw) {
		return Set{}, fmt.Errorf("artifact requirement set is not canonical")
	}
	actual, err := set.CanonicalDigest()
	if err != nil {
		return Set{}, err
	}
	if actual != digest {
		return Set{}, fmt.Errorf("artifact requirement-set digest mismatch")
	}
	return set, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("artifact requirement set has trailing JSON")
		}
		return fmt.Errorf("decode artifact requirement set: %w", err)
	}
	return nil
}

func validateRequirements(requirements []Requirement, prefix string, seen map[string]string) error {
	for _, requirement := range requirements {
		if !strings.HasPrefix(requirement.ID, prefix) || !identifierPattern.MatchString(requirement.ID) {
			return fmt.Errorf("invalid %s requirement id %q", strings.TrimSuffix(prefix, ":"), requirement.ID)
		}
		if !revisionPattern.MatchString(requirement.Revision) {
			return fmt.Errorf("invalid requirement revision for %q", requirement.ID)
		}
		if revision, exists := seen[requirement.ID]; exists {
			if revision != requirement.Revision {
				return fmt.Errorf("conflicting requirement revisions for %q", requirement.ID)
			}
			return fmt.Errorf("duplicate requirement %q", requirement.ID)
		}
		seen[requirement.ID] = requirement.Revision
	}
	return nil
}

func sortRequirements(requirements []Requirement) {
	sort.Slice(requirements, func(i, j int) bool {
		if requirements[i].ID == requirements[j].ID {
			return requirements[i].Revision < requirements[j].Revision
		}
		return requirements[i].ID < requirements[j].ID
	})
}

func validateTarget(target *TargetPredicate) error {
	if target == nil {
		return nil
	}
	if len(target.Distros) == 0 && len(target.Architectures) == 0 {
		return fmt.Errorf("empty target predicate must be omitted")
	}
	if len(target.Distros) > MaxTargetValues || len(target.Architectures) > MaxTargetValues {
		return fmt.Errorf("target predicate value count exceeds %d", MaxTargetValues)
	}
	if err := validateTargetValues(target.Distros, map[string]bool{"ubuntu": true, "debian": true, "arch": true}, "distro"); err != nil {
		return err
	}
	return validateTargetValues(target.Architectures, map[string]bool{"x86": true, "arm": true}, "architecture")
}

func validateTargetValues(values []string, allowed map[string]bool, field string) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if !allowed[value] {
			return fmt.Errorf("invalid target %s", field)
		}
		if seen[value] {
			return fmt.Errorf("duplicate target %s", field)
		}
		seen[value] = true
	}
	return nil
}

func cloneTarget(target *TargetPredicate) *TargetPredicate {
	if target == nil {
		return nil
	}
	return &TargetPredicate{
		Distros:       append([]string(nil), target.Distros...),
		Architectures: append([]string(nil), target.Architectures...),
	}
}
