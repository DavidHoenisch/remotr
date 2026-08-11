package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/audit"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type instrumentedSyncRegistry struct {
	*registry.Memory
	operations int
}

func (r *instrumentedSyncRegistry) EndpointByID(id string) (registry.Endpoint, bool) {
	r.operations++
	return r.Memory.EndpointByID(id)
}
func (r *instrumentedSyncRegistry) StoreEndpointCapabilityDocument(ctx context.Context, record registry.CapabilityDocumentRecord) (bool, error) {
	r.operations++
	return r.Memory.StoreEndpointCapabilityDocument(ctx, record)
}
func (r *instrumentedSyncRegistry) GetEndpointCapabilityDocument(ctx context.Context, endpointID string) (registry.CapabilityDocumentRecord, bool, error) {
	r.operations++
	return r.Memory.GetEndpointCapabilityDocument(ctx, endpointID)
}
func (r *instrumentedSyncRegistry) StoreEndpointDeliveryState(ctx context.Context, state registry.EndpointDeliveryState) (bool, error) {
	r.operations++
	return r.Memory.StoreEndpointDeliveryState(ctx, state)
}
func (r *instrumentedSyncRegistry) GetEndpointDeliveryState(ctx context.Context, endpointID string) (registry.EndpointDeliveryState, bool, error) {
	r.operations++
	return r.Memory.GetEndpointDeliveryState(ctx, endpointID)
}

