package sync

import (
	"encoding/json"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/executor"
)

func FuzzComplianceReportSerializationStaysBounded(f *testing.F) {
	f.Add("ordinary summary", uint16(1))
	f.Add("long summary", uint16(160))

	f.Fuzz(func(t *testing.T, summary string, rawCount uint16) {
		if len(summary) > 1024 {
			return
		}
		count := int(rawCount%160) + 1
		items := make([]engine.DriftItem, count)
		safeSummary, err := executor.NewSafeSummary([]executor.SafeField{{
			Path: "fuzz", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: summary,
		}})
		if err != nil {
			return
		}
		for i := range items {
			items[i] = engine.DriftItem{
				Address: summary, Name: summary, Description: summary, Provider: summary,
				Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift,
				DesiredSummary: safeSummary, ObservedSummary: safeSummary,
			}
		}
		var pending Pending
		pending.SetFromPipeline(nil, engine.DriftReport{Items: items}, engine.ApplyResult{}, nil, "sha256:fuzz")
		if pending.Drift == nil || len(pending.Drift.Report) > MaxComplianceReportBytes || !json.Valid(pending.Drift.Report) {
			t.Fatalf("compliance report is missing, invalid, or exceeds %d bytes: %d", MaxComplianceReportBytes, len(pending.Drift.Report))
		}
	})
}
