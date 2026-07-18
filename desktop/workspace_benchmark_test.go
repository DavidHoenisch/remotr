package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

var benchmarkWorkspaceSink WorkspaceView

func BenchmarkWorkspaceComposition(b *testing.B) {
	for _, endpointCount := range []int{10, 100, 500, 1000} {
		input := benchmarkWorkspaceInput(endpointCount)
		service := NewWorkspaceService(WithWorkspaceFreshnessThreshold(10 * time.Minute))
		b.Run(fmt.Sprintf("endpoints_%d", endpointCount), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				benchmarkWorkspaceSink = service.composeWorkspace(input)
			}
		})
	}
}

func benchmarkWorkspaceInput(endpointCount int) workspaceCompositionInput {
	loadedAt := time.Date(2032, time.March, 4, 5, 6, 7, 0, time.UTC)
	fleetCount := max(1, endpointCount/100)
	fleets := make([]string, fleetCount)
	reports := make([]admin.FleetStateReport, fleetCount)
	for fleetIndex := range fleetCount {
		fleet := fmt.Sprintf("fleet-%02d", fleetIndex)
		fleets[fleetIndex] = fleet
		reports[fleetIndex] = admin.FleetStateReport{Fleet: fleet}
	}

	statuses := []admin.StateReportStatus{
		admin.StateCompliant,
		admin.StateDrifted,
		admin.StateUnsupported,
		admin.StateCheckFailed,
		admin.StateDeferred,
		admin.StateApplyFailed,
		admin.StateNoReport,
	}
	endpoints := make([]admin.Endpoint, 0, endpointCount)
	for endpointIndex := range endpointCount {
		fleetIndex := endpointIndex % fleetCount
		fleet := fleets[fleetIndex]
		endpointID := fmt.Sprintf("endpoint-%04d", endpointIndex)
		endpoint := admin.Endpoint{
			ID:                   endpointID,
			Fleet:                fleet,
			DesiredAgentVersion:  "v2.0.0",
			ReportedAgentVersion: fmt.Sprintf("v2.%d.0", endpointIndex%4),
			Usernames:            []string{fmt.Sprintf("user-%03d", endpointIndex%100)},
			Labels: map[string]string{
				"environment": "production",
				"region":      fmt.Sprintf("region-%d", endpointIndex%5),
				"tier":        fmt.Sprintf("tier-%d", endpointIndex%3),
			},
		}
		if endpointIndex%3 != 2 {
			age := time.Duration(endpointIndex%20) * time.Minute
			endpoint.LastCheckIn = &admin.CheckInSummary{
				ReleaseRef: "release-benchmark",
				Digest:     fmt.Sprintf("digest-%04d", endpointIndex),
				At:         loadedAt.Add(-age),
			}
		}
		endpoints = append(endpoints, endpoint)
		status := statuses[endpointIndex%len(statuses)]
		reports[fleetIndex].Endpoints = append(reports[fleetIndex].Endpoints, admin.StateReport{
			EndpointID: endpointID,
			Fleet:      fleet,
			ReleaseRef: "release-benchmark",
			Digest:     fmt.Sprintf("state-%04d", endpointIndex),
			ReportedAt: loadedAt.Add(-time.Duration(endpointIndex%30) * time.Minute),
			Status:     status,
			Items: []admin.StateReportItem{{
				Address:  fmt.Sprintf("packages/package-%d", endpointIndex%25),
				Name:     fmt.Sprintf("package-%d", endpointIndex%25),
				Provider: "packages",
				Status:   status,
				DesiredSummary: executor.SafeSummary{Fields: []executor.SafeField{{
					Path: "state", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: "installed",
				}}},
				ObservedSummary: executor.SafeSummary{Fields: []executor.SafeField{{
					Path: "state", Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: "installed",
				}}},
			}},
		})
	}

	changeCount := max(1, endpointCount/100)
	changes := make([]admin.ChangeRequest, 0, changeCount)
	for changeIndex := range changeCount {
		changes = append(changes, admin.ChangeRequest{
			ID:                 fmt.Sprintf("change-%03d", changeIndex),
			Fleet:              fleets[changeIndex%fleetCount],
			ReleaseRef:         "release-benchmark",
			Risk:               models.RiskConnectivity,
			AuthorizationState: changecontrol.AuthorizationPending,
			RequiredApprovals:  1,
			FrozenTargets: []changecontrol.TargetEvidence{{
				EndpointID: fmt.Sprintf("endpoint-%04d", changeIndex%endpointCount),
			}},
			AuditHistory: []changecontrol.AuditEntry{{
				At:      loadedAt.Add(-time.Duration(changeIndex) * time.Minute),
				ActorID: "operator-benchmark",
				Action:  changecontrol.AuditCreated,
			}},
			CreatedAt: loadedAt.Add(-time.Hour),
		})
	}

	activityCount := min(endpointCount, activityPageLimit)
	activity := make([]admin.AuditEvent, 0, activityCount)
	for eventIndex := range activityCount {
		activity = append(activity, admin.AuditEvent{
			ID:           fmt.Sprintf("event-%03d", eventIndex),
			OccurredAt:   loadedAt.Add(-time.Duration(eventIndex) * time.Second),
			RequestID:    fmt.Sprintf("request-%03d", eventIndex),
			ActorType:    "operator",
			ActorID:      "operator-benchmark",
			Action:       "git_sync",
			StatusCode:   200,
			ResourceType: "server",
			ResourceID:   "primary",
			Details: map[string]any{
				"release_ref": "release-benchmark",
				"sequence":    float64(eventIndex),
			},
		})
	}

	return workspaceCompositionInput{
		identity: OperatorIdentity{
			OperatorID: "operator-benchmark",
			Roles:      []string{"read_only", "auditor"},
		},
		fleets:            fleets,
		endpoints:         endpoints,
		fleetReports:      reports,
		fleetReportErrors: make([]error, len(reports)),
		changes:           changes,
		activity: admin.AuditEventPage{
			Events:     activity,
			NextCursor: "cursor-benchmark",
		},
		loadedAt: loadedAt,
	}
}
