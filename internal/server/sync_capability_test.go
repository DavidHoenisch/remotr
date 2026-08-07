package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSyncCapabilityDocumentBoundToMTLSEndpointIdentity(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, err := (capabilitydoc.Document{
		DocumentVersion:        1,
		ArtifactSchemaVersions: []int{0, 1},
		Capabilities:           []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:                  []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
		AgentVersion:           "v1.2.3",
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC)
	server := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg, Now: func() time.Time { return now }})

	t.Run("valid current evidence uses certificate identity", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("artifactYaml")) {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		stored, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID)
		if err != nil || !ok || stored.EndpointID != endpointID || stored.Digest != document.Digest || stored.ReceivedAt.IsZero() {
			t.Fatalf("stored capability document = %+v, ok=%t err=%v", stored, ok, err)
		}
	})

	t.Run("unchanged digest retains current request observation without durable rewrite", func(t *testing.T) {
		now = now.Add(time.Hour)
		body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		persisted, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID)
		if err != nil || !ok || persisted.ReceivedAt.Equal(now) {
			t.Fatalf("persisted unchanged evidence = %+v, ok=%t err=%v", persisted, ok, err)
		}
		observed, ok := server.currentCapabilityEvidence(endpointID)
		if !ok || !observed.ReceivedAt.Equal(now) || observed.Digest != document.Digest {
			t.Fatalf("current request evidence = %+v, ok=%t", observed, ok)
		}
	})

	t.Run("tampered evidence cannot select an artifact", func(t *testing.T) {
		tampered := document
		tampered.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": tampered})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusOK && bytes.Contains(rec.Body.Bytes(), []byte("artifactYaml")) {
			t.Fatalf("tampered evidence selected artifact: %s", rec.Body.String())
		}
	})

	t.Run("bearer header cannot replace endpoint certificate", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer not-an-endpoint-identity")
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}

