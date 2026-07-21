package ubuntuqualification

import (
	"fmt"
	"sort"
)

// AuditTarget is one exact qualification row and its observed release state.
type AuditTarget struct {
	Milestone          string `json:"milestone"`
	RowKey             string `json:"row_key"`
	Status             string `json:"status"`
	Selector           string `json:"selector"`
	ExplicitlyDescoped bool   `json:"explicitly_descoped,omitempty"`
}

// DependencyGate records whether an independently governed prerequisite has
// reached its accepted terminal state.
type DependencyGate struct {
	Name     string `json:"name"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// AuditBlocker preserves the exact evidence row that prevents completion.
type AuditBlocker struct {
	Milestone string `json:"milestone"`
	RowKey    string `json:"row_key"`
	Status    string `json:"status"`
	Selector  string `json:"selector"`
}

// MilestoneDecision is the qualification decision for one M1-M5 milestone.
type MilestoneDecision struct {
	Name     string         `json:"name"`
	Complete bool           `json:"complete"`
	Blockers []AuditBlocker `json:"blockers"`
}

// ArchiveDecision is the umbrella archive-eligibility decision.
type ArchiveDecision struct {
	Eligible           bool             `json:"eligible"`
	Blockers           []AuditBlocker   `json:"blockers"`
	DependencyBlockers []DependencyGate `json:"dependency_blockers"`
}

// AuditReport is the deterministic release-audit result.
type AuditReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Milestones    []MilestoneDecision `json:"milestones"`
	Dependencies  []DependencyGate    `json:"dependencies"`
	Umbrella      ArchiveDecision     `json:"umbrella"`
}

var auditStatuses = map[string]bool{
	"qualified":    true,
	"unadvertised": true,
	"blocked":      true,
	"planned":      true,
	"missing":      true,
	"skipped":      true,
	"failing":      true,
	"untested":     true,
}

var requiredAuditMilestones = []string{"M1", "M2", "M3", "M4", "M5"}

var requiredAuditDependencies = []string{
	"complete-applicator-execution-contract",
	"complete-capability-compatible-delivery",
	"complete-core-package-providers",
	"establish-testing-and-performance-foundation",
}

// GenerateAudit derives milestone and umbrella decisions from exact observed
// rows. A non-passing row is never summarized away: its identity, state, and
// evidence selector are copied into both decisions.
func GenerateAudit(targets []AuditTarget, dependencies []DependencyGate) (AuditReport, error) {
	byMilestone := make(map[string][]AuditBlocker)
	seenRows := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target.Milestone < "M1" || target.Milestone > "M5" {
			return AuditReport{}, fmt.Errorf("target %q has invalid milestone %q", target.RowKey, target.Milestone)
		}
		if target.RowKey == "" || target.Selector == "" {
			return AuditReport{}, fmt.Errorf("target in %s requires an exact row key and selector", target.Milestone)
		}
		if seenRows[target.RowKey] {
			return AuditReport{}, fmt.Errorf("duplicate audit row %q", target.RowKey)
		}
		seenRows[target.RowKey] = true
		if !auditStatuses[target.Status] {
			return AuditReport{}, fmt.Errorf("target %q has unknown status %q", target.RowKey, target.Status)
		}
		if target.ExplicitlyDescoped && target.Status != "unadvertised" {
			return AuditReport{}, fmt.Errorf("target %q may be explicitly descoped only when unadvertised", target.RowKey)
		}

		if target.Status == "qualified" || target.ExplicitlyDescoped {
			if _, ok := byMilestone[target.Milestone]; !ok {
				byMilestone[target.Milestone] = nil
			}
			continue
		}
		byMilestone[target.Milestone] = append(byMilestone[target.Milestone], AuditBlocker{
			Milestone: target.Milestone,
			RowKey:    target.RowKey,
			Status:    target.Status,
			Selector:  target.Selector,
		})
	}
	for _, milestone := range requiredAuditMilestones {
		if _, ok := byMilestone[milestone]; !ok {
			return AuditReport{}, fmt.Errorf("audit target inventory is missing milestone %s", milestone)
		}
	}

	report := AuditReport{SchemaVersion: 1}
	for _, name := range requiredAuditMilestones {
		blockers := byMilestone[name]
		sort.Slice(blockers, func(i, j int) bool { return blockers[i].RowKey < blockers[j].RowKey })
		report.Milestones = append(report.Milestones, MilestoneDecision{
			Name:     name,
			Complete: len(blockers) == 0,
			Blockers: blockers,
		})
		report.Umbrella.Blockers = append(report.Umbrella.Blockers, blockers...)
	}

	report.Dependencies = append([]DependencyGate(nil), dependencies...)
	sort.Slice(report.Dependencies, func(i, j int) bool { return report.Dependencies[i].Name < report.Dependencies[j].Name })
	seenDependencies := make(map[string]bool, len(report.Dependencies))
	for _, dependency := range report.Dependencies {
		if dependency.Name == "" {
			return AuditReport{}, fmt.Errorf("dependency gate requires a name")
		}
		if seenDependencies[dependency.Name] {
			return AuditReport{}, fmt.Errorf("duplicate dependency gate %q", dependency.Name)
		}
		seenDependencies[dependency.Name] = true
		if !dependency.Accepted {
			report.Umbrella.DependencyBlockers = append(report.Umbrella.DependencyBlockers, dependency)
		}
	}
	for _, dependency := range requiredAuditDependencies {
		if !seenDependencies[dependency] {
			return AuditReport{}, fmt.Errorf("audit dependency inventory is missing %q", dependency)
		}
	}
	if len(seenDependencies) != len(requiredAuditDependencies) {
		return AuditReport{}, fmt.Errorf("audit dependency inventory contains an unknown gate")
	}
	report.Umbrella.Eligible = len(report.Umbrella.Blockers) == 0 && len(report.Umbrella.DependencyBlockers) == 0
	return report, nil
}
