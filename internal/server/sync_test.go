package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/changecontrol"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSync_returnsFleetArtifactForAuthenticatedEndpoint(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: smoke
    description: e2e
`)

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{
		ID:    "11111111-1111-1111-1111-111111111111",
		Fleet: "test-fleet",
	})

	uri, err := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		ConfigRepoPath: repoDir,
		ReleaseRef:     "e2e",
		Registry:       reg,
	})

	body, _ := json.Marshal(map[string]string{"lastDigest": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}},
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Unchanged {
		t.Fatal("expected artifact, got unchanged")
	}
	if !bytes.Contains(resp.ArtifactYAML, []byte("name: smoke")) {
		t.Fatalf("artifact = %q", resp.ArtifactYAML)
	}
	if resp.ReleaseRef != "e2e" {
		t.Fatalf("releaseRef = %q", resp.ReleaseRef)
	}
	if resp.RemediationPolicy != "auto" {
		t.Fatalf("remediationPolicy = %q", resp.RemediationPolicy)
	}
}

func TestSyncAcknowledgesOnlyValidPreparedRebootIntent(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: smoke\n")
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "e2e", Registry: reg})
	now := time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name       string
		intent     map[string]any
		wantStatus int
	}{
		{"valid", map[string]any{"generation": "kernel-6.12.1", "phase": "awaiting-acknowledgement", "priorBootId": "boot-1", "notBefore": now, "deadline": now.Add(time.Hour)}, http.StatusOK},
		{"missing prior boot identity", map[string]any{"generation": "kernel-6.12.1", "phase": "awaiting-acknowledgement", "notBefore": now}, http.StatusBadRequest},
		{"attempt phase cannot request pre-ack", map[string]any{"generation": "kernel-6.12.1", "phase": "attempting", "priorBootId": "boot-1", "notBefore": now}, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"rebootIntent": tc.intent})
			req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var response struct {
					RebootAcknowledged string `json:"rebootAcknowledged"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.RebootAcknowledged != "kernel-6.12.1" {
					t.Fatalf("reboot acknowledgement = %q", response.RebootAcknowledged)
				}
			}
		})
	}
}

func TestSyncAcknowledgesOnlyValidArmedNetworkTransaction(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: smoke\n")
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "e2e", Registry: reg})
	future := time.Now().UTC().Add(time.Hour)
	validHash := "sha256:" + strings.Repeat("a", 64)

	for _, tc := range []struct {
		name       string
		intent     map[string]any
		wantStatus int
	}{
		{"valid armed transaction", map[string]any{"id": "network-1", "phase": "awaiting-acknowledgement", "deadline": future, "planHash": validHash, "watchdogArmed": true}, http.StatusOK},
		{"watchdog is not armed", map[string]any{"id": "network-1", "phase": "awaiting-acknowledgement", "deadline": future, "planHash": validHash}, http.StatusBadRequest},
		{"deadline elapsed", map[string]any{"id": "network-1", "phase": "awaiting-acknowledgement", "deadline": time.Now().UTC().Add(-time.Minute), "planHash": validHash, "watchdogArmed": true}, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"networkIntent": tc.intent})
			req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				var response struct {
					NetworkAcknowledged string `json:"networkAcknowledged"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.NetworkAcknowledged != "network-1" {
					t.Fatalf("network acknowledgement = %q", response.NetworkAcknowledged)
				}
			}
		})
	}
}

func TestSync_returnsEndpointOverrideWhenPresent(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()

	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: fleet
`)
	writeTestEndpointOverride(t, repoDir, endpointID, `configurations:
  - name: override
`)

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})

	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "e2e", Registry: reg})

	body, _ := json.Marshal(map[string]string{"lastDigest": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(resp.ArtifactYAML, []byte("name: override")) {
		t.Fatalf("artifact = %q", resp.ArtifactYAML)
	}
}

func TestSync_includesFleetRemediationPolicy(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "lab", `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)

	reg := registry.NewMemory()
	reg.SetRemediationPolicy("lab", "report")
	reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "lab"})

	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{
		ConfigRepoPath: repoDir,
		Registry:       reg,
		FleetSettings:  reg,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{"lastDigest":""}`)))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.RemediationPolicy != "report" {
		t.Fatalf("remediationPolicy = %q", resp.RemediationPolicy)
	}
}

