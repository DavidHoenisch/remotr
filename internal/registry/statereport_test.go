package registry_test

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

// OS-SRM-007: authenticated state-report parsing preserves reboot-required as
// operational state without changing configuration compliance classification.
func TestStateReportRetainsRebootRequirementOutsideApplyResults(t *testing.T) {
	payload, err := registry.ParseStateReportPayload([]byte(`{
		"schemaVersion":4,
		"inCompliance":true,
		"items":[],
		"rebootRequired":{"required":true,"sources":[{"address":"base/packages/kernel","name":"kernel","provider":"apt"}]}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if payload.RebootRequired == nil || !payload.RebootRequired.Required || len(payload.RebootRequired.Sources) != 1 || payload.RebootRequired.Sources[0].Address != "base/packages/kernel" {
		t.Fatalf("reboot-required payload = %+v", payload.RebootRequired)
	}
	report := registry.StateReport{
		ReportedAt: time.Unix(1, 0), InCompliance: payload.InCompliance,
		Items: payload.Items, RebootRequired: payload.RebootRequired,
	}
	if status := registry.ClassifyStateReport(report); status != registry.StateCompliant {
		t.Fatalf("ClassifyStateReport() = %q, want compliant; report=%+v", status, report)
	}
}

// OS-ESM-009: the server preserves failed local run history separately while
// classifying the matching schedule configuration as compliant.
func TestStateReportSeparatesScheduleRuntimeFromCompliance(t *testing.T) {
	payload, err := registry.ParseStateReportPayload([]byte(`{
		"schemaVersion":3,
		"inCompliance":true,
		"items":[{"address":"base/nightly","name":"nightly","status":"compliant","reasonCode":"compliant"}],
		"scheduleRuntime":[{"address":"base/nightly","name":"nightly","provider":"endpoint-schedule/systemd-timer","status":"failed","exitCode":9,"missedRunBehavior":"catch-up"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.ScheduleRuntime) != 1 || payload.ScheduleRuntime[0].Status != "failed" || payload.ScheduleRuntime[0].ExitCode == nil || *payload.ScheduleRuntime[0].ExitCode != 9 {
		t.Fatalf("schedule runtime = %+v", payload.ScheduleRuntime)
	}
	report := registry.StateReport{
		ReportedAt: time.Unix(1, 0), InCompliance: payload.InCompliance,
		Items: payload.Items, ScheduleRuntime: payload.ScheduleRuntime,
	}
	if status := registry.ClassifyStateReport(report); status != registry.StateCompliant {
		t.Fatalf("ClassifyStateReport() = %q, want compliant; report=%+v", status, report)
	}
}
