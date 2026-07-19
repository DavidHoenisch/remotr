package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

// TestLegacySyncFixtureFreezesSchema0Delivery records the independently known
// pre-capability authenticated Sync contract. Capability-aware selection must
// retain this behavior only for a reviewed known-legacy profile.
func TestLegacySyncFixtureFreezesSchema0Delivery(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	desired, err := os.ReadFile("testdata/legacy-schema-0-desired.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFleetDesired(t, repoDir, "legacy", string(desired))

	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "legacy"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	body, err := os.ReadFile("testdata/legacy-sync-request.json")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	telemetry := &mockTelemetry{}
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-legacy", Registry: reg, Telemetry: telemetry}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ReleaseRef != "release-legacy" || response.Digest == "" || !bytes.Contains(response.ArtifactYAML, []byte("legacy-baseline")) {
		t.Fatalf("legacy response = %+v blocked=%+v body=%s", response, response.CapabilityBlocked, rec.Body.String())
	}
	if telemetry.checkInRelease != "release-legacy" || telemetry.checkInDigest != response.Digest {
		t.Fatalf("legacy active baseline = release %q digest %q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
}

func TestSyncKnownLegacyAgentSelectsLosslessSchema0(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "legacy", `configurations:
  - name: legacy-baseline
    description: known legacy schema profile
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "legacy"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{"agentVersion":"v0.1.12"}`)))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-legacy", Registry: reg}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ArtifactYAML) == 0 || bytes.Contains(response.ArtifactYAML, []byte("schemaVersion:")) {
		t.Fatalf("known legacy artifact is not schema 0, blocked=%+v body=%s:\n%s", response.CapabilityBlocked, rec.Body.String(), response.ArtifactYAML)
	}
}

func TestLegacyCapabilityProfileMappingIsVersionedAndExact(t *testing.T) {
	if legacyCapabilityProfileMappingVersion != 1 {
		t.Fatalf("legacy capability profile mapping version = %d", legacyCapabilityProfileMappingVersion)
	}
	if document, ok := knownLegacyCapabilityDocument("v0.1.12"); !ok || len(document.ArtifactSchemaVersions) != 1 || document.ArtifactSchemaVersions[0] != 0 {
		t.Fatalf("known legacy profile = %+v, ok=%t", document, ok)
	}
	if _, ok := knownLegacyCapabilityDocument("v0.1.13"); ok {
		t.Fatal("unknown version inherited a known legacy profile")
	}
}

func TestSyncUnknownLegacyAgentUsesMinimalBaseline(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "unknown", `configurations:
  - name: base
    packages:
      - name: curl
        present: true
        packageManager: apt
`)
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "unknown"}); err != nil {
		t.Fatal(err)
	}
	identity, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader([]byte(`{"agentVersion":"mystery"}`)))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identity}}}}
	rec := httptest.NewRecorder()
	New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-unknown", Registry: reg}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response syncResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ArtifactYAML) != 0 || response.CapabilityBlocked == nil {
		t.Fatalf("unknown-version response = %s", rec.Body.String())
	}
	missingPackage := false
	for _, requirement := range response.CapabilityBlocked.MissingRequirements {
		if requirement.ID == "resource:package" && requirement.Revision == "package-v1" {
			missingPackage = true
		}
		if requirement.ID == "schema:0" {
			t.Fatalf("minimal legacy baseline did not retain schema 0: %+v", response.CapabilityBlocked.MissingRequirements)
		}
	}
	if !missingPackage {
		t.Fatalf("minimal legacy missing requirements = %+v", response.CapabilityBlocked.MissingRequirements)
	}
}