// OS-USF-001: after one quiet full Sync proves the decision, the same
// authenticated hash-only request performs no persistence operation.
func TestEligibleUnchangedSyncPerformsZeroStorageOperations(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := &instrumentedSyncRegistry{Memory: registry.NewMemory()}
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := document.CanonicalBody()
	documentDigest, _ := documenthash.Digest(documenthash.Capability, canonical)
	hashes := documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: documentDigest}}
	identityURI, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	now := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	telemetry := &mockTelemetry{}
	auditLog := &mockAuditLog{}
	srv := New(Config{
		ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg, Now: func() time.Time { return now },
		Telemetry: telemetry, AuditLog: auditLog,
		FastPath: FastPathConfig{Enabled: true, MaxEntries: 16, TTL: 10 * time.Minute, CheckpointInterval: 5 * time.Minute},
	})
	send := func(body map[string]any) agentsync.Response {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: []byte("endpoint-cert"), URIs: []*url.URL{identityURI}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var response agentsync.Response
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	first := send(map[string]any{"agentVersion": document.AgentVersion, "capabilityDocument": document, "documentHashes": hashes})
	prime := send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	if !prime.Unchanged {
		t.Fatalf("durably matched hash-only prime response = %+v", prime)
	}
	if prime.SecretAuthorityToken == "" {
		t.Fatal("quiet prime response omitted secret authority token")
	}
	operationsBeforeHit := reg.operations
	auditsBeforeHit := len(auditLog.events)
	checkInsBeforeHit := telemetry.checkInCalls
	hit := send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	if !hit.Unchanged || len(hit.ArtifactYAML) != 0 {
		t.Fatalf("hit response = %+v", hit)
	}
	if hit.SecretAuthorityToken != prime.SecretAuthorityToken {
		t.Fatalf("cached authority token = %q, want %q",
			hit.SecretAuthorityToken, prime.SecretAuthorityToken)
	}
	if reg.operations != operationsBeforeHit {
		t.Fatalf("eligible hit storage operations = %d, want 0", reg.operations-operationsBeforeHit)
	}
	if len(auditLog.events) != auditsBeforeHit {
		t.Fatal("steady-window cache hit persisted an audit event")
	}
	if telemetry.checkInCalls != checkInsBeforeHit {
		t.Fatal("steady-window cache hit persisted liveness")
	}
	now = now.Add(10 * time.Minute)
	operationsBeforeDeadline := reg.operations
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	if reg.operations == operationsBeforeDeadline {
		t.Fatal("first request at validUntil incorrectly used the cache")
	}
	if telemetry.checkInRelease != first.ReleaseRef || telemetry.checkInDigest != first.Digest {
		t.Fatalf("checkpoint check-in = %q/%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
	if telemetry.checkInCalls != checkInsBeforeHit+1 {
		t.Fatalf("checkpoint check-in calls = %d, want %d", telemetry.checkInCalls, checkInsBeforeHit+1)
	}
	if len(auditLog.events) != auditsBeforeHit+1 || auditLog.events[len(auditLog.events)-1].Action != audit.ActionAgentSyncCheckpoint {
		t.Fatalf("checkpoint audits = %+v", auditLog.events[auditsBeforeHit:])
	}
	if details := auditLog.events[len(auditLog.events)-1].Details; details == nil || !bytes.Contains([]byte(details.String()), []byte("observations=2")) {
		t.Fatalf("checkpoint details = %v", details)
	}

	// Hold an endpoint mutation unstable while an authenticated request runs its
	// full path. That request must neither hit nor refill with stale authority.
	completeMutation := srv.fastPath.beginMutation(cacheScopeEndpoint, endpointID)
	operationsBeforeMutationSync := reg.operations
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	if reg.operations == operationsBeforeMutationSync {
		t.Fatal("request linearized after mutation begin used the cache")
	}
	completeMutation()
	operationsBeforeRecovery := reg.operations
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	if reg.operations == operationsBeforeRecovery {
		t.Fatal("request served a fill created while the mutation was unstable")
	}
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"capabilityDocument": document, "documentHashes": hashes,
	})
	operationsBeforeRecoveredHit := reg.operations
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	if reg.operations != operationsBeforeRecoveredHit {
		t.Fatalf("post-mutation recovery hit storage operations = %d, want 0", reg.operations-operationsBeforeRecoveredHit)
	}

	// A failed due checkpoint remains pending and cannot refill the cache.
	now = now.Add(5 * time.Minute)
	telemetry.checkInErr = errors.New("injected checkpoint failure")
	failureBody, _ := json.Marshal(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	failureReq := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(failureBody))
	failureReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: []byte("endpoint-cert"), URIs: []*url.URL{identityURI}}}}
	failureRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(failureRec, failureReq)
	if failureRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("checkpoint failure status = %d body = %s", failureRec.Code, failureRec.Body.String())
	}
	if _, pending := srv.fastPath.pendingCheckpoint(endpointID); !pending {
		t.Fatal("failed checkpoint was not retained for retry")
	}
	total, _, _ := srv.fastPath.bounds()
	if live := len(srv.fastPath.entries); live != 0 || total != 1 {
		t.Fatalf("failed checkpoint bounds = live %d total %d, want 0 live and 1 pending", live, total)
	}
	telemetry.checkInErr = nil
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"documentHashes": hashes,
	})
	_ = send(map[string]any{
		"agentVersion": document.AgentVersion, "lastReleaseRef": first.ReleaseRef, "lastDigest": first.Digest,
		"capabilityDocument": document, "documentHashes": hashes,
	})

	restarted := New(Config{
		ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg, Now: func() time.Time { return now },
		Telemetry: telemetry, AuditLog: auditLog,
		FastPath: FastPathConfig{Enabled: true, MaxEntries: 16, TTL: 10 * time.Minute, CheckpointInterval: 5 * time.Minute},
	})
	operationsBeforeRestart := reg.operations
	restartReq := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(failureBody))
	restartReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{Raw: []byte("endpoint-cert"), URIs: []*url.URL{identityURI}}}}
	restartRec := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(restartRec, restartReq)
	if restartRec.Code != http.StatusOK || reg.operations == operationsBeforeRestart {
		t.Fatalf("cold restart status=%d storage delta=%d", restartRec.Code, reg.operations-operationsBeforeRestart)
	}
}

func TestFastPathValidUntilUsesInjectedClockWithoutSleeping(t *testing.T) {
	now := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	if got, want := deriveValidUntil(now, 7*time.Minute), now.Add(7*time.Minute); !got.Equal(want) {
		t.Fatalf("validUntil = %s, want %s", got, want)
	}
}

