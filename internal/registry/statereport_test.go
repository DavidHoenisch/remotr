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

// OS-SRM-009: the Admin state-report seam preserves a non-successful
// same-boot attempt and its stable reason for operator diagnosis.
func TestStateReportRetainsCoordinatedRebootOperation(t *testing.T) {
	payload, err := registry.ParseStateReportPayload([]byte(`{
		"schemaVersion":5,
		"inCompliance":true,
		"items":[],
		"rebootRequired":{
			"required":true,
			"attemptGeneration":3,
			"intent":{"generation":"kernel-6.12.1","phase":"timed-out","priorBootId":"boot-1","currentBootId":"boot-1","attemptGeneration":3,"reason":"reboot_timeout_same_boot_id"},
			"completion":{"generation":"kernel-6.11.9","bootId":"boot-1","attemptGeneration":2,"completedAt":"2026-07-12T02:00:00Z"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	status := payload.RebootRequired
	if status == nil || status.Intent == nil || status.Completion == nil || status.AttemptGeneration != 3 || status.Intent.Phase != "timed-out" || status.Intent.Reason != "reboot_timeout_same_boot_id" || status.Intent.CurrentBootID != "boot-1" || status.Completion.Generation != "kernel-6.11.9" {
		t.Fatalf("coordinated reboot operation = %+v", status)
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
