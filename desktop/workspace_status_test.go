package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

var workspaceStatusNow = time.Date(2032, time.March, 4, 5, 6, 7, 0, time.UTC)

func TestWorkspaceCanonicalComplianceAndLabelProjection(t *testing.T) {
	workspace := loadStatusWorkspace(t)
	rows := endpointRowsByID(workspace.Endpoints)

	tests := []struct {
		name       string
		endpointID string
		want       ComplianceStatus
	}{
		{name: "compliant", endpointID: "status-compliant", want: ComplianceCompliant},
		{name: "drifted", endpointID: "status-drifted", want: ComplianceDrifted},
		{name: "unsupported", endpointID: "status-unsupported", want: ComplianceUnsupported},
		{name: "check failed", endpointID: "status-check-failed", want: ComplianceCheckFailed},
		{name: "deferred", endpointID: "status-deferred", want: ComplianceDeferred},
		{name: "apply failed", endpointID: "status-apply-failed", want: ComplianceApplyFailed},
		{name: "explicit no report", endpointID: "status-no-report", want: ComplianceNotReported},
		{name: "missing State report", endpointID: "without-state-report", want: ComplianceNotReported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row, ok := rows[test.endpointID]
			if !ok {
				t.Fatalf("Endpoint %q is missing from workspace", test.endpointID)
			}
			if row.Compliance != test.want {
				t.Errorf("Endpoint %q compliance = %q, want %q", test.endpointID, row.Compliance, test.want)
			}
		})
	}

	labelRow := rows["status-compliant"]
	wantLabels := []LabelView{
		{Key: "environment", Value: "production"},
		{Key: "region", Value: "west"},
		{Key: "tier", Value: "api"},
	}
	if !slices.Equal(labelRow.Labels, wantLabels) {
		t.Errorf("deterministic Labels = %#v, want %#v", labelRow.Labels, wantLabels)
	}

	withoutReport := rows["without-state-report"]
	if withoutReport.Freshness != FreshnessRecent || withoutReport.EvidenceAt == nil {
		t.Errorf("Endpoint without State report = %#v, want not-reported compliance with independent recent check-in", withoutReport)
	}
}

func TestWorkspaceFreshnessUsesExactTenMinuteBoundary(t *testing.T) {
	workspace := loadStatusWorkspace(t)
	rows := endpointRowsByID(workspace.Endpoints)

	tests := []struct {
		name       string
		endpointID string
		want       FreshnessStatus
	}{
		{name: "within threshold", endpointID: "status-compliant", want: FreshnessRecent},
		{name: "exactly ten minutes", endpointID: "freshness-exact", want: FreshnessRecent},
		{name: "one nanosecond beyond threshold", endpointID: "freshness-beyond", want: FreshnessStale},
		{name: "never checked in", endpointID: "freshness-never", want: FreshnessNeverReported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row, ok := rows[test.endpointID]
			if !ok {
				t.Fatalf("Endpoint %q is missing from workspace", test.endpointID)
			}
			if row.Freshness != test.want {
				t.Errorf("Endpoint %q freshness = %q, want %q", test.endpointID, row.Freshness, test.want)
			}
		})
	}

	staleCompliant := rows["freshness-beyond"]
	if staleCompliant.Compliance != ComplianceCompliant || staleCompliant.Freshness != FreshnessStale {
		t.Errorf("independent compliance/freshness = %q/%q, want %q/%q", staleCompliant.Compliance, staleCompliant.Freshness, ComplianceCompliant, FreshnessStale)
	}
	never := rows["freshness-never"]
	if never.EvidenceAt != nil {
		t.Errorf("never-reported Endpoint evidence time = %q, want absent", *never.EvidenceAt)
	}
}

func TestWorkspaceFreshnessUsesLocalThresholdPreference(t *testing.T) {
	workspace := loadStatusWorkspace(t, WithWorkspaceFreshnessThreshold(5*time.Minute))
	rows := endpointRowsByID(workspace.Endpoints)

	if got := rows["status-compliant"].Freshness; got != FreshnessRecent {
		t.Errorf("exactly five-minute check-in freshness = %q, want %q", got, FreshnessRecent)
	}
	if got := rows["freshness-exact"].Freshness; got != FreshnessStale {
		t.Errorf("ten-minute check-in with five-minute preference = %q, want %q", got, FreshnessStale)
	}
}

func loadStatusWorkspace(t *testing.T, options ...WorkspaceOption) WorkspaceView {
	t.Helper()
	profile := newStatusWorkspaceProfile(t)
	options = append([]WorkspaceOption{WithWorkspaceClock(func() time.Time {
		return workspaceStatusNow
	})}, options...)
	workspace, err := NewWorkspaceService(options...).Load(t.Context(), profile)
	if err != nil {
		t.Fatalf("load status workspace: %v", err)
	}
	return workspace
}

func newStatusWorkspaceProfile(t *testing.T) ConnectionProfile {
	t.Helper()
	tlsFixture := newConnectionTLSFixture(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "GET /v1/admin/me":
			writeWorkspaceJSON(response, `{"operator_id":"operator-status","roles":["read_only"]}`)
		case "GET /v1/admin/fleets":
			writeWorkspaceJSON(response, `["production"]`)
		case "GET /v1/admin/endpoints":
			writeWorkspaceJSON(response, `[
				{"id":"status-compliant","fleet":"production","labels":{"tier":"api","region":"west","environment":"production"},"last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"status-drifted","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"status-unsupported","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"status-check-failed","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"status-deferred","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"status-apply-failed","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"status-no-report","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:01:07Z"}},
				{"id":"without-state-report","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T05:05:07Z"}},
				{"id":"freshness-exact","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T04:56:07Z"}},
				{"id":"freshness-beyond","fleet":"production","last_check_in":{"release_ref":"release-42","digest":"digest","at":"2032-03-04T04:56:06.999999999Z"}},
				{"id":"freshness-never","fleet":"production"}
			]`)
		case "GET /v1/admin/fleets/production/state-report":
			writeWorkspaceJSON(response, `{"fleet":"production","summary":{"total":11},"endpoints":[
				{"endpoint_id":"status-compliant","fleet":"production","status":"compliant","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"status-drifted","fleet":"production","status":"drifted","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"status-unsupported","fleet":"production","status":"unsupported","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"status-check-failed","fleet":"production","status":"check_failed","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"status-deferred","fleet":"production","status":"deferred","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"status-apply-failed","fleet":"production","status":"apply_failed","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"status-no-report","fleet":"production","status":"no_report","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"freshness-exact","fleet":"production","status":"compliant","reported_at":"2032-03-04T05:01:07Z"},
				{"endpoint_id":"freshness-beyond","fleet":"production","status":"compliant","reported_at":"2032-03-04T05:01:07Z"}
			]}`)
		case "GET /v1/admin/change-requests":
			writeWorkspaceJSON(response, `[]`)
		case "GET /v1/admin/audit-events":
			writeWorkspaceJSON(response, `{"events":[]}`)
		default:
			http.NotFound(response, request)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{tlsFixture.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    connectionCertPool(t, tlsFixture.caPEM),
		MinVersion:   tls.VersionTLS12,
		Time: func() time.Time {
			return connectionTestTime
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := tlsFixture.saveClientState(
		t,
		"operator-status",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		tlsFixture.caPEM,
	)
	return connectionProfileForServer(t, "Status mapping", server.URL, stateDir)
}

func endpointRowsByID(rows []EndpointRow) map[string]EndpointRow {
	byID := make(map[string]EndpointRow, len(rows))
	for _, row := range rows {
		byID[row.EndpointID] = row
	}
	return byID
}
