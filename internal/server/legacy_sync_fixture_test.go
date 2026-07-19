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
		t.Fatalf("legacy response = %+v", response)
	}
	if telemetry.checkInRelease != "release-legacy" || telemetry.checkInDigest != response.Digest {
		t.Fatalf("legacy active baseline = release %q digest %q", telemetry.checkInRelease, telemetry.checkInDigest)
	}
}
