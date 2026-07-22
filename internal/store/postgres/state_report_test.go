package postgres

import (
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
	"github.com/jackc/pgx/v5/pgtype"
)

// OS-AEC-118. Public seam: the Postgres-backed State-report projection used
// by the authenticated Admin API.
func TestEndpointStateReportNewerComplianceSupersedesHistoricalApplyFailure(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	failureAt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	reportAt := failureAt.Add(time.Hour)
	query := &fakeQuerier{
		byID: map[string]db.Endpoint{
			endpointID: {ID: endpointID, Fleet: "engineering"},
		},
		hasApplyFailure: true,
		latestApplyFailure: db.ApplyFailure{
			EndpointID: endpointID, ReleaseRef: "release-current",
			ResourceAddress: "base/apply", Message: "exit status 1",
			ReportedAt: pgtype.Timestamptz{Time: failureAt, Valid: true},
		},
		hasDriftReport: true,
		latestDriftReport: db.DriftReport{
			EndpointID: endpointID, ReleaseRef: "release-current", Digest: "digest-current",
			ReportJson: []byte(`{"schemaVersion":2,"inCompliance":true,"items":[]}`),
			ReportedAt: pgtype.Timestamptz{Time: reportAt, Valid: true},
		},
	}

	report, ok, err := NewFromQueries(query).GetEndpointStateReport(t.Context(), endpointID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected State report")
	}
	if report.Status != registry.StateCompliant || !report.InCompliance {
		t.Fatalf("current compliance = %q/%t, want compliant/true", report.Status, report.InCompliance)
	}
	if report.ApplyFailure != nil {
		t.Fatalf("current State report retained historical failure: %+v", report.ApplyFailure)
	}

	endpoint, found, err := NewFromQueries(query).GetEndpoint(t.Context(), endpointID)
	if err != nil || !found {
		t.Fatalf("endpoint history lookup found=%t err=%v", found, err)
	}
	if endpoint.LastApplyFailure == nil || endpoint.LastApplyFailure.ResourceAddress != "base/apply" {
		t.Fatalf("historical failure was not retained: %+v", endpoint.LastApplyFailure)
	}
}

// OS-AEC-119. Public seam: the Postgres-backed State-report projection used
// by the authenticated Admin API.
func TestEndpointStateReportCorrelatesCurrentApplyFailureToReleaseAndTime(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	reportAt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name           string
		failureRelease string
		failureAt      time.Time
		wantStatus     registry.StateReportStatus
		wantFailure    bool
	}{
		{name: "same release newer failure is current", failureRelease: "release-current", failureAt: reportAt.Add(time.Minute), wantStatus: registry.StateApplyFailed, wantFailure: true},
		{name: "different release newer failure is historical", failureRelease: "release-other", failureAt: reportAt.Add(time.Minute), wantStatus: registry.StateCompliant},
		{name: "missing failure release cannot override current evidence", failureAt: reportAt.Add(time.Minute), wantStatus: registry.StateCompliant},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &fakeQuerier{
				byID:            map[string]db.Endpoint{endpointID: {ID: endpointID, Fleet: "engineering"}},
				hasApplyFailure: true,
				latestApplyFailure: db.ApplyFailure{
					EndpointID: endpointID, ReleaseRef: test.failureRelease,
					ResourceAddress: "base/apply", Message: "exit status 1",
					ReportedAt: pgtype.Timestamptz{Time: test.failureAt, Valid: true},
				},
				hasDriftReport: true,
				latestDriftReport: db.DriftReport{
					EndpointID: endpointID, ReleaseRef: "release-current", Digest: "digest-current",
					ReportJson: []byte(`{"schemaVersion":2,"inCompliance":true,"items":[]}`),
					ReportedAt: pgtype.Timestamptz{Time: reportAt, Valid: true},
				},
			}
			report, ok, err := NewFromQueries(query).GetEndpointStateReport(t.Context(), endpointID)
			if err != nil || !ok {
				t.Fatalf("State report ok=%t err=%v", ok, err)
			}
			if report.Status != test.wantStatus || (report.ApplyFailure != nil) != test.wantFailure {
				t.Fatalf("status/failure = %q/%t, want %q/%t", report.Status, report.ApplyFailure != nil, test.wantStatus, test.wantFailure)
			}
		})
	}
}

func TestParseDriftReportJSON(t *testing.T) {
	t.Run("compliant", func(t *testing.T) {
		parsed, err := parseDriftReportJSON([]byte(`{"inCompliance":true,"items":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		if !parsed.InCompliance || len(parsed.Items) != 0 {
			t.Fatalf("parsed = %+v", parsed)
		}
	})

	t.Run("drift", func(t *testing.T) {
		parsed, err := parseDriftReportJSON([]byte(`{"inCompliance":false,"items":[{"address":"cfg/a","name":"a","description":"drift"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.InCompliance || len(parsed.Items) != 1 || parsed.Items[0].Address != "cfg/a" {
			t.Fatalf("parsed = %+v", parsed)
		}
	})

	t.Run("reboot required", func(t *testing.T) {
		parsed, err := parseDriftReportJSON([]byte(`{"schemaVersion":4,"inCompliance":true,"items":[],"rebootRequired":{"required":true,"sources":[{"address":"cfg/kernel","provider":"apt"}]}}`))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.RebootRequired == nil || !parsed.RebootRequired.Required || len(parsed.RebootRequired.Sources) != 1 || parsed.RebootRequired.Sources[0].Address != "cfg/kernel" {
			t.Fatalf("parsed = %+v", parsed)
		}
	})
}
