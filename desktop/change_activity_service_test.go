package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	activitySecretCanary    = "activity-detail-secret-canary"
	activityForbiddenCanary = "activity-forbidden-response-canary"
)

func TestChangeRequestServicePreservesReadOnlyServerEvidence(t *testing.T) {
	fixture := newChangeActivityFixture(t, false)
	service := NewChangeRequestService()

	summaries, err := service.List(t.Context(), fixture.profile)
	if err != nil {
		t.Fatalf("list Change requests: %v", err)
	}
	if got := changeSummaryIDs(summaries); !slices.Equal(got, []string{"change-active", "change-pending"}) {
		t.Fatalf("Change request IDs = %v, want stable exact IDs", got)
	}
	if summaries[0].Risk != "destructive" || summaries[0].Lifecycle != "authorized" || summaries[0].TargetCount != 1 || summaries[0].UpdatedAt != "2032-03-04T05:05:07Z" {
		t.Errorf("active Change summary = %#v, want exact server risk/lifecycle/target/update", summaries[0])
	}
	if summaries[1].Risk != "connectivity" || summaries[1].Lifecycle != "pending" || summaries[1].TargetCount != 2 || summaries[1].UpdatedAt != "2032-03-04T05:03:07Z" {
		t.Errorf("pending Change summary = %#v, want exact server risk/lifecycle/target/update", summaries[1])
	}

	detail, err := service.LoadDetail(t.Context(), fixture.profile, "change-active")
	if err != nil {
		t.Fatalf("load Change request detail: %v", err)
	}
	if !detail.ReadOnly || detail.Summary.ChangeRequestID != "change-active" || detail.Summary.Lifecycle != "authorized" {
		t.Fatalf("Change detail identity/read-only state = %#v", detail)
	}
	if len(detail.Resources) != 1 || detail.Resources[0].Address != "base/firewall" || detail.Resources[0].Risk != "destructive" || detail.Resources[0].Provider != "nftables" {
		t.Errorf("Change resource plan = %#v, want bounded server evidence", detail.Resources)
	}
	if len(detail.Targets) != 1 || detail.Targets[0].EndpointID != "endpoint-a" || !detail.Targets[0].Compatible || !detail.Targets[0].PreflightReady {
		t.Errorf("Change targets = %#v, want exact frozen evidence", detail.Targets)
	}
	if len(detail.Approvals) != 1 || detail.Approvals[0].OperatorID != "operator-approver" || detail.Approvals[0].ApprovedAt != "2032-03-04T05:04:07Z" {
		t.Errorf("Change approvals = %#v, want exact approval evidence", detail.Approvals)
	}
	if len(detail.Outcomes) != 1 || detail.Outcomes[0].EndpointID != "endpoint-a" || detail.Outcomes[0].State != "verified_successful" {
		t.Errorf("Change outcomes = %#v, want exact outcome evidence", detail.Outcomes)
	}
	if len(detail.History) != 2 || detail.History[0].Action != "created" || detail.History[1].Action != "rollout_authorized" {
		t.Errorf("Change history = %#v, want server order", detail.History)
	}
	if detail.PolicyWarning != "destructive review required" {
		t.Errorf("Change policy warning = %q, want exact server warning", detail.PolicyWarning)
	}

}

