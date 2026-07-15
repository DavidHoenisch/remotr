package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

// OS-SRM-007: operators can see a persisted reboot requirement even when the
// current state report has no Apply results and remains configuration-compliant.
func TestStateReportOutputExposesPersistedRebootRequirement(t *testing.T) {
	report := admin.StateReport{
		EndpointID: "endpoint-1", Fleet: "engineering", ReportedAt: time.Now(),
		InCompliance: true, Status: admin.StateCompliant,
		RebootRequired: &admin.StateReportRebootRequired{Required: true, Sources: []admin.StateReportRebootSource{{
			Address: "base/packages/kernel", Name: "kernel", Provider: "apt",
		}}},
	}

	human := captureStdout(t, func() { printEndpointStateReport(report) })
	for _, want := range []string{"reboot_required: true", "address: base/packages/kernel", "provider: apt", "apply_results: (none)"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}
}

// OS-SRM-009: the operator can distinguish an unchanged boot identity from a
// successful coordinated reboot without seeing raw provider diagnostics.
func TestStateReportOutputExposesCoordinatedRebootTimeout(t *testing.T) {
	report := admin.StateReport{
		EndpointID: "endpoint-1", Fleet: "engineering", ReportedAt: time.Now(),
		InCompliance: true, Status: admin.StateCompliant,
		RebootRequired: &admin.StateReportRebootRequired{
			Required: true, AttemptGeneration: 3,
			Intent: &admin.StateReportRebootIntent{
				Generation: "kernel-6.12.1", Phase: "timed-out", PriorBootID: "boot-1",
				CurrentBootID: "boot-1", AttemptGeneration: 3, Reason: "reboot_timeout_same_boot_id",
			},
		},
	}

	human := captureStdout(t, func() { printEndpointStateReport(report) })
	for _, want := range []string{
		"reboot_phase: timed-out", "reboot_generation: kernel-6.12.1",
		"reboot_attempt_generation: 3", "reboot_prior_boot_id: boot-1",
		"reboot_current_boot_id: boot-1", "reboot_reason: reboot_timeout_same_boot_id",
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}
}

func TestStateReportOutput_exposesStructuredOutcomeBuckets(t *testing.T) {
	report := admin.FleetStateReport{
		Fleet: "engineering",
		Summary: admin.FleetStateSummary{
			Total:       7,
			Compliant:   1,
			Drift:       1,
			Unsupported: 1,
			CheckFailed: 1,
			Deferred:    1,
			ApplyFailed: 1,
			NoReport:    1,
		},
		Endpoints: []admin.StateReport{{
			EndpointID: "endpoint-unsupported",
			Fleet:      "engineering",
			ReportedAt: time.Now(),
			Status:     admin.StateUnsupported,
			Items: []admin.StateReportItem{{
				Address:         "base/firewall",
				Name:            "allow-control",
				Description:     "managed firewall rule",
				Provider:        "nftables",
				Status:          admin.StateUnsupported,
				ReasonCode:      "provider_unavailable",
				DesiredSummary:  "allow tcp/443",
				ObservedSummary: "backend not installed",
				Subresults: []admin.StateReportSubresult{
					{Target: "alice", Status: admin.StateCompliant, ReasonCode: "compliant"},
					{Target: "bob", Status: admin.StateUnsupported, ReasonCode: "provider_unavailable", ObservedSummary: "backend unavailable"},
				},
			}},
		}},
	}

	human := captureStdout(t, func() { printFleetStateReport(report, true) })
	for _, want := range []string{
		"UNSUPPORTED     1",
		"CHECK FAILED    1",
		"DEFERRED        1",
		"APPLY FAILED    1",
		"status: unsupported",
		"provider: nftables",
		"reason_code: provider_unavailable",
		"desired_summary: allow tcp/443",
		"target: alice",
		"target: bob",
	} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}

	raw := captureStdout(t, func() {
		if err := encodeJSON(report); err != nil {
			t.Fatal(err)
		}
	})
	var decoded struct {
		Summary struct {
			Unsupported int `json:"unsupported"`
			CheckFailed int `json:"check_failed"`
			Deferred    int `json:"deferred"`
			ApplyFailed int `json:"apply_failed"`
		} `json:"summary"`
		Endpoints []struct {
			Status string `json:"status"`
			Items  []struct {
				Provider   string `json:"provider"`
				Status     string `json:"status"`
				ReasonCode string `json:"reasonCode"`
				Subresults []struct {
					Target string `json:"target"`
					Status string `json:"status"`
				} `json:"subresults"`
			} `json:"items"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Summary.Unsupported != 1 || decoded.Summary.CheckFailed != 1 || decoded.Summary.Deferred != 1 || decoded.Summary.ApplyFailed != 1 {
		t.Fatalf("JSON summary = %+v", decoded.Summary)
	}
	if len(decoded.Endpoints) != 1 || decoded.Endpoints[0].Status != "unsupported" || len(decoded.Endpoints[0].Items) != 1 || decoded.Endpoints[0].Items[0].Provider != "nftables" || decoded.Endpoints[0].Items[0].ReasonCode != "provider_unavailable" {
		t.Fatalf("JSON endpoints = %+v", decoded.Endpoints)
	}
	if len(decoded.Endpoints[0].Items[0].Subresults) != 2 || decoded.Endpoints[0].Items[0].Subresults[1].Target != "bob" {
		t.Fatalf("JSON subresults = %+v", decoded.Endpoints[0].Items[0].Subresults)
	}
}
