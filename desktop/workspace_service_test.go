package main

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const workspaceForbiddenCanary = "workspace-audit-forbidden-response-canary"

func TestWorkspaceServiceLoadsCompleteAndSectionForbiddenResults(t *testing.T) {
	tests := []struct {
		name           string
		auditForbidden bool
	}{
		{name: "complete workspace"},
		{name: "audit forbidden only", auditForbidden: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWorkspaceServerFixture(t, test.auditForbidden)
			workspace, err := NewWorkspaceService().Load(t.Context(), fixture.profile)
			if err != nil {
				t.Fatalf("load workspace: %v", err)
			}

			if workspace.Operator.OperatorID != "operator-workspace" || !slices.Equal(workspace.Operator.Roles, []string{"read_only", "auditor"}) {
				t.Fatalf("Operator view = %#v, want controlled identity and roles", workspace.Operator)
			}
			if got := endpointIDs(workspace.Endpoints); !slices.Equal(got, []string{"endpoint-a", "endpoint-b"}) {
				t.Fatalf("Endpoint IDs = %v, want [endpoint-a endpoint-b]", got)
			}
			if got := fleetNames(workspace.Fleets); !slices.Equal(got, []string{"empty", "production"}) {
				t.Fatalf("Fleet names = %v, want [empty production]", got)
			}
			if len(workspace.ChangeRequests) != 1 || workspace.ChangeRequests[0].ChangeRequestID != "change-1" || workspace.ChangeRequests[0].TargetCount != 2 {
				t.Fatalf("Change request summaries = %#v, want controlled change-1", workspace.ChangeRequests)
			}
			for _, section := range []SectionResult{
				workspace.Sections.Fleets,
				workspace.Sections.Endpoints,
				workspace.Sections.State,
				workspace.Sections.ChangeRequests,
			} {
				if section.State != SectionReady || section.Error != nil || section.Snapshot.LoadedAt == "" {
					t.Errorf("available section = %#v, want ready result with load timestamp", section)
				}
			}

			if test.auditForbidden {
				if len(workspace.Activity) != 0 || workspace.ActivityNextCursor != "" {
					t.Fatalf("forbidden Activity = %#v cursor %q, want no rows/cursor", workspace.Activity, workspace.ActivityNextCursor)
				}
				activitySection := workspace.Sections.Activity
				if activitySection.State != SectionUnavailable || activitySection.Error == nil || activitySection.Error.Kind != ErrorAuthorization {
					t.Fatalf("forbidden Activity section = %#v, want authorization-specific unavailable result", activitySection)
				}
				if !strings.Contains(strings.ToLower(activitySection.Error.Message), "not authorized") {
					t.Fatalf("forbidden Activity guidance = %#v, want authorization explanation", activitySection.Error)
				}
			} else {
				if len(workspace.Activity) != 1 || workspace.Activity[0].EventID != "event-1" || workspace.ActivityNextCursor != "cursor-2" {
					t.Fatalf("Activity result = %#v cursor %q, want controlled event and cursor", workspace.Activity, workspace.ActivityNextCursor)
				}
				if workspace.Sections.Activity.State != SectionReady || workspace.Sections.Activity.Error != nil {
					t.Fatalf("Activity section = %#v, want ready", workspace.Sections.Activity)
				}
			}

			encoded, err := json.Marshal(workspace)
			if err != nil {
				t.Fatalf("encode safe workspace: %v", err)
			}
			for _, forbidden := range []string{
				workspaceForbiddenCanary,
				fixture.stateDir,
				"BEGIN PRIVATE KEY",
				"BEGIN CERTIFICATE",
				connectionBootstrapTokenCanary,
			} {
				if strings.Contains(string(encoded), forbidden) {
					t.Errorf("workspace disclosed forbidden value %q: %s", forbidden, encoded)
				}
			}

			fixture.assertRequestInventory(t)
			if got := fixture.maxConcurrent.Load(); got < 2 || got > 4 {
				t.Fatalf("maximum concurrent top-level section requests = %d, want bounded range [2,4]", got)
			}
		})
	}
}

type workspaceServerFixture struct {
	profile       ConnectionProfile
	stateDir      string
	requestMu     sync.Mutex
	requests      map[string]int
	active        atomic.Int32
	maxConcurrent atomic.Int32
	releaseFanout chan struct{}
	releaseOnce   sync.Once
}

