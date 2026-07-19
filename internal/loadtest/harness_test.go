package loadtest

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
)

func TestEndpointIDIsUniqueAndValid(t *testing.T) {
	seen := map[string]bool{}
	for i := range 400 {
		id := EndpointID("run-abc", i)
		if seen[id] {
			t.Fatalf("duplicate endpoint id %q", id)
		}
		if len(id) < 4 || len(id) > 63 {
			t.Fatalf("endpoint id %q has invalid length", id)
		}
		seen[id] = true
	}
}

func TestSummarizeRecordsLatencyAndErrors(t *testing.T) {
	summary := Summarize([]Sample{
		{Latency: 10 * time.Millisecond, ResponseBytes: 100, CapabilityBlocked: true},
		{Latency: 15 * time.Millisecond, ResponseBytes: 50, CapabilityBlocked: true, Unmanaged: true},
		{Latency: 20 * time.Millisecond, ResponseBytes: 200},
		{Latency: 30 * time.Millisecond, Err: &agentsync.HTTPStatusError{StatusCode: http.StatusServiceUnavailable}},
	})
	if summary.Requests != 4 || summary.Successes != 3 || summary.Errors != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.Overloaded != 1 {
		t.Fatalf("overloaded = %d, want 1", summary.Overloaded)
	}
	if summary.P95 != 30*time.Millisecond || summary.ResponseBytes != 350 || summary.CapabilityBlocked != 2 || summary.Unmanaged != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestCapabilityMixedPopulationIsBalancedAt400Endpoints(t *testing.T) {
	counts := map[string]int{}
	for index := range 400 {
		counts[capabilityPopulation(index)]++
	}
	want := map[string]int{
		populationCompatible:      80,
		populationBlockedExisting: 80,
		populationUnmanagedNew:    80,
		populationTelemetry:       80,
		populationReconnecting:    80,
	}
	if len(counts) != len(want) {
		t.Fatalf("population counts = %+v", counts)
	}
	for population, expected := range want {
		if counts[population] != expected {
			t.Errorf("population %q = %d, want %d", population, counts[population], expected)
		}
	}
}

func TestCapabilityMixedRequestsAreBoundedAndRevisionExplicit(t *testing.T) {
	compatible, err := capabilityMixedRequest(populationCompatible, "endpoint-compatible")
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := capabilityMixedRequest(populationBlockedExisting, "endpoint-blocked")
	if err != nil {
		t.Fatal(err)
	}
	telemetry, err := capabilityMixedRequest(populationTelemetry, "endpoint-telemetry")
	if err != nil {
		t.Fatal(err)
	}
	if compatible.CapabilityDocument == nil || blocked.CapabilityDocument == nil || telemetry.CapabilityDocument == nil {
		t.Fatal("mixed requests omitted current capability documents")
	}
	if !hasCapabilityRevision(compatible, "provider:package/apt", "1") || !hasCapabilityRevision(blocked, "provider:package/apt", "0") {
		t.Fatalf("mixed provider revisions: compatible=%+v blocked=%+v", compatible.CapabilityDocument, blocked.CapabilityDocument)
	}
	if telemetry.Drift == nil || telemetry.SystemInfo == nil || telemetry.FirewallAudit == nil {
		t.Fatalf("telemetry population request = %+v", telemetry)
	}
	for population, request := range map[string]agentsync.Request{
		populationCompatible:      compatible,
		populationBlockedExisting: blocked,
		populationTelemetry:       telemetry,
	} {
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 96*1024 {
			t.Errorf("%s request size = %d, want <= 96 KiB", population, len(raw))
		}
	}
}

func TestCapabilityBlockedWaveRetainsActiveClientIdentity(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(response).Encode(agentsync.Response{
			ReleaseRef: "release-target",
			CapabilityBlocked: &agentsync.CapabilityBlocked{
				TargetReleaseRef:    "release-target",
				MissingRequirements: []agentsync.MissingRequirement{{ID: "provider:package/apt", Revision: "1"}},
			},
		})
	}))
	t.Cleanup(server.Close)
	client := agentsync.NewClient(server.URL, &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) //nolint:gosec // test server
	harness := Harness{
		cfg: Config{Concurrency: 1},
		endpoints: []endpoint{{
			id: "endpoint-blocked", client: client, population: populationBlockedExisting,
			lastReleaseRef: "release-active", lastDigest: "digest-active",
		}},
	}

	summary := harness.SyncWave(context.Background(), agentsync.Request{})
	if summary.Errors != 0 || summary.CapabilityBlocked != 1 {
		t.Fatalf("blocked summary = %+v", summary)
	}
	endpoint := harness.endpoints[0]
	if endpoint.lastReleaseRef != "release-active" || endpoint.lastDigest != "digest-active" {
		t.Fatalf("blocked client identity = %q/%q, want active artifact", endpoint.lastReleaseRef, endpoint.lastDigest)
	}
}

