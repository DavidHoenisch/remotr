package registry_test

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

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