func TestFastPathEligibilityFailsClosed(t *testing.T) {
	hash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	base := syncRequest{
		LastReleaseRef: "release", LastDigest: "digest",
		documentHashes: &documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: hash}},
	}
	tests := []struct {
		name   string
		mutate func(*syncRequest)
	}{
		{name: "full capability", mutate: func(r *syncRequest) { r.CapabilityDocument = json.RawMessage(`{}`) }},
		{name: "full system information", mutate: func(r *syncRequest) { r.SystemInfo = &systemInfoPayload{Report: json.RawMessage(`{}`)} }},
		{name: "labels", mutate: func(r *syncRequest) { r.Labels = map[string]string{"site": "west"} }},
		{name: "usernames", mutate: func(r *syncRequest) { r.Usernames = []string{"alice"} }},
		{name: "drift", mutate: func(r *syncRequest) { r.Drift = &driftReportPayload{} }},
		{name: "apply failure", mutate: func(r *syncRequest) { r.ApplyFailure = &applyFailurePayload{} }},
		{name: "cron result", mutate: func(r *syncRequest) { r.CronResults = []cronResultPayload{{RunID: "run"}} }},
		{name: "diagnostic result", mutate: func(r *syncRequest) { r.DiagnosticResult = &diagnosticResultPayload{} }},
		{name: "firewall audit", mutate: func(r *syncRequest) { r.FirewallAudit = &firewallAuditPayload{} }},
		{name: "change preflight", mutate: func(r *syncRequest) {
			r.ChangePreflights = []changecontrol.PreflightReport{{ChangeRequestID: "change"}}
		}},
		{name: "reboot intent", mutate: func(r *syncRequest) { r.RebootIntent = &agentsync.RebootIntentPayload{} }},
		{name: "network intent", mutate: func(r *syncRequest) { r.NetworkIntent = &agentsync.NetworkIntentPayload{} }},
		{name: "upgrade status", mutate: func(r *syncRequest) { r.AgentUpgradeStatus = &agentUpgradeStatusPayload{} }},
		{name: "missing hashes", mutate: func(r *syncRequest) { r.documentHashes = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			if eligibleHashOnlyRequest(request) {
				t.Fatal("request unexpectedly eligible")
			}
		})
	}

	cache := newUnchangedSyncCache(FastPathConfig{Enabled: true, MaxEntries: 2, TTL: time.Minute})
	response := syncResponse{
		Unchanged: true, ReleaseRef: "release", Digest: "digest",
		AcceptedDocumentHashes: &documenthash.Summary{Version: 1, Documents: cloneHashes(base.documentHashes.Documents)},
	}
	cache.put("endpoint", "fingerprint", base, response, time.Unix(0, 0))
	changed := base
	changed.LastDigest = "changed"
	if _, hit := cache.get("endpoint", "fingerprint", changed, time.Unix(1, 0)); hit {
		t.Fatal("changed delivery identity hit cache")
	}
	if _, hit := cache.get("endpoint", "other-fingerprint", base, time.Unix(1, 0)); hit {
		t.Fatal("unknown certificate authority hit cache")
	}

	oneShotResponses := []syncResponse{
		{Unchanged: true, AgentUpgrade: &agentUpgradePayload{}},
		{Unchanged: true, DueCrons: []dueCronPayload{{RunID: "run"}}},
		{Unchanged: true, DiagnosticCollection: &diagnosticCollectionPayload{}},
		{Unchanged: true, ExecutionLeases: []changecontrol.ExecutionLease{{ID: "lease"}}},
		{Unchanged: true, RebootAcknowledged: "reboot"},
		{Unchanged: true, NetworkAcknowledged: "network"},
		{Unchanged: true, RequestedDocuments: []string{documenthash.Capability}},
	}
	for index, candidate := range oneShotResponses {
		if quietCacheableResponse(candidate) {
			t.Fatalf("one-shot response %d unexpectedly cacheable: %+v", index, candidate)
		}
	}
}

