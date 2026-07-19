package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	endpointDetailScheduleCanary = "endpoint-detail-schedule-response-canary"
	endpointDetailFirewallCanary = "endpoint-detail-firewall-response-canary"
	endpointDetailSystemCanary   = "endpoint-detail-raw-system-canary"
)

func TestEndpointDetailServicePreservesPartialEvidence(t *testing.T) {
	fixture := newEndpointDetailFixture(t, false)
	detail, err := NewEndpointDetailService(WithEndpointDetailClock(func() time.Time {
		return workspaceStatusNow
	})).Load(t.Context(), fixture.profile, "endpoint-a")
	if err != nil {
		t.Fatalf("load Endpoint detail: %v", err)
	}

	header := detail.Header
	if header.EndpointID != "endpoint-a" || header.Fleet != "production" || header.Compliance != ComplianceDrifted || header.Freshness != FreshnessRecent {
		t.Errorf("Endpoint detail header = %#v, want exact selected identity and independent statuses", header)
	}
	if header.ReportedAgentVersion != "v1.9.0" || header.DesiredAgentVersion != "v2.0.0" || header.ReleaseRef != "release-42" {
		t.Errorf("Endpoint detail versions/release = %#v, want controlled evidence", header)
	}
	if !slices.Equal(header.Labels, []LabelView{{Key: "region", Value: "west"}, {Key: "tier", Value: "api"}}) {
		t.Errorf("Endpoint detail Labels = %#v, want deterministic complete Labels", header.Labels)
	}

	for name, section := range map[string]SectionResult{
		"overview": detail.Sections.Overview,
		"State":    detail.Sections.State,
		"system":   detail.Sections.System,
	} {
		if section.State != SectionReady || section.Error != nil {
			t.Errorf("%s section = %#v, want independently ready", name, section)
		}
	}
	if detail.Sections.Schedules.State != SectionUnavailable || detail.Sections.Schedules.Error == nil || detail.Sections.Schedules.Error.Kind != ErrorUnavailable {
		t.Errorf("Schedules section = %#v, want service-unavailable classification", detail.Sections.Schedules)
	}
	if detail.Sections.Firewall.State != SectionUnavailable || detail.Sections.Firewall.Error == nil || detail.Sections.Firewall.Error.Kind != ErrorAuthorization {
		t.Errorf("Firewall section = %#v, want authorization-unavailable classification", detail.Sections.Firewall)
	}
	if len(detail.State.Items) != 1 || detail.State.Items[0].Address != "packages/curl" {
		t.Errorf("State evidence = %#v, want controlled package evidence", detail.State)
	}
	if len(detail.Schedules) != 0 || len(detail.Firewall) != 0 {
		t.Errorf("unavailable evidence crossed section boundary: schedules=%#v firewall=%#v", detail.Schedules, detail.Firewall)
	}
	if detail.System.OS != "Debian GNU/Linux 13" || detail.System.Kernel != "6.12.8" || detail.System.CPU != "Test CPU" || detail.System.CPUCores != "4" || detail.System.Memory != "8 GiB" {
		t.Errorf("safe System evidence = %#v, want controlled bounded summary", detail.System)
	}

	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("encode Endpoint detail: %v", err)
	}
	for _, forbidden := range []string{
		endpointDetailScheduleCanary,
		endpointDetailFirewallCanary,
		endpointDetailSystemCanary,
		fixture.stateDir,
		"BEGIN PRIVATE KEY",
		"BEGIN CERTIFICATE",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("Endpoint detail disclosed forbidden value %q: %s", forbidden, encoded)
		}
	}
	fixture.assertPartialRequestInventory(t)
	if got := fixture.maxConcurrent.Load(); got < 2 || got > 4 {
		t.Errorf("maximum concurrent detail requests = %d, want bounded range [2,4]", got)
	}
}

func TestEndpointDetailServiceCancelsObsoleteSelection(t *testing.T) {
	fixture := newEndpointDetailFixture(t, true)
	service := NewEndpointDetailService(WithEndpointDetailClock(func() time.Time {
		return workspaceStatusNow
	}))
	type loadResult struct {
		detail EndpointDetailView
		err    error
	}
	firstResult := make(chan loadResult, 1)
	go func() {
		detail, err := service.Load(t.Context(), fixture.profile, "endpoint-slow")
		firstResult <- loadResult{detail: detail, err: err}
	}()

	select {
	case <-fixture.slowStarted:
	case <-t.Context().Done():
		t.Fatal("slow Endpoint detail request did not start")
	}

	current, err := service.Load(t.Context(), fixture.profile, "endpoint-fast")
	if err != nil {
		t.Fatalf("load replacement Endpoint detail: %v", err)
	}
	if current.Header.EndpointID != "endpoint-fast" || current.State.EndpointID != "endpoint-fast" || current.System.Hostname != "endpoint-fast-host" {
		t.Fatalf("replacement detail mixed Endpoint identities: %#v", current)
	}

	var obsolete loadResult
	select {
	case obsolete = <-firstResult:
	case <-t.Context().Done():
		t.Fatal("obsolete Endpoint detail load did not finish")
	}
	if !errors.Is(obsolete.err, ErrObsoleteEndpointDetail) {
		t.Errorf("obsolete selection error = %v, want %v", obsolete.err, ErrObsoleteEndpointDetail)
	}
	if obsolete.detail.Header.EndpointID == "endpoint-slow" {
		t.Errorf("obsolete Endpoint detail was returned as renderable data: %#v", obsolete.detail)
	}
	select {
	case <-fixture.slowCanceled:
	case <-t.Context().Done():
		t.Fatal("obsolete Endpoint network requests were not canceled")
	}
}