func TestActivityServiceUsesCursorFiltersOrderAndSafeDetails(t *testing.T) {
	fixture := newChangeActivityFixture(t, false)
	request := ActivityPageRequest{
		Since:        "2032-03-04T04:00:00Z",
		Until:        "2032-03-04T06:00:00Z",
		Action:       "git_sync",
		ActorType:    "operator",
		Cursor:       "cursor-1",
		SeenEventIDs: []string{"event-duplicate"},
	}
	page, err := NewActivityService().LoadPage(t.Context(), fixture.profile, request)
	if err != nil {
		t.Fatalf("load Activity page: %v", err)
	}
	if got := activityEventIDs(page.Events); !slices.Equal(got, []string{"event-2", "event-3"}) {
		t.Fatalf("deduplicated Activity IDs = %v, want server order [event-2 event-3]", got)
	}
	if page.NextCursor != "cursor-2" || page.Section.State != SectionReady || page.Section.Error != nil {
		t.Errorf("Activity page state/cursor = %#v, want ready cursor-2", page)
	}
	if len(page.Events[0].Details) != 3 {
		t.Fatalf("safe Activity details = %#v, want three classified fields", page.Events[0].Details)
	}
	if page.Events[1].Details == nil || len(page.Events[1].Details) != 0 {
		t.Fatalf("detail-less Activity event details = %#v, want an empty collection", page.Events[1].Details)
	}
	details := activityDetailsByKey(page.Events[0].Details)
	if details["release_ref"] != "release-42" || details["note"] != "<script>window.evil=true</script>" || details["token"] != "true" {
		t.Errorf("safe Activity details = %#v, want literal formatted values", details)
	}
	encoded := strings.Join([]string{details["release_ref"], details["note"]}, " ")
	if strings.Contains(encoded, activitySecretCanary) {
		t.Errorf("Activity details disclosed secret canary: %s", encoded)
	}
	fixture.assertActivityQuery(t, url.Values{
		"since":      []string{"2032-03-04T04:00:00Z"},
		"until":      []string{"2032-03-04T06:00:00Z"},
		"action":     []string{"git_sync"},
		"actor_type": []string{"operator"},
		"cursor":     []string{"cursor-1"},
		"limit":      []string{"50"},
	})
}

func TestActivityAuthorizationFailureRemainsLocal(t *testing.T) {
	fixture := newChangeActivityFixture(t, true)
	changes, err := NewChangeRequestService().List(t.Context(), fixture.profile)
	if err != nil || len(changes) != 2 {
		t.Fatalf("permitted Change requests were not usable: changes=%#v err=%v", changes, err)
	}
	page, err := NewActivityService().LoadPage(t.Context(), fixture.profile, ActivityPageRequest{})
	if err != nil {
		t.Fatalf("forbidden Activity should return a local section result: %v", err)
	}
	if len(page.Events) != 0 || page.NextCursor != "" || page.Section.State != SectionUnavailable || page.Section.Error == nil || page.Section.Error.Kind != ErrorAuthorization {
		t.Fatalf("forbidden Activity page = %#v, want authorization-specific empty state", page)
	}
	if strings.Contains(page.Section.Error.Message+page.Section.Error.Guidance, activityForbiddenCanary) {
		t.Errorf("forbidden Activity error disclosed response canary: %#v", page.Section.Error)
	}
}

type changeActivityFixture struct {
	profile       ConnectionProfile
	forbidAudit   bool
	queryMu       sync.Mutex
	activityQuery url.Values
}

func newChangeActivityFixture(t *testing.T, forbidAudit bool) *changeActivityFixture {
	t.Helper()
	tlsFixture := newConnectionTLSFixture(t)
	fixture := &changeActivityFixture{forbidAudit: forbidAudit}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 || request.TLS.PeerCertificates[0].Subject.CommonName != "operator-change-activity" {
			http.Error(response, "verified Change/Activity Operator required", http.StatusUnauthorized)
			return
		}
		switch request.Method + " " + request.URL.Path {
		case "GET /v1/admin/me":
			writeWorkspaceJSON(response, `{"operator_id":"operator-change-activity","roles":["read_only","auditor"]}`)
		case "GET /v1/admin/change-requests":
			writeWorkspaceJSON(response, changeRequestListFixtureJSON)
		case "GET /v1/admin/change-requests/change-active":
			writeWorkspaceJSON(response, changeRequestDetailFixtureJSON)
		case "GET /v1/admin/audit-events":
			fixture.queryMu.Lock()
			fixture.activityQuery = request.URL.Query()
			fixture.queryMu.Unlock()
			if fixture.forbidAudit {
				http.Error(response, activityForbiddenCanary, http.StatusForbidden)
				return
			}
			writeWorkspaceJSON(response, `{"events":[
				{"id":"event-2","occurred_at":"2032-03-04T05:06:07Z","request_id":"request-2","actor_type":"operator","actor_id":"operator-change-activity","actor_fingerprint":"fingerprint-must-not-cross","action":"git_sync","method":"POST","path":"/v1/admin/git-sync","status_code":200,"resource_type":"server","resource_id":"primary","client_ip":"192.0.2.20","details":{"fields":[{"path":"release_ref","sensitivity":"public","projection":"value","text":"release-42"},{"path":"note","sensitivity":"public","projection":"value","text":"<script>window.evil=true</script>"},{"path":"token","sensitivity":"secret","projection":"presence","present":true}]}},
				{"id":"event-duplicate","occurred_at":"2032-03-04T05:05:07Z","actor_type":"operator","action":"git_sync","status_code":200},
				{"id":"event-2","occurred_at":"2032-03-04T05:04:07Z","actor_type":"operator","action":"git_sync","status_code":200},
				{"id":"event-3","occurred_at":"2032-03-04T05:03:07Z","actor_type":"operator","action":"git_sync","status_code":403}
			],"next_cursor":"cursor-2"}`)
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
		"operator-change-activity",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		tlsFixture.caPEM,
	)
	fixture.profile = connectionProfileForServer(t, "Change Activity", server.URL, stateDir)
	return fixture
}

