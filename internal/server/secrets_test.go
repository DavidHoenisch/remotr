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

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

type recordingSecretResolver struct {
	request  secrets.ResolveRequest
	calls    int
	material string
}

func (r *recordingSecretResolver) Resolve(_ context.Context, request secrets.ResolveRequest) (secrets.Resolved, error) {
	r.request = request
	r.calls++
	material := r.material
	if material == "" {
		material = "secret-canary"
	}
	return secrets.Resolved{
		Provider:    secrets.ProviderRemotr,
		Version:     "7",
		Fingerprint: "sha256:safe",
		Material:    []byte(material),
	}, nil
}

// OS-UPM-010 through OS-UPM-013 and OS-UPM-016: the authenticated endpoint
// may resolve the active artifact's exact Ubuntu Pro token use, and no sibling
// purpose, reference, resource, fleet, or endpoint scope reaches the provider.
func TestResolveUbuntuProTokenAuthorizesExactActiveArtifactScope(t *testing.T) {
	const endpointID = "11111111-1111-1111-1111-111111111111"
	const tokenReference = "remotr:ubuntu-pro/production@active"
	const tokenCanary = "ubuntu-pro-token-canary-4-1"
	repoDir := t.TempDir()
	writeTestFleetDesired(t, repoDir, "test-fleet", `schemaVersion: 1
configurations:
  - name: ubuntu-pro
    resources:
      - kind: ubuntuPro
        name: primary-subscription
        lifecycle: attached
        tokenRef: `+tokenReference+`
`)

	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	resolver := &recordingSecretResolver{material: tokenCanary}
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-1", Registry: reg, Secrets: resolver})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{{ID: "resource:ubuntu-pro", Revision: "ubuntu-pro-v1"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	syncBody, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
	syncReq := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(syncBody))
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

	resolve := func(reference, address, purpose string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{
			"reference": reference, "artifactDigest": syncResponse.Digest, "resourceAddress": address, "purpose": purpose,
			"endpointId": "attacker-endpoint", "fleet": "attacker-fleet",
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}

	rec := resolve(tokenReference, "ubuntu-pro/primary-subscription", "ubuntu-pro-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("exact resolution status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(tokenCanary)) {
		t.Fatalf("exact resolution omitted provider material: %s", rec.Body.String())
	}
	if resolver.calls != 1 || resolver.request.EndpointID != endpointID || resolver.request.Fleet != "test-fleet" ||
		resolver.request.ArtifactDigest != syncResponse.Digest || resolver.request.ResourceAddress != "ubuntu-pro/primary-subscription" ||
		resolver.request.Reference != tokenReference || resolver.request.Purpose != "ubuntu-pro-token" {
		t.Fatalf("resolver request = %#v, calls = %d", resolver.request, resolver.calls)
	}

	for name, request := range map[string][3]string{
		"wrong purpose":   {tokenReference, "ubuntu-pro/primary-subscription", "network-credential"},
		"wrong reference": {"remotr:ubuntu-pro/sibling@active", "ubuntu-pro/primary-subscription", "ubuntu-pro-token"},
		"wrong resource":  {tokenReference, "sibling/primary-subscription", "ubuntu-pro-token"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := resolve(request[0], request[1], request[2])
			if rec.Code != http.StatusForbidden || resolver.calls != 1 {
				t.Fatalf("status = %d, body = %s, provider calls = %d", rec.Code, rec.Body.String(), resolver.calls)
			}
		})
	}
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
      - kind: download
        name: helper
        url: https://downloads.example.test/helper
        dest: /opt/remotr/helper
        authenticationRef: remotr:downloads/helper@active
`)

	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"}); err != nil {
		t.Fatal(err)
	}
	resolver := &recordingSecretResolver{}
	srv := New(Config{ConfigRepoPath: repoDir, ReleaseRef: "release-1", Registry: reg, Secrets: resolver})
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)

	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{1}, AgentVersion: "v1.2.3",
		Capabilities: []capabilitydoc.Capability{
			{ID: "resource:network-profile", Revision: "networkProfile-v1"},
			{ID: "resource:download", Revision: "download-v1"},
			{ID: "provider:network/network-manager", Revision: "1"},
		},
		Facts: []capabilitydoc.Fact{{Key: "network", Value: "network-manager"}},
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	syncBody, _ := json.Marshal(map[string]any{"agentVersion": "v1.2.3", "capabilityDocument": document})
	syncReq := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(syncBody))
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

	downloadBody, _ := json.Marshal(map[string]string{
		"reference":       "remotr:downloads/helper@active",
		"artifactDigest":  syncResponse.Digest,
		"resourceAddress": "office/helper",
		"purpose":         "download-authentication",
	})
	downloadReq := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(downloadBody))
	downloadReq.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	downloadRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK || resolver.calls != 2 {
		t.Fatalf("download resolution status = %d body=%s provider calls=%d", downloadRec.Code, downloadRec.Body.String(), resolver.calls)
	}
	if resolver.request.ResourceAddress != "office/helper" || resolver.request.Reference != "remotr:downloads/helper@active" || resolver.request.Purpose != "download-authentication" {
		t.Fatalf("download resolver request = %#v", resolver.request)
	}

	var wrongPurpose map[string]string
	if err := json.Unmarshal(downloadBody, &wrongPurpose); err != nil {
		t.Fatal(err)
	}
	wrongPurpose["purpose"] = "password-hash"
	body, _ = json.Marshal(wrongPurpose)
	req = httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || resolver.calls != 2 {
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
