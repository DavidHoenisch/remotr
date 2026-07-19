package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
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