func TestSync_gzipWhenAcceptEncoding(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "test-fleet"})
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg})

	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{"lastDigest":""}`)))
	req.Header.Set("Accept-Encoding", "gzip")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("encoding = %q", rec.Header().Get("Content-Encoding"))
	}

	gz, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	var resp syncResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Digest == "" {
		t.Fatal("expected digest in gzip response")
	}
}

type mockTelemetry struct {
	checkInRelease string
	checkInDigest  string
	labels         map[string]string
	usernames      []string
	driftRelease   string
	driftDigest    string
	driftJSON      []byte
	applyAddress   string
	systemDigest   string
	systemJSON     []byte
	firewallDigest string
	firewallJSON   []byte
	stateReports   *registry.Memory
}

func (m *mockTelemetry) RecordEndpointCheckIn(_ context.Context, _, releaseRef, digest string) error {
	m.checkInRelease = releaseRef
	m.checkInDigest = digest
	return nil
}

func (m *mockTelemetry) UpsertEndpointLabels(_ context.Context, _ string, labels map[string]string) error {
	m.labels = labels
	return nil
}

func (m *mockTelemetry) UpsertEndpointSystemInfo(_ context.Context, _, digest string, infoJSON []byte) error {
	m.systemDigest = digest
	m.systemJSON = infoJSON
	return nil
}

func (m *mockTelemetry) InsertDriftReport(_ context.Context, endpointID, releaseRef, digest string, report registry.StateReportPayload) error {
	m.driftRelease = releaseRef
	m.driftDigest = digest
	m.driftJSON, _ = json.Marshal(report)
	if m.stateReports != nil {
		m.stateReports.SetEndpointStateReport(endpointID, registry.DriftSummary{
			ReleaseRef: releaseRef,
			Digest:     digest,
			ReportedAt: time.Now().UTC(),
		}, report)
	}
	return nil
}

func TestSyncPersistsStateReportUnderAgentReportedRelease(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: base\n")
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	tel := &mockTelemetry{}
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-new", Registry: reg, Telemetry: tel})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(`{
		"lastReleaseRef":"release-reported",
		"lastDigest":"sha256:reported",
		"drift":{"digest":"sha256:reported","report":{"schemaVersion":7,"items":[]}}
	}`))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if tel.driftRelease != "release-reported" || tel.driftDigest != "sha256:reported" {
		t.Fatalf("stored report identity = %q/%q", tel.driftRelease, tel.driftDigest)
	}
}

func (m *mockTelemetry) InsertApplyFailure(_ context.Context, _, _, resourceAddress string, _ executor.SafeError) error {
	m.applyAddress = resourceAddress
	return nil
}

func (m *mockTelemetry) UpdateAgentUpgradeReport(context.Context, string, string, string, string, bool) error {
	return nil
}

func (m *mockTelemetry) UpdateEndpointUsernames(_ context.Context, _ string, usernames []string) error {
	m.usernames = append([]string(nil), usernames...)
	return nil
}

func (m *mockTelemetry) InsertFirewallAuditReport(_ context.Context, _ string, digest string, report []byte) error {
	m.firewallDigest = digest
	m.firewallJSON = append([]byte(nil), report...)
	return nil
}

func TestSync_persistsTelemetry(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: "11111111-1111-1111-1111-111111111111", Fleet: "test-fleet"})
	tel := &mockTelemetry{}
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg, Telemetry: tel})

	body := []byte(`{
		"lastDigest":"abc",
		"labels":{"site":"berlin"},
		"usernames":["alice","bob"],
		"systemInfo":{"digest":"s1","report":{"cpu":{"modelName":"Test CPU"}}},
		"drift":{"digest":"d1","report":{"drifted":true}},
		"applyFailure":{"resourceAddress":"cfg/res","message":"failed"},
		"firewallAudit":{"digest":"f1","report":{"defaultIncomingPolicy":"drop"}}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if tel.labels["site"] != "berlin" {
		t.Fatalf("labels = %+v", tel.labels)
	}
	if len(tel.usernames) != 2 || tel.usernames[0] != "alice" || tel.usernames[1] != "bob" {
		t.Fatalf("usernames = %+v", tel.usernames)
	}
	if tel.driftDigest != "d1" {
		t.Fatalf("drift digest = %q", tel.driftDigest)
	}
	if tel.applyAddress != "cfg/res" {
		t.Fatalf("apply address = %q", tel.applyAddress)
	}
	if tel.systemDigest != "s1" {
		t.Fatalf("system digest = %q", tel.systemDigest)
	}
	if len(tel.systemJSON) == 0 {
		t.Fatal("expected system info json")
	}
	if tel.firewallDigest != "f1" || !bytes.Contains(tel.firewallJSON, []byte(`"defaultIncomingPolicy":"drop"`)) {
		t.Fatalf("firewall audit = %q/%s", tel.firewallDigest, tel.firewallJSON)
	}
}

