package ubuntuqualification_test

import (
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/ubuntuqualification"
)

// OS-AEC-101: every non-passing evidence state remains an exact blocker in
// both the owning milestone and the umbrella decision.
func TestAuditPreservesExactBlockingRow(t *testing.T) {
	targets := []ubuntuqualification.AuditTarget{
		{Milestone: "M1", RowKey: "file/posix/file-v1/container", Status: "blocked", Selector: "make:m1-file"},
		{Milestone: "M2", RowKey: "user/shadow/user-v1/vm", Status: "planned", Selector: "make:m2-user"},
		{Milestone: "M3", RowKey: "mount/util-linux/mount-v1/vm", Status: "missing", Selector: "make:m3-mount"},
		{Milestone: "M4", RowKey: "service/systemd/service-state-v1/vm", Status: "skipped", Selector: "make:m4-service"},
		{Milestone: "M5", RowKey: "auditRules/auditd/auditRules-v1/vm", Status: "failing", Selector: "make:m5-audit"},
		{Milestone: "M5", RowKey: "browserPolicy/firefox/browserPolicy-v1/vm", Status: "untested", Selector: "make:m5-browser"},
	}
	report, err := ubuntuqualification.GenerateAudit(targets, acceptedAuditDependencies())
	if err != nil {
		t.Fatal(err)
	}
	if report.Umbrella.Eligible {
		t.Fatal("umbrella became eligible with blocking evidence states")
	}
	if len(report.Umbrella.Blockers) != len(targets) {
		t.Fatalf("umbrella blockers = %+v, want all %d exact targets", report.Umbrella.Blockers, len(targets))
	}

	want := make(map[string]ubuntuqualification.AuditTarget, len(targets))
	for _, target := range targets {
		want[target.RowKey] = target
	}
	for _, blocker := range report.Umbrella.Blockers {
		target, ok := want[blocker.RowKey]
		if !ok {
			t.Errorf("unexpected umbrella blocker %+v", blocker)
			continue
		}
		if blocker.Milestone != target.Milestone || blocker.Status != target.Status || blocker.Selector != target.Selector {
			t.Errorf("blocker = %+v, want exact target %+v", blocker, target)
		}
		delete(want, blocker.RowKey)
	}
	if len(want) != 0 {
		t.Errorf("audit dropped exact blockers: %+v", want)
	}

	for _, milestone := range report.Milestones {
		if milestone.Complete {
			t.Errorf("%s reported complete with blockers %+v", milestone.Name, milestone.Blockers)
		}
		if len(milestone.Blockers) == 0 {
			t.Errorf("%s lost its exact blocker", milestone.Name)
		}
	}
}

// OS-AEC-103 repository seam: the positive decision is derived from the
// checked-in exact manifest, matrix, traceability, and dependency gates.
func TestCheckedInQualificationAuditClosesOnlyOnExactEvidence(t *testing.T) {
	report, err := ubuntuqualification.LoadRepositoryAudit(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Umbrella.Eligible {
		t.Fatalf("checked-in qualification audit is blocked: %+v", report.Umbrella)
	}
	if len(report.Milestones) != 5 {
		t.Fatalf("milestone decisions = %d, want 5", len(report.Milestones))
	}
	if len(report.QualifiedTargets) != 44 || len(report.DescopedTargets) != 10 {
		t.Fatalf("checked-in audit targets = %d qualified, %d descoped; want 44 and 10", len(report.QualifiedTargets), len(report.DescopedTargets))
	}
	for _, milestone := range report.Milestones {
		if !milestone.Complete || len(milestone.Blockers) != 0 {
			t.Errorf("checked-in %s decision = %+v, want complete", milestone.Name, milestone)
		}
	}
}

// OS-AEC-103: an eligible umbrella decision requires every milestone and every
// independently governed sibling workstream to have a positive terminal state.
func TestAuditClosesOnlyAfterAllGatesPass(t *testing.T) {
	targets := []ubuntuqualification.AuditTarget{
		{Milestone: "M1", RowKey: "file/posix/file-v1/container", Status: "qualified", Selector: "make:m1-file"},
		{Milestone: "M2", RowKey: "user/shadow/user-v1/vm", Status: "qualified", Selector: "make:m2-user"},
		{Milestone: "M3", RowKey: "mount/util-linux/mount-v1/vm", Status: "qualified", Selector: "make:m3-mount"},
		{Milestone: "M4", RowKey: "service/systemd/service-state-v1/vm", Status: "qualified", Selector: "make:m4-service"},
		{Milestone: "M5", RowKey: "auditRules/auditd/auditRules-v1/vm", Status: "qualified", Selector: "make:m5-audit"},
		{Milestone: "M5", RowKey: "browserPolicy/edge/browserPolicy-v1/vm", Status: "unadvertised", Selector: "openspec:UHF-104", ExplicitlyDescoped: true},
	}
	dependencies := acceptedAuditDependencies()

	report, err := ubuntuqualification.GenerateAudit(targets, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Umbrella.Eligible {
		t.Fatalf("fully passing audit is not eligible: %+v", report.Umbrella)
	}
	for _, milestone := range report.Milestones {
		if !milestone.Complete {
			t.Errorf("%s is incomplete: %+v", milestone.Name, milestone.Blockers)
		}
	}

	for index := range dependencies {
		candidate := append([]ubuntuqualification.DependencyGate(nil), dependencies...)
		candidate[index].Accepted = false
		candidate[index].Reason = "not accepted"
		report, err := ubuntuqualification.GenerateAudit(targets, candidate)
		if err != nil {
			t.Fatal(err)
		}
		if report.Umbrella.Eligible || len(report.Umbrella.DependencyBlockers) != 1 ||
			report.Umbrella.DependencyBlockers[0].Name != candidate[index].Name {
			t.Errorf("unaccepted dependency %q did not remain the exact blocker: %+v", candidate[index].Name, report.Umbrella)
		}
	}

	if _, err := ubuntuqualification.GenerateAudit(targets[:4], dependencies); err == nil {
		t.Fatal("audit accepted a target inventory with no M5 row")
	}
	if _, err := ubuntuqualification.GenerateAudit(targets, dependencies[:3]); err == nil {
		t.Fatal("audit accepted an incomplete dependency inventory")
	}
}

func acceptedAuditDependencies() []ubuntuqualification.DependencyGate {
	return []ubuntuqualification.DependencyGate{
		{Name: "complete-applicator-execution-contract", Accepted: true},
		{Name: "complete-capability-compatible-delivery", Accepted: true},
		{Name: "complete-core-package-providers", Accepted: true},
		{Name: "establish-testing-and-performance-foundation", Accepted: true},
	}
}
