package main

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

func TestAuditJSONOutputRejectsUnclassifiedDetailsAndPreservesSafeProjection(t *testing.T) {
	const canary = "audit-cli-secret-canary"
	unsafe := admin.AuditEventPage{Events: []admin.AuditEvent{{
		ID: "event", Details: &executor.SafeSummary{Fields: []executor.SafeField{{
			Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafeValue, Text: canary,
		}}},
	}}}
	var unsafeErr error
	unsafeOutput := captureStdout(t, func() { unsafeErr = encodeJSON(unsafe) })
	if unsafeErr == nil {
		t.Fatal("unclassified audit details were encoded")
	}
	if strings.Contains(unsafeOutput, canary) {
		t.Fatalf("rejected audit output leaked canary: %s", unsafeOutput)
	}

	present := true
	safe := admin.AuditEventPage{Events: []admin.AuditEvent{{
		ID: "event", Details: &executor.SafeSummary{Fields: []executor.SafeField{{
			Path: "secret", Sensitivity: executor.SafeSecret, Projection: executor.SafePresence, Present: &present,
		}}},
	}}}
	safeOutput := captureStdout(t, func() {
		if err := encodeJSON(safe); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(safeOutput, `"sensitivity": "secret"`) || !strings.Contains(safeOutput, `"present": true`) || strings.Contains(safeOutput, canary) {
		t.Fatalf("classified audit JSON output = %s", safeOutput)
	}
}