// OS-USF-009: resource limits are deterministic and bound the entire
// process-local decision cache, including accumulated observations.
func TestFastPathResourceBoundsDeterministic(t *testing.T) {
	now := time.Date(2026, 8, 7, 2, 0, 0, 0, time.UTC)
	hash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := syncRequest{
		LastReleaseRef: "release", LastDigest: "digest",
		documentHashes: &documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: hash}},
	}
	response := syncResponse{
		Unchanged: true, ReleaseRef: "release", Digest: "digest",
		AcceptedDocumentHashes: &documenthash.Summary{Version: 1, Documents: cloneHashes(request.documentHashes.Documents)},
	}

	t.Run("entry count uses deterministic LRU", func(t *testing.T) {
		cache := newUnchangedSyncCache(FastPathConfig{Enabled: true, MaxEntries: 2, TTL: time.Minute})
		cache.put("a", "fingerprint", request, response, now)
		cache.put("b", "fingerprint", request, response, now)
		if _, hit := cache.get("a", "fingerprint", request, now); !hit {
			t.Fatal("expected a to be present before eviction")
		}
		cache.put("c", "fingerprint", request, response, now)
		if _, hit := cache.get("b", "fingerprint", request, now); hit {
			t.Fatal("least recently used entry b was retained")
		}
		if _, hit := cache.get("a", "fingerprint", request, now); !hit {
			t.Fatal("recently used entry a was evicted")
		}
	})

	t.Run("bytes and lifetime are bounded", func(t *testing.T) {
		probe := newUnchangedSyncCache(FastPathConfig{Enabled: true, MaxEntries: 2, TTL: time.Minute})
		probe.put("a", "fingerprint", request, response, now)
		_, oneEntryBytes, _ := probe.bounds()
		cache := newUnchangedSyncCache(FastPathConfig{
			Enabled: true, MaxEntries: 10, MaxBytes: oneEntryBytes, TTL: time.Minute,
		})
		cache.put("a", "fingerprint", request, response, now)
		cache.put("b", "fingerprint", request, response, now)
		entries, usedBytes, _ := cache.bounds()
		if entries > 1 || usedBytes > oneEntryBytes {
			t.Fatalf("bounds after byte eviction = entries %d bytes %d, want <= 1 and <= %d", entries, usedBytes, oneEntryBytes)
		}
		if _, hit := cache.get("b", "fingerprint", request, now.Add(time.Minute)); hit {
			t.Fatal("entry remained usable at its exact TTL boundary")
		}
	})

	t.Run("observations and soak growth are globally bounded", func(t *testing.T) {
		cache := newUnchangedSyncCache(FastPathConfig{
			Enabled: true, MaxEntries: 8, MaxBytes: 1 << 20, MaxObservations: 3, TTL: time.Hour,
		})
		for i := 0; i < 1_000; i++ {
			id := string(rune('a' + i%20))
			cache.put(id, "fingerprint", request, response, now)
			_, _ = cache.get(id, "fingerprint", request, now)
		}
		entries, usedBytes, observations := cache.bounds()
		if entries > 8 || usedBytes > 1<<20 || observations > 3 {
			t.Fatalf("soak bounds = entries %d bytes %d observations %d", entries, usedBytes, observations)
		}
	})

	t.Run("pending checkpoints share the entry and byte budget", func(t *testing.T) {
		cache := newUnchangedSyncCache(FastPathConfig{
			Enabled: true, MaxEntries: 2, MaxBytes: 1 << 20, TTL: time.Hour, CheckpointInterval: time.Second,
		})
		cache.put("a", "fingerprint", request, response, now)
		cache.put("b", "fingerprint", request, response, now)
		if _, hit, checkpoint := cache.getWithCheckpoint("a", "fingerprint", request, now.Add(time.Second)); hit || checkpoint == nil {
			t.Fatal("expected a to become a pending checkpoint")
		}
		if _, hit, checkpoint := cache.getWithCheckpoint("b", "fingerprint", request, now.Add(time.Second)); hit || checkpoint == nil {
			t.Fatal("expected b to become a pending checkpoint")
		}
		entries, usedBytes, _ := cache.bounds()
		if entries != 2 || usedBytes == 0 {
			t.Fatalf("pending checkpoint bounds = entries %d bytes %d, want 2 and non-zero", entries, usedBytes)
		}
		cache.put("c", "fingerprint", request, response, now.Add(time.Second))
		entries, usedBytes, _ = cache.bounds()
		if entries > 2 || usedBytes > 1<<20 {
			t.Fatalf("bounds with pending checkpoints = entries %d bytes %d", entries, usedBytes)
		}
	})
}

