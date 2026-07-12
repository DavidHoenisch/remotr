package traceability

import "fmt"

// EvidenceRunner runs one evidence selector and returns its failure, if any.
type EvidenceRunner func(selector string) error

// AdvertisementIssues returns the governing evidence failures that prevent a
// capability from being advertised. Planned and deferred entries never count
// as evidence; each verified entry must have passing selectors and environments.
func AdvertisementIssues(manifest Manifest, change, capability string, run EvidenceRunner) []Issue {
	issues := make([]Issue, 0)
	found := false
	for id, entry := range manifest.Scenarios {
		if entry.Source.Change != change || entry.Source.Capability != capability {
			continue
		}
		found = true
		location := "manifest:" + id
		if entry.Lifecycle != "verified" {
			issues = append(issues, Issue{location, "cannot advertise with lifecycle " + entry.Lifecycle})
			continue
		}
		if len(entry.Selectors) == 0 || len(entry.Environments) == 0 {
			issues = append(issues, Issue{location, "verified evidence requires selectors and environments"})
			continue
		}
		for _, selector := range entry.Selectors {
			if !validSelector(selector) {
				issues = append(issues, Issue{location, "invalid selector " + selector})
				continue
			}
			if err := run(selector); err != nil {
				issues = append(issues, Issue{location, fmt.Sprintf("failing evidence %s: %v", selector, err)})
			}
		}
	}
	if !found {
		issues = append(issues, Issue{"manifest", "no traceability entries for " + change + "/" + capability})
	}
	return issues
}