// OS-USF-003: a client-declared hash is only evidence after the server has
// recomputed it over the canonical, domain-separated capability document.
func TestSyncRejectsMismatchedCapabilityDocumentHashWithoutPersistence(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, err := (capabilitydoc.Document{
		DocumentVersion:        1,
		ArtifactSchemaVersions: []int{0, 1},
		Capabilities:           []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:                  []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
		AgentVersion:           "v1.2.3",
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"agentVersion":       document.AgentVersion,
		"capabilityDocument": document,
		"documentHashes": map[string]any{
			"version": 1,
			"documents": map[string]string{
				"capability": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if stored, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID); err != nil || ok {
		t.Fatalf("mismatched document was persisted: %+v, ok=%t err=%v", stored, ok, err)
	}
}

func TestSyncAcknowledgesCanonicalCapabilityHashAfterPersistence(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := document.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := documenthash.Digest(documenthash.Capability, canonical)
	if err != nil {
		t.Fatal(err)
	}
	deliveryBody, err := documenthash.CanonicalDelivery("", "")
	if err != nil {
		t.Fatal(err)
	}
	deliveryHash, err := documenthash.Digest(documenthash.Delivery, deliveryBody)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{"distro": "ubuntu", "arch": "x86"}
	usernames := []string{"alice", "bob"}
	targetingBody, err := documenthash.CanonicalTargeting(labels, usernames)
	if err != nil {
		t.Fatal(err)
	}
	targetingHash, err := documenthash.Digest(documenthash.Targeting, targetingBody)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"agentVersion": document.AgentVersion, "capabilityDocument": document, "labels": labels, "usernames": usernames,
		"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{
			documenthash.Capability: hash, documenthash.Delivery: deliveryHash, documenthash.Targeting: targetingHash,
		}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Accepted *documenthash.Summary `json:"acceptedDocumentHashes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Accepted == nil || response.Accepted.Documents[documenthash.Capability] != hash ||
		response.Accepted.Documents[documenthash.Delivery] != deliveryHash || response.Accepted.Documents[documenthash.Targeting] != targetingHash {
		t.Fatalf("accepted hashes = %+v, want capability, delivery, and targeting", response.Accepted)
	}
	if stored, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID); err != nil || !ok || stored.Digest != document.Digest {
		t.Fatalf("persisted document = %+v, ok=%t err=%v", stored, ok, err)
	}
}

func TestSyncRejectsMismatchedDeliveryOrTargetingHashWithoutPersistence(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	const falseHash = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "delivery",
			body: map[string]any{
				"agentVersion": "v0.1.12", "lastReleaseRef": "release-1", "lastDigest": "digest-1",
				"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Delivery: falseHash}},
			},
		},
		{
			name: "targeting",
			body: map[string]any{
				"agentVersion": "v0.1.12", "labels": map[string]string{"distro": "ubuntu"}, "usernames": []string{"alice"},
				"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Targeting: falseHash}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reg := registry.NewMemory()
			if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "legacy"}); err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(test.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
			rec := httptest.NewRecorder()
			New(Config{Registry: reg}).Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			endpoint, ok := reg.EndpointByID(endpointID)
			if !ok || len(endpoint.Labels) != 0 || len(endpoint.Usernames) != 0 {
				t.Fatalf("mismatched document persisted targeting state: %+v", endpoint)
			}
		})
	}
}

func TestHashOnlySystemInformationUsesDurableCanonicalContent(t *testing.T) {
	report := json.RawMessage(`{"cpu":{"model":"test"},"memory":1024}`)
	canonical, err := documenthash.CanonicalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := documenthash.Digest(documenthash.SystemInformation, canonical)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := registry.Endpoint{SystemInfo: &registry.SystemInfoSummary{ReportJSON: []byte(" { \"memory\" : 1024, \"cpu\" : { \"model\" : \"test\" } } ")}}
	request := syncRequest{documentHashes: &documenthash.Summary{
		Version: documenthash.CurrentVersion, Documents: map[string]string{documenthash.SystemInformation: hash},
	}}
	if err := restoreHashOnlySystemInformation(endpoint, &request); err != nil {
		t.Fatal(err)
	}
	if request.acceptedHashes[documenthash.SystemInformation] != hash || len(requestedDocuments(request)) != 0 {
		t.Fatalf("durably matched system information was not accepted: accepted=%v requested=%v", request.acceptedHashes, requestedDocuments(request))
	}

	request = syncRequest{documentHashes: &documenthash.Summary{
		Version: documenthash.CurrentVersion, Documents: map[string]string{documenthash.SystemInformation: "sha256:" + strings.Repeat("0", 64)},
	}}
	if err := restoreHashOnlySystemInformation(endpoint, &request); err != nil {
		t.Fatal(err)
	}
	if got := requestedDocuments(request); len(got) != 1 || got[0] != documenthash.SystemInformation {
		t.Fatalf("mismatched durable system information requests = %v", got)
	}
}

func TestSyncRequestsUnknownHashOnlyCapabilityDocument(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	body, _ := json.Marshal(map[string]any{
		"agentVersion": "v1.2.3",
		"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{
			documenthash.Capability: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response agentsync.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.RequestedDocuments) != 1 || response.RequestedDocuments[0] != documenthash.Capability || response.CapabilityBlocked == nil {
		t.Fatalf("hash-only response = %+v", response)
	}
	if stored, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID); err != nil || ok {
		t.Fatalf("hash-only request established capability state: %+v, ok=%t err=%v", stored, ok, err)
	}
}

func TestSyncModernAgentMissingOrInvalidCapabilityDocumentBlocks(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, _ := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	server := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg})
	send := func(t *testing.T, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := send(t, map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document}); rec.Code != http.StatusOK {
		t.Fatalf("initial sync status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec := send(t, map[string]any{"agentVersion": "v1.2.3"})
	if rec.Code != http.StatusOK {
		t.Fatalf("omission status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		ArtifactYAML      []byte `json:"artifactYaml"`
		CapabilityBlocked *struct {
			TargetReleaseRef string `json:"targetReleaseRef"`
		} `json:"capabilityBlocked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ArtifactYAML) != 0 || response.CapabilityBlocked == nil || response.CapabilityBlocked.TargetReleaseRef != "release-modern" {
		t.Fatalf("modern omission response = %s", rec.Body.String())
	}
	if _, ok := server.currentCapabilityEvidence(endpointID); ok {
		t.Fatal("missing current evidence was substituted from persisted state")
	}

	tampered := document
	tampered.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	rec = send(t, map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": tampered})
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("capabilityBlocked")) || bytes.Contains(rec.Body.Bytes(), []byte("artifactYaml")) {
		t.Fatalf("invalid modern evidence status=%d body=%s", rec.Code, rec.Body.String())
	}

	freshEndpointID := "22222222-2222-2222-2222-222222222222"
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: freshEndpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	freshIdentity, _ := url.Parse("urn:remotr:endpoint:" + freshEndpointID)
	raw, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3"})
	freshRequest := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
	freshRequest.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{freshIdentity}}}}
	freshResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(freshResponse, freshRequest)
	if freshResponse.Code != http.StatusOK || !bytes.Contains(freshResponse.Body.Bytes(), []byte("capabilityBlocked")) || bytes.Contains(freshResponse.Body.Bytes(), []byte("artifactYaml")) {
		t.Fatalf("fresh modern omission status=%d body=%s", freshResponse.Code, freshResponse.Body.String())
	}
}

