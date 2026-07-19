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

func TestSyncExistingEndpointCapabilityBlockedRetainsActiveArtifact(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"agentVersion": "v1.2.3", "capabilityDocument": document,
		"lastReleaseRef": "release-active", "lastDigest": "digest-active",
	})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{
		ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Telemetry: telemetry,
	}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CapabilityBlocked == nil || response.CapabilityBlocked.TargetReleaseRef != "release-target" || len(response.ArtifactYAML) != 0 {
		t.Fatalf("blocked response = %s", rec.Body.String())
	}
	if telemetry.checkInRelease != "release-active" || telemetry.checkInDigest != "digest-active" {
		t.Fatalf("active artifact advanced or was lost: release=%q digest=%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
}

func TestSyncNewEndpointCapabilityBlockedIsUnmanaged(t *testing.T) {
	const endpointID = "22222222-2222-2222-2222-222222222222"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "engineering", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering"}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-target", Registry: reg, Telemetry: telemetry}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.CapabilityBlocked == nil || !response.CapabilityBlocked.Unmanaged || len(response.ArtifactYAML) != 0 {
		t.Fatalf("new blocked endpoint response = %s", rec.Body.String())
	}
	if telemetry.checkInRelease != "" || telemetry.checkInDigest != "" {
		t.Fatalf("new blocked endpoint gained active state: release=%q digest=%q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
}
