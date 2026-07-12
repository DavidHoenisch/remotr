package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

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
}
