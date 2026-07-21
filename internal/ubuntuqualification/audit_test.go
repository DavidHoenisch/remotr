package ubuntuqualification_test

import (
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
	report, err := ubuntuqualification.GenerateAudit(targets, []ubuntuqualification.DependencyGate{{Name: "shared-foundation", Accepted: true}})
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