func TestSync_acceptsStructuredThenDowngradedLegacyComplianceReports(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `configurations:
  - name: base
`)

	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	tel := &mockTelemetry{stateReports: reg}
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "compat-window", Registry: reg, Telemetry: tel, StateReports: reg})

	post := func(body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	}

	post(`{
		"lastDigest":"",
		"agentVersion":"v2.0.0",
		"drift":{"digest":"structured","report":{"schemaVersion":2,"inCompliance":false,"items":[{"address":"base/firewall","name":"firewall","provider":"nftables","status":"unsupported","reasonCode":"provider_unavailable"}]}}
	}`)
	structured, ok, err := reg.GetEndpointStateReport(context.Background(), endpointID)
	if err != nil || !ok {
		t.Fatalf("structured report: ok=%t err=%v", ok, err)
	}
	if structured.Status != registry.StateUnsupported || len(structured.Items) != 1 || structured.Items[0].Provider != "nftables" {
		t.Fatalf("structured report = %+v", structured)
	}

	post(`{
		"lastDigest":"",
		"agentVersion":"v1.0.0",
		"drift":{"digest":"legacy-after-downgrade","report":{"inCompliance":false,"items":[{"address":"base/package","name":"package","description":"package missing"}]}}
	}`)
	legacy, ok, err := reg.GetEndpointStateReport(context.Background(), endpointID)
	if err != nil || !ok {
		t.Fatalf("legacy report: ok=%t err=%v", ok, err)
	}
	if legacy.Digest != "legacy-after-downgrade" || legacy.Status != registry.StateDrifted || len(legacy.Items) != 1 || legacy.Items[0].ReasonCode != "legacy_drift" {
		t.Fatalf("legacy report = %+v", legacy)
	}
}

// OS-AEC-074: versioned Sync telemetry is rejected before any durable sink
// when a field claims a secret classification with a raw-value projection.
func TestSyncRejectsUnclassifiedVersion7StateReportBeforePersistence(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: base\n")
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	tel := &mockTelemetry{}
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "classified", Registry: reg, Telemetry: tel})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	body := `{
		"lastDigest":"",
		"drift":{"digest":"unsafe","report":{"schemaVersion":7,"inCompliance":false,"items":[{
			"address":"base/managed","name":"managed","status":"drifted","reasonCode":"state_drift",
			"desiredSummary":{"fields":[{"path":"content","sensitivity":"secret","projection":"value","text":"durable-secret-canary"}]}
		}]}}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if len(tel.driftJSON) != 0 {
		t.Fatalf("invalid classified report reached persistence: %s", tel.driftJSON)
	}
}

func TestSync_deliversEndpointExecutionLeaseFromCurrentPreflight(t *testing.T) {
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: base\n")
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	ids := []string{"request", "rollout", "lease"}
	index := 0
	changes := changecontrol.NewRegistry(changecontrol.RegistryOptions{NewID: func() string { id := ids[index]; index++; return id }})
	requests, err := changes.CreateChangeRequests(changecontrol.FleetPlan{
		Fleet: "test-fleet", ReleaseRef: "release", ArtifactDigest: "artifact",
		Targets:   []changecontrol.TargetEvidence{{EndpointID: endpointID, Compatible: true, PreflightReady: true}},
		Resources: []changecontrol.ResourcePlan{{Address: "base/firewall", DesiredHash: "hash", Risk: models.RiskConnectivity, Provider: "nftables"}},
	}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := changes.AuthorizeRollout(requests[0].ID, changecontrol.RolloutSpec{}, "approver", "CHG-1"); err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg, ChangeControl: changes})
	body := []byte(`{"changePreflights":[{"change_request_id":"request","ready":true}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.ExecutionLeases) != 1 || response.ExecutionLeases[0].ID != "lease" || response.ExecutionLeases[0].EndpointID != endpointID {
		t.Fatalf("leases = %+v", response.ExecutionLeases)
	}
}

func TestSync_rejectsRequestWithoutEndpointIdentity(t *testing.T) {
	srv := New(Config{Registry: registry.NewMemory(), ConfigRepoPath: t.TempDir()})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}