type endpointDetailFixture struct {
	profile        ConnectionProfile
	stateDir       string
	requestMu      sync.Mutex
	requests       map[string]int
	active         atomic.Int32
	maxConcurrent  atomic.Int32
	releaseFanout  chan struct{}
	releaseOnce    sync.Once
	slowStarted    chan struct{}
	slowStartOnce  sync.Once
	slowCanceled   chan struct{}
	slowCancelOnce sync.Once
}

func newEndpointDetailFixture(t *testing.T, cancellation bool) *endpointDetailFixture {
	t.Helper()
	tlsFixture := newConnectionTLSFixture(t)
	fixture := &endpointDetailFixture{
		requests:      map[string]int{},
		releaseFanout: make(chan struct{}),
		slowStarted:   make(chan struct{}),
		slowCanceled:  make(chan struct{}),
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		key := request.Method + " " + request.URL.Path
		fixture.recordRequest(key)
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 || request.TLS.PeerCertificates[0].Subject.CommonName != "operator-detail" {
			http.Error(response, "verified detail Operator required", http.StatusUnauthorized)
			return
		}

		if strings.Contains(request.URL.Path, "/endpoint-slow") {
			fixture.slowStartOnce.Do(func() { close(fixture.slowStarted) })
			<-request.Context().Done()
			fixture.slowCancelOnce.Do(func() { close(fixture.slowCanceled) })
			return
		}
		if !cancellation && strings.Contains(request.URL.Path, "/endpoint-a") {
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

		switch key {
		case "GET /v1/admin/me":
			writeWorkspaceJSON(response, `{"operator_id":"operator-detail","roles":["read_only"]}`)
		case "GET /v1/admin/endpoints/endpoint-a":
			writeWorkspaceJSON(response, `{"id":"endpoint-a","fleet":"production","labels":{"tier":"api","region":"west"},"desired_agent_version":"v2.0.0","reported_agent_version":"v1.9.0","last_check_in":{"release_ref":"release-42","digest":"digest-a","at":"2032-03-04T05:01:07Z"},"system_info":{"digest":"system-a","reported_at":"2032-03-04T05:02:07Z","report":{"hostname":"endpoint-a-host","osRelease":{"prettyName":"Debian GNU/Linux 13"},"cpu":{"modelName":"Test CPU","coreCount":"4"},"ram":{"memTotal":"8 GiB"},"kernel":{"version":"6.12.8"},"rawSecret":"endpoint-detail-raw-system-canary"}}}`)
		case "GET /v1/admin/endpoints/endpoint-a/state-report":
			writeWorkspaceJSON(response, `{"endpoint_id":"endpoint-a","fleet":"production","release_ref":"release-42","digest":"state-a","reported_at":"2032-03-04T05:03:07Z","status":"drifted","items":[{"address":"packages/curl","name":"curl","provider":"packages","status":"drifted","reasonCode":"version_mismatch","desiredSummary":{"fields":[{"path":"version","sensitivity":"public","projection":"value","text":"v2"}]},"observedSummary":{"fields":[{"path":"version","sensitivity":"public","projection":"value","text":"v1"}]}}]}`)
		case "GET /v1/admin/endpoints/endpoint-a/cron-report":
			http.Error(response, endpointDetailScheduleCanary, http.StatusServiceUnavailable)
		case "GET /v1/admin/endpoints/endpoint-a/firewall-audit":
			http.Error(response, endpointDetailFirewallCanary, http.StatusForbidden)
		case "GET /v1/admin/endpoints/endpoint-fast":
			writeWorkspaceJSON(response, `{"id":"endpoint-fast","fleet":"production","last_check_in":{"release_ref":"release-fast","digest":"digest-fast","at":"2032-03-04T05:05:07Z"},"system_info":{"reported_at":"2032-03-04T05:05:07Z","report":{"hostname":"endpoint-fast-host"}}}`)
		case "GET /v1/admin/endpoints/endpoint-fast/state-report":
			writeWorkspaceJSON(response, `{"endpoint_id":"endpoint-fast","fleet":"production","release_ref":"release-fast","reported_at":"2032-03-04T05:05:07Z","status":"compliant","items":[]}`)
		case "GET /v1/admin/endpoints/endpoint-fast/cron-report":
			writeWorkspaceJSON(response, `{"endpoint_id":"endpoint-fast","fleet":"production","jobs":[]}`)
		case "GET /v1/admin/endpoints/endpoint-fast/firewall-audit":
			writeWorkspaceJSON(response, `{"endpoint_id":"endpoint-fast","reported_at":"2032-03-04T05:05:07Z","report":[]}`)
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
		"operator-detail",
		connectionTestTime.Add(-time.Hour),
		connectionTestTime.Add(time.Hour),
		tlsFixture.caPEM,
	)
	fixture.profile = connectionProfileForServer(t, "Endpoint detail", server.URL, fixture.stateDir)
	return fixture
}

func (f *endpointDetailFixture) recordRequest(key string) {
	f.requestMu.Lock()
	defer f.requestMu.Unlock()
	f.requests[key]++
}

func (f *endpointDetailFixture) assertPartialRequestInventory(t *testing.T) {
	t.Helper()
	want := map[string]int{
		"GET /v1/admin/me":                                  1,
		"GET /v1/admin/endpoints/endpoint-a":                1,
		"GET /v1/admin/endpoints/endpoint-a/state-report":   1,
		"GET /v1/admin/endpoints/endpoint-a/cron-report":    1,
		"GET /v1/admin/endpoints/endpoint-a/firewall-audit": 1,
	}
	f.requestMu.Lock()
	defer f.requestMu.Unlock()
	if !mapsEqual(f.requests, want) {
		t.Fatalf("Endpoint detail request inventory = %#v, want %#v", f.requests, want)
	}
}