func (f *changeActivityFixture) assertActivityQuery(t *testing.T, want url.Values) {
	t.Helper()
	f.queryMu.Lock()
	defer f.queryMu.Unlock()
	if !reflect.DeepEqual(f.activityQuery, want) {
		t.Fatalf("Activity query = %#v, want %#v", f.activityQuery, want)
	}
}

func changeSummaryIDs(summaries []ChangeRequestSummary) []string {
	ids := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ChangeRequestID)
	}
	return ids
}

func activityEventIDs(events []ActivityEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventID)
	}
	return ids
}

func activityDetailsByKey(details []ActivityDetail) map[string]string {
	result := make(map[string]string, len(details))
	for _, detail := range details {
		result[detail.Key] = detail.Value
	}
	return result
}

const changeRequestListFixtureJSON = `[
	{"id":"change-pending","fleet":"production","release_ref":"release-42","risk":"connectivity","authorization_state":"pending","required_approvals":2,"frozen_targets":[{"endpoint_id":"endpoint-a"},{"endpoint_id":"endpoint-b"}],"audit_history":[{"at":"2032-03-04T05:03:07Z","actor_id":"operator-a","action":"created"}],"created_at":"2032-03-04T05:02:07Z"},
	{"id":"change-active","fleet":"production","release_ref":"release-41","risk":"destructive","authorization_state":"authorized","required_approvals":1,"approvals":[{"operator_id":"operator-approver","approved_at":"2032-03-04T05:04:07Z"}],"frozen_targets":[{"endpoint_id":"endpoint-a"}],"audit_history":[{"at":"2032-03-04T05:01:07Z","actor_id":"operator-a","action":"created"},{"at":"2032-03-04T05:05:07Z","actor_id":"operator-approver","action":"rollout_authorized"}],"created_at":"2032-03-04T05:01:07Z"}
]`

const changeRequestDetailFixtureJSON = `{
	"id":"change-active","fleet":"production","release_ref":"release-41","artifact_digest":"artifact-41","authorization_group":"network-transition","risk":"destructive","authorization_state":"authorized","required_approvals":1,"policy_warning":"destructive review required","created_at":"2032-03-04T05:01:07Z",
	"resources":[{"address":"base/firewall","desired_hash":"hash-1","risk":"destructive","provider":"nftables","authorization_group":"network-transition","depends_on":[],"activation_targets":["firewalld"],"predicted_effects":[{"code":"resource_update","details":{"fields":[{"path":"content","sensitivity":"secret","projection":"presence","present":true}]}}],"rollback_class":"automatic","baseline_eligible":true}],
	"resource_hashes":{"base/firewall":"hash-1"},
	"frozen_targets":[{"endpoint_id":"endpoint-a","compatible":true,"preflight_ready":true}],
	"approvals":[{"operator_id":"operator-approver","approved_at":"2032-03-04T05:04:07Z","justification":"approved"}],
	"outcomes":{"endpoint-a":{"endpoint_id":"endpoint-a","state":"verified_successful","reason":"observed compliant"}},
	"audit_history":[{"at":"2032-03-04T05:01:07Z","actor_id":"operator-a","action":"created"},{"at":"2032-03-04T05:05:07Z","actor_id":"operator-approver","action":"rollout_authorized"}]
}`
