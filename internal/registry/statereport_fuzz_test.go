package registry

import (
	"encoding/json"
	"testing"
	"time"
)

func FuzzStateReportJSONRoundTrip(f *testing.F) {
	f.Add("endpoint-1", "fleet-a", "cfg/pkg", "description")

	f.Fuzz(func(t *testing.T, endpoint, fleet, address, description string) {
		if len(endpoint)+len(fleet)+len(address)+len(description) > 4096 {
			return
		}
		report := StateReport{
			EndpointID: endpoint, Fleet: fleet,
			Items: []StateReportItem{{Address: address, Description: description}},
			RebootRequired: &StateReportRebootRequired{
				Required: true, Sources: []StateReportRebootSource{{Address: address, Provider: "fuzz-provider"}}, AttemptGeneration: 2,
				Intent:     &StateReportRebootIntent{Generation: "g2", Phase: "timed-out", PriorBootID: "boot-2", CurrentBootID: "boot-2", AttemptGeneration: 2, Reason: "reboot_timeout_same_boot_id"},
				Completion: &StateReportRebootCompletion{Generation: "g1", BootID: "boot-2", AttemptGeneration: 1, CompletedAt: time.Unix(1, 0).UTC()},
			},
		}
		raw, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 8192 {
			t.Fatal("serialized report exceeded bounded fixture size")
		}
		var decoded StateReport
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.EndpointID != endpoint || decoded.Fleet != fleet || len(decoded.Items) != 1 || decoded.Items[0].Address != address || decoded.Items[0].Description != description || decoded.RebootRequired == nil || !decoded.RebootRequired.Required || len(decoded.RebootRequired.Sources) != 1 || decoded.RebootRequired.Sources[0].Address != address || decoded.RebootRequired.Intent == nil || decoded.RebootRequired.Intent.Reason != "reboot_timeout_same_boot_id" || decoded.RebootRequired.Completion == nil || decoded.RebootRequired.Completion.Generation != "g1" {
			t.Fatal("state report changed after JSON round trip")
		}
	})
}
