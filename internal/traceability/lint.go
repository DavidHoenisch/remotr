package traceability

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	classes        = map[string]bool{"unit": true, "contract": true, "acceptance": true, "container": true, "vm": true, "fuzz": true, "mutation": true, "performance": true, "manual": true}
	lifecycles     = map[string]bool{"planned": true, "verified": true, "deferred": true, "not-applicable": true, "removed": true}
	goTestSelector = regexp.MustCompile(`^go-test:(\./[A-Za-z0-9_./-]+):\^.+\$$`)
)

// Issue is a source-location-aware traceability lint failure.
type Issue struct {
	Location string
	Message  string
}

func (issue Issue) String() string { return issue.Location + ": " + issue.Message }

// Lint loads canonical scenarios and validates their traceability manifest.
func Lint(openspecRoot, registryPath, manifestPath string) ([]Issue, error) {
	registry, err := LoadPrefixRegistry(registryPath)
	if err != nil {
		return nil, err
	}
	inventory, err := Inventory(openspecRoot)
	if err != nil {
		return nil, err
	}
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	return Validate(inventory, registry, manifest), nil
}

// Validate lints parsed inputs, making all error cases unit-testable.
func Validate(inventory []Scenario, registry PrefixRegistry, manifest Manifest) []Issue {
	issues := make([]Issue, 0)
	if manifest.Version != 1 {
		issues = append(issues, Issue{"manifest", fmt.Sprintf("unsupported version %d", manifest.Version)})
	}
	seen := map[string]Scenario{}
	canonical := map[string]Scenario{}
	modifiers := map[string]Scenario{}
	for _, scenario := range inventory {
		location := fmt.Sprintf("%s:%d", scenario.Path, scenario.ScenarioLine)
		if scenario.VerificationID == "" {
			issues = append(issues, Issue{location, "missing verification ID"})
			continue
		}
		id, err := ParseVerificationID(scenario.VerificationID, registry)
		if err != nil {
			issues = append(issues, Issue{location, err.Error()})
			continue
		}
		owner := registry.Prefixes[id.Prefix]
		isCanonical := owner.Change == scenario.Change && owner.Capability == scenario.Capability
		isModifier := owner.Capability == scenario.Capability && modifierAllowed(owner, scenario.Change) && (scenario.Operation == "added" || scenario.Operation == "modified")
		if !isCanonical && !isModifier {
			issues = append(issues, Issue{location, fmt.Sprintf("verification ID %s belongs to %s/%s", id.Value, owner.Change, owner.Capability)})
		}
		if isCanonical {
			if prior, duplicate := canonical[id.Value]; duplicate {
				issues = append(issues, Issue{location, fmt.Sprintf("duplicate/reused verification ID %s (first at %s:%d)", id.Value, prior.Path, prior.ScenarioLine)})
			} else {
				canonical[id.Value] = scenario
			}
		}
		if isModifier {
			if prior, competing := modifiers[id.Value]; competing {
				issues = append(issues, Issue{location, fmt.Sprintf("competing modifier for verification ID %s (first at %s:%d)", id.Value, prior.Path, prior.ScenarioLine)})
			} else {
				modifiers[id.Value] = scenario
			}
		}
		if _, exists := seen[id.Value]; !exists {
			seen[id.Value] = scenario
		}
	}

	for id, scenario := range seen {
		entry, exists := manifest.Scenarios[id]
		if !exists {
			issues = append(issues, Issue{scenario.Path, "missing manifest entry for " + id})
			continue
		}
		parsed, err := ParseVerificationID(id, registry)
		if err != nil {
			continue
		}
		owner := registry.Prefixes[parsed.Prefix]
		if entry.Source.Change != owner.Change || entry.Source.Capability != owner.Capability {
			issues = append(issues, Issue{"manifest:" + id, "source does not match canonical scenario"})
		}
		issues = append(issues, validateEntry(id, entry, manifest.Environments)...)
	}
	for id := range manifest.Scenarios {
		if _, exists := seen[id]; !exists {
			issues = append(issues, Issue{"manifest:" + id, "orphan manifest entry"})
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].String() < issues[j].String() })
	return issues
}

func modifierAllowed(owner PrefixOwnership, change string) bool {
	for _, modifier := range owner.Modifiers {
		if modifier == change {
			return true
		}
	}
	return false
}

func validateEntry(id string, entry ManifestEntry, environments map[string]string) []Issue {
	issues := make([]Issue, 0)
	location := "manifest:" + id
	if !lifecycles[entry.Lifecycle] {
		issues = append(issues, Issue{location, "invalid lifecycle " + entry.Lifecycle})
	}
	if len(entry.VerificationClasses) == 0 {
		issues = append(issues, Issue{location, "missing verification classes"})
	}
	for _, class := range entry.VerificationClasses {
		if !classes[class] {
			issues = append(issues, Issue{location, "invalid verification class " + class})
		}
	}
	for _, selector := range entry.Selectors {
		if !validSelector(selector) {
			issues = append(issues, Issue{location, "invalid selector " + selector})
		}
	}
	for _, environment := range entry.Environments {
		if _, exists := environments[environment]; !exists {
			issues = append(issues, Issue{location, "unknown environment " + environment})
		}
	}
	if entry.Lifecycle == "verified" && (len(entry.Selectors) == 0 || len(entry.Environments) == 0) {
		issues = append(issues, Issue{location, "verified entry requires selectors and environments"})
	}
	if entry.Lifecycle != "verified" && strings.TrimSpace(entry.DispositionReason) == "" {
		issues = append(issues, Issue{location, "non-verified entry requires disposition reason"})
	}
	return issues
}

func validSelector(selector string) bool {
	return goTestSelector.MatchString(selector) || strings.HasPrefix(selector, "godog:@os_") || strings.HasPrefix(selector, "make:")
}