func newWorkspaceServerFixture(t *testing.T, auditForbidden bool) *workspaceServerFixture {
	t.Helper()
	tlsFixture := newConnectionTLSFixture(t)
	fixture := &workspaceServerFixture{
		requests:      map[string]int{},
		releaseFanout: make(chan struct{}),
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		fixture.recordRequest(request.Method + " " + request.URL.Path)
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 || request.TLS.PeerCertificates[0].Subject.CommonName != "operator-workspace" {
			http.Error(response, "verified workspace Operator required", http.StatusUnauthorized)
			return
		}

		if isWorkspaceTopLevelPath(request.URL.Path) {
			active := fixture.active.Add(1)
			defer fixture.active.Add(-1)
			for {
				maximum := fixture.maxConcurrent.Load()
				if active <= maximum || fixture.maxConcurrent.CompareAndSwap(maximum, active) {
					break
				}
			}
			if active >= 2 {
				fixture.releaseOnce.Do(func() { close(fixture.releaseFanout) })
			}
			select {
			case <-fixture.releaseFanout:
			case <-request.Context().Done():
				return
			}
		}

		switch request.Method + " " + request.URL.Path {
		case "GET /v1/admin/me":
			writeWorkspaceJSON(response, `{"operator_id":"operator-workspace","roles":["read_only","auditor"]}`)
		case "GET /v1/admin/fleets":
			writeWorkspaceJSON(response, `["production","empty"]`)
		case "GET /v1/admin/endpoints":
			writeWorkspaceJSON(response, `[
				{"id":"endpoint-b","fleet":"production","labels":{"region":"west"},"desired_agent_version":"v2.0.0","reported_agent_version":"v1.9.0","usernames":["bob"],"last_check_in":{"release_ref":"release-41","digest":"digest-b","at":"2032-03-04T04:56:07Z"}},
				{"id":"endpoint-a","fleet":"production","labels":{"region":"east"},"desired_agent_version":"v2.0.0","reported_agent_version":"v2.0.0","usernames":["alice"],"last_check_in":{"release_ref":"release-42","digest":"digest-a","at":"2032-03-04T05:01:07Z"}}
			]`)
		case "GET /v1/admin/fleets/production/state-report":
			writeWorkspaceJSON(response, `{"fleet":"production","summary":{"total":2,"compliant":1,"drift":1,"unsupported":0,"check_failed":0,"deferred":0,"apply_failed":0,"no_report":0},"endpoints":[
				{"endpoint_id":"endpoint-a","fleet":"production","release_ref":"release-42","digest":"state-a","reported_at":"2032-03-04T05:02:07Z","in_compliance":true,"status":"compliant","items":[]},
				{"endpoint_id":"endpoint-b","fleet":"production","release_ref":"release-41","digest":"state-b","reported_at":"2032-03-04T04:57:07Z","in_compliance":false,"status":"drifted","items":[]}
			]}`)
		case "GET /v1/admin/fleets/empty/state-report":
			writeWorkspaceJSON(response, `{"fleet":"empty","summary":{"total":0,"compliant":0,"drift":0,"unsupported":0,"check_failed":0,"deferred":0,"apply_failed":0,"no_report":0},"endpoints":[]}`)
		case "GET /v1/admin/change-requests":
			writeWorkspaceJSON(response, `[{"id":"change-1","fleet":"production","release_ref":"release-42","risk":"standard","authorization_state":"pending","required_approvals":1,"approvals":[],"frozen_targets":[{"endpoint_id":"endpoint-a"},{"endpoint_id":"endpoint-b"}],"audit_history":[{"at":"2032-03-04T05:03:07Z","actor_id":"operator-workspace","action":"created"}],"created_at":"2032-03-04T05:03:07Z"}]`)
		case "GET /v1/admin/audit-events":
			if auditForbidden {
				http.Error(response, workspaceForbiddenCanary, http.StatusForbidden)
				return
			}
			writeWorkspaceJSON(response, `{"events":[{"id":"event-1","occurred_at":"2032-03-04T05:04:07Z","request_id":"request-1","actor_type":"operator","actor_id":"operator-workspace","actor_fingerprint":"fingerprint-must-not-cross","action":"git_sync","method":"POST","path":"/v1/admin/git-sync","status_code":200,"resource_type":"server","resource_id":"primary","client_ip":"192.0.2.10","details":{"release_ref":"release-42"}}],"next_cursor":"cursor-2"}`)
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

	fixture.stateDir = tlsFixture.saveClientState(
		t,
		"operator-workspace",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		tlsFixture.caPEM,
	)
	fixture.profile = connectionProfileForServer(t, "Workspace", server.URL, fixture.stateDir)
	return fixture
}

func (f *workspaceServerFixture) recordRequest(key string) {
	f.requestMu.Lock()
	defer f.requestMu.Unlock()
	f.requests[key]++
}

func (f *workspaceServerFixture) assertRequestInventory(t *testing.T) {
	t.Helper()
	want := map[string]int{
		"GET /v1/admin/me":                             1,
		"GET /v1/admin/fleets":                         1,
		"GET /v1/admin/endpoints":                      1,
		"GET /v1/admin/fleets/production/state-report": 1,
		"GET /v1/admin/fleets/empty/state-report":      1,
		"GET /v1/admin/change-requests":                1,
		"GET /v1/admin/audit-events":                   1,
	}
	f.requestMu.Lock()
	defer f.requestMu.Unlock()
	if !mapsEqual(f.requests, want) {
		t.Fatalf("request inventory = %#v, want %#v", f.requests, want)
	}
}

func isWorkspaceTopLevelPath(path string) bool {
	return path == "/v1/admin/fleets" || path == "/v1/admin/endpoints" || path == "/v1/admin/change-requests" || path == "/v1/admin/audit-events"
}

func writeWorkspaceJSON(response http.ResponseWriter, body string) {
	response.Header().Set("Content-Type", "application/json")
	_, _ = response.Write([]byte(body))
}

func endpointIDs(endpoints []EndpointRow) []string {
	ids := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		ids = append(ids, endpoint.EndpointID)
	}
	return ids
}

func fleetNames(fleets []FleetSummary) []string {
	names := make([]string, 0, len(fleets))
	for _, fleet := range fleets {
		names = append(names, fleet.Fleet)
	}
	return names
}

func mapsEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
