package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type recordingSecretResolver struct {
	request secrets.ResolveRequest
	calls   int
}

func (r *recordingSecretResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.request = request
	r.calls++
	return secrets.Resolved{
		Provider:    secrets.ProviderRemotr,
		Version:     "7",
		Fingerprint: "sha256:safe",
		Material:    []byte("secret-canary"),
	}, nil
}

func TestResolveSecretAuthorizesEndpointArtifactResourceAndPurpose(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `schemaVersion: 1
configurations:
  - name: office
    resources:
      - kind: networkProfile
        name: wifi
        provider: network-manager
        selector: {name: wlan0}
        profileName: office
        profileType: wifi
        ssid: corp
        credentialRef: remotr:wifi/office@active
`)

	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	resolver := &recordingSecretResolver{}
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-1", Registry: reg, Secrets: resolver})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)

	syncReq := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(`{}`))
	syncReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	syncRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync status = %d, body = %s", syncRec.Code, syncRec.Body.String())
	}
	var syncResponse struct {
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(syncRec.Body.Bytes(), &syncResponse); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{
		"reference":       "remotr:wifi/office@active",
		"artifactDigest":  syncResponse.Digest,
		"resourceAddress": "office/wifi",
		"purpose":         "network-credential",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Provider    string `json:"provider"`
		Version     string `json:"version"`
		Fingerprint string `json:"fingerprint"`
		Material    []byte `json:"material"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if string(response.Material) != "secret-canary" || response.Provider != "remotr" || response.Version != "7" {
		t.Fatalf("response = %#v", response)
	}
	if resolver.request.EndpointID != endpointID || resolver.request.Fleet != "test-fleet" || resolver.request.ArtifactDigest != syncResponse.Digest || resolver.request.ResourceAddress != "office/wifi" || resolver.request.Purpose != "network-credential" {
		t.Fatalf("resolver request = %#v", resolver.request)
	}

	var wrongPurpose map[string]string
	if err := json.Unmarshal(body, &wrongPurpose); err != nil {
		t.Fatal(err)
	}
	wrongPurpose["purpose"] = "password-hash"
	body, _ = json.Marshal(wrongPurpose)
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || resolver.calls != 1 {
		t.Fatalf("wrong-purpose status = %d, provider calls = %d", rec.Code, resolver.calls)
	}
}

func TestResolveSecretRejectsMissingArtifactBindingBeforeProviderAccess(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", "configurations:\n  - name: base\n")
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	resolver := &recordingSecretResolver{}
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-1", Registry: reg, Secrets: resolver})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	body := bytes.NewBufferString(`{"reference":"remotr:wifi/office@active","artifactDigest":"wrong","resourceAddress":"office/wifi","purpose":"network-credential"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", body)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if resolver.request.Reference != "" {
		t.Fatalf("provider was called for unauthorized request: %#v", resolver.request)
	}
}