func TestFastPathTopologyFailsClosedAndReportsStatus(t *testing.T) {
	tests := []struct {
		name        string
		config      FastPathConfig
		wantEnabled bool
		wantReason  string
	}{
		{
			name: "single serving process", config: FastPathConfig{Enabled: true, ServingProcesses: 1},
			wantEnabled: true, wantReason: "enabled",
		},
		{
			name: "uncoordinated serving processes", config: FastPathConfig{Enabled: true, ServingProcesses: 2},
			wantEnabled: false, wantReason: "multiple_serving_processes_without_coordinator",
		},
		{
			name: "explicitly disabled", config: FastPathConfig{Enabled: false, ServingProcesses: 2},
			wantEnabled: false, wantReason: "disabled_by_configuration",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv := New(Config{FastPath: test.config})
			status := srv.FastPathStatus()
			if status.Enabled != test.wantEnabled || status.Reason != test.wantReason {
				t.Fatalf("status = %+v, want enabled=%t reason=%q", status, test.wantEnabled, test.wantReason)
			}
			if (srv.fastPath != nil) != test.wantEnabled {
				t.Fatalf("cache present = %t, want %t", srv.fastPath != nil, test.wantEnabled)
			}
		})
	}
}

func TestFastPathMutationBarrierRejectsStaleFill(t *testing.T) {
	now := time.Date(2026, 8, 7, 3, 0, 0, 0, time.UTC)
	hash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := syncRequest{
		LastReleaseRef: "release", LastDigest: "digest",
		documentHashes: &documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: hash}},
	}
	response := syncResponse{
		Unchanged: true, ReleaseRef: "release", Digest: "digest",
		AcceptedDocumentHashes: &documenthash.Summary{Version: 1, Documents: cloneHashes(request.documentHashes.Documents)},
	}
	cache := newUnchangedSyncCache(FastPathConfig{Enabled: true, MaxEntries: 8, TTL: time.Minute})

	stale := cache.authoritySnapshot("endpoint", "engineering")
	complete := cache.beginMutation(cacheScopeEndpoint, "endpoint")
	cache.putWithSnapshot("endpoint", "engineering", "fingerprint", request, response, now, stale)
	if entries, _, _ := cache.bounds(); entries != 0 {
		t.Fatalf("fill during mutation retained %d entries", entries)
	}
	complete()
	cache.putWithSnapshot("endpoint", "engineering", "fingerprint", request, response, now, stale)
	if entries, _, _ := cache.bounds(); entries != 0 {
		t.Fatalf("stale pre-mutation fill retained %d entries after completion", entries)
	}

	fresh := cache.authoritySnapshot("endpoint", "engineering")
	cache.putWithSnapshot("endpoint", "engineering", "fingerprint", request, response, now, fresh)
	if _, hit := cache.get("endpoint", "fingerprint", request, now); !hit {
		t.Fatal("fresh post-mutation fill did not become eligible")
	}
}

func BenchmarkUnchangedSyncDecision(b *testing.B) {
	now := time.Unix(0, 0)
	hash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := syncRequest{
		LastReleaseRef: "release", LastDigest: "digest",
		documentHashes: &documenthash.Summary{Version: 1, Documents: map[string]string{
			documenthash.Capability: hash, documenthash.SystemInformation: hash,
		}},
	}
	response := syncResponse{
		Unchanged: true, ReleaseRef: "release", Digest: "digest", RemediationPolicy: "auto",
		AcceptedDocumentHashes: &documenthash.Summary{Version: 1, Documents: cloneHashes(request.documentHashes.Documents)},
	}
	cache := newUnchangedSyncCache(FastPathConfig{
		Enabled: true, ServingProcesses: 1, MaxEntries: 400, MaxBytes: 4 << 20,
		MaxObservations: ^uint64(0), TTL: 10 * time.Minute, CheckpointInterval: 5 * time.Minute,
	})
	cache.put("endpoint", "fingerprint", request, response, now)
	b.Run("hit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = cache.get("endpoint", "fingerprint", request, now)
		}
	})
	b.Run("miss", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = cache.get("absent", "fingerprint", request, now)
		}
	})
}