func hasCapabilityRevision(request agentsync.Request, id, revision string) bool {
	for _, capability := range request.CapabilityDocument.Capabilities {
		if capability.ID == id && capability.Revision == revision {
			return true
		}
	}
	return false
}

func TestSummarizeReportsStartSpreadAndPeakBucket(t *testing.T) {
	started := time.Unix(100, 0)
	summary := Summarize([]Sample{
		{StartedAt: started},
		{StartedAt: started.Add(50 * time.Millisecond)},
		{StartedAt: started.Add(100 * time.Millisecond)},
	})
	if summary.StartSpread != 100*time.Millisecond {
		t.Fatalf("start spread = %s, want 100ms", summary.StartSpread)
	}
	if summary.MaxStartsPer100ms != 2 {
		t.Fatalf("peak starts = %d, want 2", summary.MaxStartsPer100ms)
	}
}

func TestDatabaseDeltaSubtractsCountersAndKeepsEndingPoolState(t *testing.T) {
	before := DatabaseMetrics{
		PoolTotalConns:   2,
		PoolAcquireCount: 4,
		XactCommit:       10,
		BlocksHit:        100,
	}
	after := DatabaseMetrics{
		PoolTotalConns:   3,
		PoolAcquireCount: 9,
		XactCommit:       17,
		BlocksHit:        140,
	}

	delta := after.Delta(before)
	if delta.PoolTotalConns != 3 || delta.PoolAcquireCount != 5 || delta.XactCommit != 7 || delta.BlocksHit != 40 {
		t.Fatalf("delta = %+v", delta)
	}
}

func TestParseRSSBytes(t *testing.T) {
	rss, err := parseRSSBytes(strings.NewReader("Name:\tremotr-load\nVmRSS:\t   1234 kB\n"))
	if err != nil {
		t.Fatalf("parseRSSBytes: %v", err)
	}
	if rss != 1234*1024 {
		t.Fatalf("rss = %d, want %d", rss, 1234*1024)
	}
}

func TestCloseIdleConnectionsForcesEachEndpointTransportToReconnect(t *testing.T) {
	first := &closeTrackingTransport{}
	second := &closeTrackingTransport{}
	harness := Harness{endpoints: []endpoint{
		{client: &agentsync.Client{HTTPClient: &http.Client{Transport: first}}},
		{client: &agentsync.Client{HTTPClient: &http.Client{Transport: second}}},
	}}

	harness.closeIdleConnections()

	if !first.closed || !second.closed {
		t.Fatalf("transports were not closed: first=%t second=%t", first.closed, second.closed)
	}
}

func TestFanoutArtifactCreatesDistinctContentAndDigest(t *testing.T) {
	original := []byte("version: v1\nresources: []\n")
	artifact, digest := fanoutArtifact(original, "release fan-out")
	wantDigest := sha256.Sum256(artifact)
	if string(artifact) == string(original) {
		t.Fatalf("artifact did not change: %q", artifact)
	}
	if digest != stringDigest(wantDigest) {
		t.Fatalf("digest = %q, want %q", digest, stringDigest(wantDigest))
	}
}

func TestTelemetryHeavyRequestIsBoundedAndContainsPersistedTelemetry(t *testing.T) {
	request := telemetryHeavyRequest("endpoint-a")
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal telemetry request: %v", err)
	}
	if len(body) < 24*1024 || len(body) > 48*1024 {
		t.Fatalf("telemetry request size = %d, want 24-48 KiB", len(body))
	}
	if request.SystemInfo == nil || request.Drift == nil || request.FirewallAudit == nil || len(request.Labels) < 4 || len(request.Usernames) < 2 {
		t.Fatalf("telemetry request missing persisted fields: %+v", request)
	}
}

type closeTrackingTransport struct {
	closed bool
}

func (t *closeTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not used")
}

func (t *closeTrackingTransport) CloseIdleConnections() {
	t.closed = true
}
