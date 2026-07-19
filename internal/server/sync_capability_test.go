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
	server := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-modern", Registry: reg})

	t.Run("valid current evidence uses certificate identity", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("artifactYaml")) {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
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
