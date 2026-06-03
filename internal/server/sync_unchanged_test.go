package server

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestSync_returnsArtifactWhenReleaseRefAdvancesWithSameDigest(t *testing.T) {
	repoDir := t.TempDir()
	fleetDir := filepath.Join(repoDir, "fleets", "test-fleet")
	if err := os.MkdirAll(fleetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := []byte("configurations:\n  - name: smoke\n")
	if err := os.WriteFile(filepath.Join(fleetDir, "desired.yaml"), artifact, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(artifact)
	digest := hex.EncodeToString(sum[:])

	reg := registry.NewMemory()
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})

	uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		ConfigRepoPath: repoDir,
		ReleaseRef:     "new-ref",
		Registry:       reg,
	})

	body, _ := json.Marshal(map[string]string{
		"lastDigest":     digest,
		"lastReleaseRef": "old-ref",
	})
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
		t.Fatal("expected artifact when release ref advanced")
	}
	if string(resp.ArtifactYAML) != string(artifact) {
		t.Fatalf("artifact = %q", resp.ArtifactYAML)
	}
	if resp.ReleaseRef != "new-ref" {
		t.Fatalf("releaseRef = %q", resp.ReleaseRef)
	}
}
