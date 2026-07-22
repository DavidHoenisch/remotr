package registry_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestParseStateReportPayloadAdmitsOnlyClassifiedVersion7Summaries(t *testing.T) {
	const canary = "legacy-state-report-secret-canary"
	legacy, err := registry.ParseStateReportPayload([]byte(`{"schemaVersion":6,"inCompliance":false,"items":[{"address":"base/file","desiredSummary":"` + canary + `"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacyJSON), canary) {
		t.Fatalf("legacy unclassified summary survived admission: %s", legacyJSON)
	}

	unsafe := []byte(`{"schemaVersion":7,"inCompliance":false,"items":[{"address":"base/file","desiredSummary":{"fields":[{"path":"content","sensitivity":"secret","projection":"value","text":"` + canary + `"}]}}]}`)
	if _, err := registry.ParseStateReportPayload(unsafe); err == nil {
		t.Fatal("version-7 secret raw-value summary was accepted")
	}
}

func TestParseStateReportPayloadVersion8RejectsUnverifiableResourceHashes(t *testing.T) {
	const hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	valid := []byte(`{"schemaVersion":8,"inCompliance":false,"items":[{"address":"base/file","name":"file","provider":"files","providerRevision":"file-v1","effectiveHash":"` + hash + `","status":"drifted"}],"apply":[{"address":"base/file","name":"file","provider":"files","providerRevision":"file-v1","effectiveHash":"` + hash + `","status":"changed"}]}`)
	payload, err := registry.ParseStateReportPayload(valid)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Items[0].EffectiveHash != hash || payload.Items[0].ProviderRevision != "file-v1" || payload.Apply[0].EffectiveHash != hash {
		t.Fatalf("canonical identities were not preserved: %+v", payload)
	}

	tests := []struct {
		name string
		raw  string
	}{
		{"missing hash", `{"schemaVersion":8,"items":[{"address":"base/file","provider":"files","providerRevision":"file-v1"}]}`},
		{"malformed hash", `{"schemaVersion":8,"items":[{"address":"base/file","provider":"files","providerRevision":"file-v1","effectiveHash":"sha256:not-a-digest"}]}`},
		{"missing revision", `{"schemaVersion":8,"items":[{"address":"base/file","provider":"files","effectiveHash":"` + hash + `"}]}`},
		{"duplicate address", `{"schemaVersion":8,"items":[{"address":"base/file","provider":"files","providerRevision":"file-v1","effectiveHash":"` + hash + `"},{"address":"base/file","provider":"files","providerRevision":"file-v1","effectiveHash":"` + hash + `"}]}`},
		{"conflicting apply hash", `{"schemaVersion":8,"items":[{"address":"base/file","provider":"files","providerRevision":"file-v1","effectiveHash":"` + hash + `"}],"apply":[{"address":"base/file","provider":"files","providerRevision":"file-v1","effectiveHash":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","status":"changed"}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := registry.ParseStateReportPayload([]byte(test.raw)); err == nil {
				t.Fatalf("version-8 report was accepted: %s", test.raw)
			}
		})
	}
}

func TestParseStateReportPayloadVersion9RequiresClosedPreflightEvidence(t *testing.T) {
	const hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	valid := []byte(`{"schemaVersion":9,"items":[{"address":"base/sudo","provider":"sudo","providerRevision":"sudo-v1","effectiveHash":"` + hash + `","status":"drifted","preflightStatus":"ready","preflightReason":"preflight_ready"}]}`)
	payload, err := registry.ParseStateReportPayload(valid)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Items[0].PreflightStatus != registry.PlanPreflightReady || payload.Items[0].PreflightReason != "preflight_ready" {
		t.Fatalf("preflight evidence = %+v", payload.Items[0])
	}

	tests := []string{
		`{"schemaVersion":9,"items":[{"address":"base/sudo","provider":"sudo","providerRevision":"sudo-v1","effectiveHash":"` + hash + `","preflightReason":"preflight_ready"}]}`,
		`{"schemaVersion":9,"items":[{"address":"base/sudo","provider":"sudo","providerRevision":"sudo-v1","effectiveHash":"` + hash + `","preflightStatus":"unknown","preflightReason":"preflight_ready"}]}`,
		`{"schemaVersion":9,"items":[{"address":"base/sudo","provider":"sudo","providerRevision":"sudo-v1","effectiveHash":"` + hash + `","preflightStatus":"ready","preflightReason":"provider said secret value"}]}`,
		`{"schemaVersion":9,"items":[{"address":"base/sudo","provider":"sudo","providerRevision":"sudo-v1","effectiveHash":"` + hash + `","preflightStatus":"not_required","preflightReason":"preflight_ready"}]}`,
	}
	for _, raw := range tests {
		if _, err := registry.ParseStateReportPayload([]byte(raw)); err == nil {
			t.Fatalf("version-9 report accepted invalid preflight evidence: %s", raw)
		}
	}
}

func TestParseStateReportPayloadVersion10RequiresClosedActivationBootstrapIdentity(t *testing.T) {
	const hash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	valid := []byte(`{"schemaVersion":10,"items":[{"address":"subscriptions/primary","provider":"ubuntu-pro","providerRevision":"ubuntu-pro-v1","effectiveHashStatus":"authorization_required","status":"drifted","preflightStatus":"ready","preflightReason":"preflight_ready"},{"address":"base/file","provider":"file","providerRevision":"file-v1","effectiveHash":"` + hash + `","status":"compliant","preflightStatus":"not_required"}]}`)
	parsed, err := registry.ParseStateReportPayload(valid)
	if err != nil {
		t.Fatalf("parse activation bootstrap evidence: %v", err)
	}
	if parsed.SchemaVersion != 10 || len(parsed.Items) != 2 || parsed.Items[0].EffectiveHash != "" || parsed.Items[1].EffectiveHash != hash {
		t.Fatalf("activation bootstrap evidence = %+v", parsed)
	}

	invalid := [][]byte{
		[]byte(`{"schemaVersion":10,"items":[{"address":"subscriptions/primary","provider":"ubuntu-pro","providerRevision":"ubuntu-pro-v1","status":"drifted","preflightStatus":"ready","preflightReason":"preflight_ready"}]}`),
		[]byte(`{"schemaVersion":10,"items":[{"address":"subscriptions/primary","provider":"ubuntu-pro","providerRevision":"ubuntu-pro-v1","effectiveHashStatus":"provider_error","status":"drifted","preflightStatus":"ready","preflightReason":"preflight_ready"}]}`),
		[]byte(`{"schemaVersion":10,"items":[{"address":"subscriptions/primary","provider":"ubuntu-pro","providerRevision":"ubuntu-pro-v1","effectiveHash":"` + hash + `","effectiveHashStatus":"authorization_required","status":"drifted","preflightStatus":"ready","preflightReason":"preflight_ready"}]}`),
		[]byte(`{"schemaVersion":10,"items":[{"address":"subscriptions/primary","provider":"ubuntu-pro","effectiveHashStatus":"authorization_required","status":"drifted","preflightStatus":"ready","preflightReason":"preflight_ready"}]}`),
	}
	for index, raw := range invalid {
		if _, err := registry.ParseStateReportPayload(raw); err == nil {
			t.Fatalf("invalid activation bootstrap evidence %d was accepted", index)
		}
	}
}

func TestMemoryStateReportsPreserveAuthenticatedSchemaVersion(t *testing.T) {
	memory := registry.NewMemory()
	if err := memory.RegisterEndpoint(registry.Endpoint{ID: "endpoint-a", Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	memory.SetEndpointStateReport("endpoint-a", registry.DriftSummary{ReleaseRef: "release-1", Digest: "sha256:artifact", ReportedAt: time.Unix(1, 0)}, registry.StateReportPayload{SchemaVersion: 9})

	report, ok, err := memory.GetEndpointStateReport(t.Context(), "endpoint-a")
	if err != nil || !ok || report.SchemaVersion != 9 {
		t.Fatalf("endpoint report = %+v ok=%v err=%v", report, ok, err)
	}
	fleet, err := memory.ListFleetStateReports(t.Context(), "engineering")
	if err != nil || len(fleet.Endpoints) != 1 || fleet.Endpoints[0].SchemaVersion != 9 {
		t.Fatalf("fleet report = %+v err=%v", fleet, err)
	}
}

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