func TestSyncReconnectUsesCurrentCapabilityDocument(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	server := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg, Now: func() time.Time { return now }})
	send := func(t *testing.T, document capabilitydoc.Document) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"agentVersion": document.AgentVersion, "capabilityDocument": document})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec
	}
	oldDocument, _ := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	if rec := send(t, oldDocument); rec.Code != http.StatusOK {
		t.Fatalf("initial sync status=%d body=%s", rec.Code, rec.Body.String())
	}

	now = now.AddDate(1, 0, 0)
	currentDocument := oldDocument
	currentDocument.Facts = []capabilitydoc.Fact{{Key: "architecture", Value: "arm"}}
	currentDocument, _ = currentDocument.WithCanonicalDigest()
	if rec := send(t, currentDocument); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("artifactYaml")) {
		t.Fatalf("reconnect status=%d body=%s", rec.Code, rec.Body.String())
	}
	current, ok := server.currentCapabilityEvidence(endpointID)
	if !ok || current.Digest != currentDocument.Digest || !current.ReceivedAt.Equal(now) {
		t.Fatalf("current reconnect evidence = %+v, ok=%t", current, ok)
	}
	persisted, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID)
	if err != nil || !ok || persisted.Digest != currentDocument.Digest || !persisted.ReceivedAt.Equal(now) {
		t.Fatalf("persisted reconnect evidence = %+v, ok=%t err=%v", persisted, ok, err)
	}
}

func TestCapabilityPersistenceSurvivesServerRestart(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, _ := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	send := func(t *testing.T, server *Server, body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec
	}
	first := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg})
	if rec := send(t, first, map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document}); rec.Code != http.StatusOK {
		t.Fatalf("initial sync status=%d body=%s", rec.Code, rec.Body.String())
	}
	restarted := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg})
	if _, ok := restarted.currentCapabilityEvidence(endpointID); ok {
		t.Fatal("new server unexpectedly retained process-local current evidence")
	}
	rec := send(t, restarted, map[string]any{"agentVersion": "v1.2.3"})
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("capabilityBlocked")) || bytes.Contains(rec.Body.Bytes(), []byte("artifactYaml")) {
		t.Fatalf("restart readiness response status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSyncRejectsSecretBearingCapabilityFactWithoutStorageOrDisclosure(t *testing.T) {
	const canary = "capability-secret-canary"
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "modern", "configurations:\n  - name: modern\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, _ := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: canary}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	raw, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(raw))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("capabilityBlocked")) || bytes.Contains(rec.Body.Bytes(), []byte(canary)) {
		t.Fatalf("secret-bearing fact response status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(logs.String(), canary) {
		t.Fatalf("secret-bearing fact entered logs: %s", logs.String())
	}
	if stored, ok, err := reg.GetEndpointCapabilityDocument(t.Context(), endpointID); err != nil || ok {
		t.Fatalf("secret-bearing fact was stored: %+v, ok=%t err=%v", stored, ok, err)
	}
}
