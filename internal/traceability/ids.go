// Package traceability defines stable OpenSpec verification identifiers.
package traceability

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	prefixPattern = regexp.MustCompile(`^OS-[A-Z][A-Z0-9]{1,15}$`)
	idPattern     = regexp.MustCompile(`^(OS-[A-Z][A-Z0-9]{1,15})-([0-9]{3,})$`)
)

// PrefixRegistry maps an immutable verification-ID prefix to exactly one
// OpenSpec change and capability. Prefixes are never reused for another
// capability, even if the original capability is retired.
type PrefixRegistry struct {
	Version  int                        `yaml:"version"`
	Prefixes map[string]PrefixOwnership `yaml:"prefixes"`
}

// PrefixOwnership identifies the canonical OpenSpec source for a prefix.
type PrefixOwnership struct {
	Change     string `yaml:"change"`
	Capability string `yaml:"capability"`
}

// VerificationID is the parsed form of an OpenSpec verification ID.
type VerificationID struct {
	Value    string
	Prefix   string
	Sequence int
}

// LoadPrefixRegistry loads and validates the versioned central registry.
func LoadPrefixRegistry(path string) (PrefixRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PrefixRegistry{}, err
	}
	var registry PrefixRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return PrefixRegistry{}, fmt.Errorf("decode prefix registry: %w", err)
	}
	if registry.Version != 1 {
		return PrefixRegistry{}, fmt.Errorf("unsupported prefix registry version %d", registry.Version)
	}
	if len(registry.Prefixes) == 0 {
		return PrefixRegistry{}, fmt.Errorf("prefix registry is empty")
	}
	for prefix, ownership := range registry.Prefixes {
		if !prefixPattern.MatchString(prefix) {
			return PrefixRegistry{}, fmt.Errorf("invalid prefix %q", prefix)
		}
		if ownership.Change == "" || ownership.Capability == "" {
			return PrefixRegistry{}, fmt.Errorf("prefix %q must declare change and capability", prefix)
		}
	}
	return registry, nil
}

// ParseVerificationID validates the immutable comment value and ensures its
// prefix is centrally registered. The comment syntax is:
//
//	<!-- verification-id: OS-AEC-001 -->
func ParseVerificationID(value string, registry PrefixRegistry) (VerificationID, error) {
	matches := idPattern.FindStringSubmatch(value)
	if matches == nil {
		return VerificationID{}, fmt.Errorf("invalid verification ID %q", value)
	}
	if _, ok := registry.Prefixes[matches[1]]; !ok {
		return VerificationID{}, fmt.Errorf("unregistered verification ID prefix %q", matches[1])
	}
	sequence, err := strconv.Atoi(matches[2])
	if err != nil || sequence < 1 {
		return VerificationID{}, fmt.Errorf("invalid verification ID sequence %q", matches[2])
	}
	return VerificationID{Value: value, Prefix: matches[1], Sequence: sequence}, nil
}

// VerificationIDFromComment extracts a validation-ready identifier from one
// exact OpenSpec comment. It intentionally rejects surrounding prose so a
// malformed comment cannot silently become an identifier.
func VerificationIDFromComment(comment string, registry PrefixRegistry) (VerificationID, error) {
	const prefix = "<!-- verification-id: "
	const suffix = " -->"
	if !strings.HasPrefix(comment, prefix) || !strings.HasSuffix(comment, suffix) {
		return VerificationID{}, fmt.Errorf("invalid verification-id comment %q", comment)
	}
	return ParseVerificationID(strings.TrimSuffix(strings.TrimPrefix(comment, prefix), suffix), registry)
}
