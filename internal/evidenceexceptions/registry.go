// Package evidenceexceptions validates the reviewed, expiring exceptions to
// Remotr's automated evidence policy.
package evidenceexceptions

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var exceptionIDPattern = regexp.MustCompile(`^EXC-[0-9]{3}$`)

var allowedKinds = map[string]bool{
	"manual":            true,
	"not-applicable":    true,
	"equivalent-mutant": true,
	"quarantine":        true,
}

// Registry is the versioned repository inventory of reviewed evidence
// exceptions. Review applies to the complete Records snapshot.
type Registry struct {
	Version int      `yaml:"version"`
	Review  Review   `yaml:"review"`
	Records []Record `yaml:"records"`
}

// Review identifies who reviewed this exact registry snapshot and why.
type Review struct {
	ReviewedBy string `yaml:"reviewed_by"`
	ReviewedAt string `yaml:"reviewed_at"`
	Scope      string `yaml:"scope"`
}

// Record describes one temporary exception from automated evidence.
type Record struct {
	ID                 string   `yaml:"id"`
	Kind               string   `yaml:"kind"`
	VerificationIDs    []string `yaml:"verification_ids,omitempty"`
	Owner              string   `yaml:"owner"`
	Issue              string   `yaml:"issue"`
	Reason             string   `yaml:"reason"`
	EquivalentSelector string   `yaml:"equivalent_selector,omitempty"`
	Expires            string   `yaml:"expires"`
}

// Load reads and validates a registry as of the caller-supplied instant.
func Load(path string, asOf time.Time) (Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, err
	}
	return Decode(data, asOf)
}

// Decode strictly decodes and validates a registry as of the caller-supplied
// instant. Injecting the time keeps expiry checks deterministic in tests.
func Decode(data []byte, asOf time.Time) (Registry, error) {
	var registry Registry
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("decode evidence exceptions: %w", err)
	}
	if err := Validate(registry, asOf); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

// Validate rejects unreviewed, expired, duplicate, or incomplete records.
func Validate(registry Registry, asOf time.Time) error {
	if registry.Version != 1 {
		return fmt.Errorf("evidence-exception registry version = %d, want 1", registry.Version)
	}
	if strings.TrimSpace(registry.Review.ReviewedBy) == "" || strings.TrimSpace(registry.Review.Scope) == "" {
		return fmt.Errorf("evidence-exception registry requires reviewed_by and scope")
	}
	reviewedAt, err := time.Parse("2006-01-02", registry.Review.ReviewedAt)
	if err != nil {
		return fmt.Errorf("evidence-exception registry reviewed_at %q: %w", registry.Review.ReviewedAt, err)
	}
	if reviewedAt.After(asOf) {
		return fmt.Errorf("evidence-exception registry review date %s is after validation date %s", reviewedAt.Format("2006-01-02"), asOf.Format("2006-01-02"))
	}
	if len(registry.Records) == 0 {
		return fmt.Errorf("evidence-exception registry requires at least one record")
	}

	seen := make(map[string]struct{}, len(registry.Records))
	for index, record := range registry.Records {
		if err := validateRecord(record, asOf); err != nil {
			return fmt.Errorf("evidence-exception record %d: %w", index+1, err)
		}
		if _, duplicate := seen[record.ID]; duplicate {
			return fmt.Errorf("evidence-exception record %d: duplicate id %s", index+1, record.ID)
		}
		seen[record.ID] = struct{}{}
	}
	return nil
}

func validateRecord(record Record, asOf time.Time) error {
	if !exceptionIDPattern.MatchString(record.ID) {
		return fmt.Errorf("id %q must match EXC-NNN", record.ID)
	}
	if !allowedKinds[record.Kind] {
		return fmt.Errorf("%s kind %q is not supported", record.ID, record.Kind)
	}
	if strings.TrimSpace(record.Owner) == "" || strings.TrimSpace(record.Issue) == "" || record.Issue == "pending-triage" || strings.TrimSpace(record.Reason) == "" {
		return fmt.Errorf("%s requires owner, reviewed issue, and reason", record.ID)
	}
	if (record.Kind == "equivalent-mutant" || record.Kind == "quarantine") && strings.TrimSpace(record.EquivalentSelector) == "" {
		return fmt.Errorf("%s kind %s requires equivalent_selector", record.ID, record.Kind)
	}
	expires, err := time.Parse("2006-01-02", record.Expires)
	if err != nil {
		return fmt.Errorf("%s expires %q: %w", record.ID, record.Expires, err)
	}
	if !expires.After(asOf) {
		return fmt.Errorf("%s expired on %s", record.ID, record.Expires)
	}
	return nil
}
