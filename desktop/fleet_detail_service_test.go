package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFleetDetailServiceAggregatesMixedAndEmptyFleets(t *testing.T) {
	tests := []struct {
		name  string
		fleet string
		check func(*testing.T, FleetDetailView)
	}{
		{
			name:  "mixed member evidence",
			fleet: "mixed",
			check: func(t *testing.T, detail FleetDetailView) {
				t.Helper()
				if detail.Fleet != "mixed" || detail.Summary.Fleet != "mixed" || detail.Summary.EndpointCount != 3 {
					t.Fatalf("mixed Fleet identity/count = %#v, want exact Fleet with three members", detail)
				}
				if got := endpointIDs(detail.Members); !slices.Equal(got, []string{"endpoint-a", "endpoint-b", "endpoint-c"}) {
					t.Fatalf("mixed Fleet member IDs = %v, want stable exact members", got)
				}
				wantCompliance := map[string]int{
					string(ComplianceCompliant):   1,
					string(ComplianceDrifted):     1,
					string(ComplianceNotReported): 1,
				}
				if got := nonzeroStatusCounts(detail.Summary.Compliance); !mapsEqual(got, wantCompliance) {
					t.Errorf("mixed compliance counts = %#v, want %#v", got, wantCompliance)
				}
				wantFreshness := map[string]int{
					string(FreshnessRecent):        1,
					string(FreshnessStale):         1,
					string(FreshnessNeverReported): 1,
				}
				if got := nonzeroStatusCounts(detail.Summary.Freshness); !mapsEqual(got, wantFreshness) {
					t.Errorf("mixed freshness counts = %#v, want %#v", got, wantFreshness)
				}
				wantVersions := map[string]int{"v1.9.0": 1, "v2.0.0": 1, "not_reported": 1}
				if got := nonzeroStatusCounts(detail.Summary.AgentVersions); !mapsEqual(got, wantVersions) {
					t.Errorf("mixed agent-version counts = %#v, want %#v", got, wantVersions)
				}
				for _, dimension := range [][]StatusCount{detail.Summary.Compliance, detail.Summary.Freshness, detail.Summary.AgentVersions} {
					if totalStatusCounts(dimension) != len(detail.Members) {
						t.Errorf("Fleet distribution %#v does not equal %d visible members", dimension, len(detail.Members))
					}
				}
				if detail.Empty || detail.EmptyMessage != "" || detail.Sections.Members.State != SectionReady {
					t.Errorf("mixed Fleet empty/section state = %#v, want populated ready detail", detail)
				}
			},
		},
		{
			name:  "explicit empty Fleet",
			fleet: "empty",
			check: func(t *testing.T, detail FleetDetailView) {
				t.Helper()
				if detail.Fleet != "empty" || detail.Summary.Fleet != "empty" || detail.Summary.EndpointCount != 0 || len(detail.Members) != 0 {
					t.Fatalf("empty Fleet detail = %#v, want visible zero-member Fleet", detail)
				}
				if !detail.Empty || !strings.Contains(detail.EmptyMessage, "No Endpoints are enrolled") {
					t.Errorf("empty Fleet explanation = %q, want explicit no-enrollment copy", detail.EmptyMessage)
				}
				if detail.Sections.Members.State != SectionEmpty || detail.Sections.State.State != SectionEmpty {
					t.Errorf("empty Fleet section states = %#v, want explicit empty results", detail.Sections)
				}
				if totalStatusCounts(detail.Summary.Compliance) != 0 || totalStatusCounts(detail.Summary.Freshness) != 0 || totalStatusCounts(detail.Summary.AgentVersions) != 0 {
					t.Errorf("empty Fleet distributions are nonzero: %#v", detail.Summary)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := newFleetDetailProfile(t)
			detail, err := NewFleetDetailService(WithFleetDetailClock(func() time.Time {
				return workspaceStatusNow
			})).Load(t.Context(), profile, test.fleet)
			if err != nil {
				t.Fatalf("load Fleet detail: %v", err)
			}
			test.check(t, detail)
		})
	}
}

func newFleetDetailProfile(t *testing.T) ConnectionProfile {
	t.Helper()
	tlsFixture := newConnectionTLSFixture(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 || request.TLS.PeerCertificates[0].Subject.CommonName != "operator-fleet-detail" {
			http.Error(response, "verified Fleet-detail Operator required", http.StatusUnauthorized)
			return
		}
		switch request.Method + " " + request.URL.Path {
		case "GET /v1/admin/me":
			writeWorkspaceJSON(response, `{"operator_id":"operator-fleet-detail","roles":["read_only"]}`)
		case "GET /v1/admin/fleets":
			writeWorkspaceJSON(response, `["empty","mixed"]`)
		case "GET /v1/admin/endpoints":
			writeWorkspaceJSON(response, `[
				{"id":"endpoint-c","fleet":"mixed"},
				{"id":"other-endpoint","fleet":"other","reported_agent_version":"v9.9.9"},
				{"id":"endpoint-b","fleet":"mixed","reported_agent_version":"v1.9.0","last_check_in":{"release_ref":"release-41","digest":"digest-b","at":"2032-03-04T04:56:06.999999999Z"}},
				{"id":"endpoint-a","fleet":"mixed","reported_agent_version":"v2.0.0","last_check_in":{"release_ref":"release-42","digest":"digest-a","at":"2032-03-04T05:01:07Z"}}
			]`)
		case "GET /v1/admin/fleets/mixed/state-report":
			writeWorkspaceJSON(response, `{"fleet":"mixed","summary":{"total":3,"compliant":1,"drift":1,"no_report":1},"endpoints":[
				{"endpoint_id":"endpoint-a","fleet":"mixed","status":"compliant","reported_at":"2032-03-04T05:02:07Z"},
				{"endpoint_id":"endpoint-b","fleet":"mixed","status":"drifted","reported_at":"2032-03-04T04:57:07Z"}
			]}`)
		case "GET /v1/admin/fleets/empty/state-report":
			writeWorkspaceJSON(response, `{"fleet":"empty","summary":{"total":0},"endpoints":[]}`)
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
		"operator-fleet-detail",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		tlsFixture.caPEM,
	)
	return connectionProfileForServer(t, "Fleet detail", server.URL, stateDir)
}

func nonzeroStatusCounts(counts []StatusCount) map[string]int {
	result := map[string]int{}
	for _, count := range counts {
		if count.Count != 0 {
			result[count.Status] = count.Count
		}
	}
	return result
}

func totalStatusCounts(counts []StatusCount) int {
	total := 0
	for _, count := range counts {
		total += count.Count
	}
	return total
}
