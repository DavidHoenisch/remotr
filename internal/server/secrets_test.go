package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/ubuntupro"
	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/types"
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

func TestResolveSecretRequiresAuthenticatedKnownEndpointBeforeProviderAccess(t *testing.T) {
	resolver := &recordingSecretResolver{}
	srv := New(Config{Registry: registry.NewMemory(), Secrets: resolver})
	body := []byte(`{"reference":"remotr:ubuntu-pro/shared@active","artifactDigest":"sha256:artifact","resourceAddress":"subscriptions/primary","purpose":"ubuntu-pro-token"}`)

	for name, scenario := range map[string]struct {
		tlsState   *tls.ConnectionState
		wantStatus int
	}{
		"missing client certificate": {wantStatus: http.StatusUnauthorized},
		"certificate without endpoint identity": {
			tlsState:   &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}},
			wantStatus: http.StatusUnauthorized,
		},
		"authenticated unknown endpoint": {
			tlsState: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{
				URIs: []*url.URL{{Scheme: "urn", Opaque: "remotr:endpoint:00000000-0000-0000-0000-000000000001"}},
			}}},
			wantStatus: http.StatusForbidden,
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
			req.TLS = scenario.tlsState
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != scenario.wantStatus {
				t.Fatalf("status=%d body=%q, want %d", rec.Code, rec.Body.String(), scenario.wantStatus)
			}
		})
	}
	if resolver.calls != 0 {
		t.Fatalf("secret provider calls=%d, want zero before endpoint authentication", resolver.calls)
	}
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
	var resolved secrets.Resolved
	if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
		t.Fatal(err)
	}
	if string(resolved.Material) != tokenCanary {
		t.Fatalf("exact resolution material = %q, want canary", resolved.Material)
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

func TestResolveGlobalSecretAcrossTwoAuthenticatedFleetArtifacts(t *testing.T) {
	const reference = "remotr:ubuntu-pro/shared@active"
	const canary = "global-two-fleet-canary"
	repoDir := t.TempDir()
	for _, fleet := range []string{"engineering", "production"} {
		writeTestFleetDesired(t, repoDir, fleet, `schemaVersion: 1
configurations:
  - name: subscriptions
    resources:
      - kind: ubuntuPro
        name: primary
        lifecycle: attached
        tokenRef: `+reference+`
`)
	}
	service := testSecretRegistryService(t, nil, nil)
	if _, err := service.Upload(t.Context(), secrets.UploadRequest{Name: "ubuntu-pro/shared", Scope: secrets.ScopeGlobal, Material: []byte(canary), ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	uses := make([]secrets.ActivationUse, 0, 2)
	for _, fleet := range []string{"engineering", "production"} {
		uses = append(uses, secrets.ActivationUse{Fleet: fleet, ResourceAddress: "subscriptions/primary", Purpose: "ubuntu-pro-token", Risk: models.RiskNormal})
	}
	if _, err := service.Activate(t.Context(), secrets.ActivationRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1", Uses: uses}); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewMemory()
	endpoints := map[string]string{
		"11111111-1111-1111-1111-111111111111": "engineering",
		"22222222-2222-2222-2222-222222222222": "production",
	}
	for endpointID, fleet := range endpoints {
		if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: fleet}); err != nil {
			t.Fatal(err)
		}
	}
	artifactStore := &OnDemandArtifactResolver{RepoRoot: repoDir}
	srv := New(Config{ConfigRepoPath: repoDir, ArtifactStore: artifactStore, ReleaseRef: "release-1", Registry: reg, Secrets: service})
	for endpointID, fleet := range endpoints {
		_, digest, err := resolveFleetDesiredArtifact(t.Context(), artifactStore, repoDir, fleet, "release-1")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]string{
			"reference": reference, "artifactDigest": digest, "resourceAddress": "subscriptions/primary", "purpose": "ubuntu-pro-token",
		})
		uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
		req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("fleet %s status=%d body=%s", fleet, rec.Code, rec.Body.String())
		}
		var resolved secrets.Resolved
		if err := json.Unmarshal(rec.Body.Bytes(), &resolved); err != nil {
			t.Fatal(err)
		}
		if resolved.Version != "1" || string(resolved.Material) != canary {
			t.Fatalf("fleet %s resolved=%#v", fleet, resolved)
		}
		runner := &globalUbuntuProRunner{token: canary}
		resource := models.UbuntuProResource{
			ResourceMeta: models.ResourceMeta{Lifecycle: models.UbuntuProAttached},
			Name:         "primary", TokenRef: reference,
		}
		applicator := ubuntupro.New(resource, facts.Facts{
			Distro: types.Ubuntu, DistroVersion: "24.04", Arch: types.X86, Package: types.Apt,
			OSID: "ubuntu", OSReleaseSourceCount: 2, OSReleaseConsistent: true, DistroVendor: "Ubuntu",
		}, runner, func(context.Context, string) ([]byte, error) {
			return append([]byte(nil), resolved.Material...), nil
		})
		if result := applicator.ApplyResult(t.Context()); result.Status != executor.Changed {
			t.Fatalf("fleet %s Apply = %#v", fleet, result)
		}
		if check := applicator.Check(t.Context()); check.Status != executor.Compliant {
			t.Fatalf("fleet %s second Check = %#v", fleet, check)
		}
		if !runner.sawProtectedToken || runner.tokenInArgv {
			t.Fatalf("fleet %s protected input evidence = %#v", fleet, runner)
		}
	}
	metadata, err := service.ListMetadata(t.Context(), "ubuntu-pro/shared")
	if err != nil || len(metadata) != 1 || metadata[0].Scope != secrets.ScopeGlobal {
		t.Fatalf("global history=%#v err=%v", metadata, err)
	}

	endpointID := "11111111-1111-1111-1111-111111111111"
	_, digest, err := resolveFleetDesiredArtifact(t.Context(), artifactStore, repoDir, "engineering", "release-1")
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	resolveDenied := func(fields map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(fields)
		req := httptest.NewRequest(http.MethodPost, "/v1/secrets/resolve", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	base := map[string]string{"reference": reference, "artifactDigest": digest, "resourceAddress": "subscriptions/primary", "purpose": "ubuntu-pro-token"}
	denialBodies := []string{}
	for name, mutate := range map[string]func(map[string]string){
		"wrong artifact": func(fields map[string]string) { fields["artifactDigest"] = "sha256:wrong" },
		"wrong resource": func(fields map[string]string) { fields["resourceAddress"] = "subscriptions/other" },
		"wrong purpose":  func(fields map[string]string) { fields["purpose"] = "repository-credential" },
	} {
		t.Run(name, func(t *testing.T) {
			fields := maps.Clone(base)
			mutate(fields)
			rec := resolveDenied(fields)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			denialBodies = append(denialBodies, rec.Body.String())
		})
	}
	if _, err := service.Revoke(t.Context(), secrets.RevokeRequest{Name: "ubuntu-pro/shared", Version: "1", ActorID: "operator-1"}); err != nil {
		t.Fatal(err)
	}
	revoked := resolveDenied(base)
	if revoked.Code != http.StatusForbidden {
		t.Fatalf("revoked status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	denialBodies = append(denialBodies, revoked.Body.String())
	for _, body := range denialBodies {
		if body != denialBodies[0] || strings.Contains(body, canary) || strings.Contains(body, "global") || strings.Contains(body, "ubuntu-pro/shared") || strings.Contains(body, "version") {
			t.Fatalf("authorization denial leaked or differed: %q vs %q", body, denialBodies[0])
		}
	}
}

type globalUbuntuProRunner struct {
	token             string
	attached          bool
	sawProtectedToken bool
	tokenInArgv       bool
}

func (r *globalUbuntuProRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return r.RunContext(context.Background(), name, args...)
}

func (r *globalUbuntuProRunner) RunContext(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	for _, arg := range args {
		if strings.Contains(arg, r.token) {
			r.tokenInArgv = true
		}
	}
	if name != "/usr/bin/pro" || len(args) != 2 || args[0] != "api" || args[1] != "u.pro.status.is_attached.v1" {
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro read boundary")
	}
	return globalUbuntuProEnvelope("IsAttachedResult", fmt.Sprintf(`{"is_attached":%t}`, r.attached)), nil, nil
}

func (r *globalUbuntuProRunner) RunInput(name string, input []byte, args ...string) ([]byte, []byte, error) {
	return r.RunInputContext(context.Background(), name, input, args...)
}

func (r *globalUbuntuProRunner) RunInputContext(_ context.Context, name string, input []byte, args ...string) ([]byte, []byte, error) {
	for _, arg := range args {
		if strings.Contains(arg, r.token) {
			r.tokenInArgv = true
		}
	}
	if name != "/usr/bin/pro" || len(args) != 4 || args[0] != "api" || args[1] != "u.pro.attach.token.full_token_attach.v1" || args[2] != "--data" || args[3] != "-" {
		return nil, nil, fmt.Errorf("unexpected Ubuntu Pro mutation boundary")
	}
	var request struct {
		Token              string `json:"token"`
		AutoEnableServices bool   `json:"auto_enable_services"`
	}
	if err := json.Unmarshal(input, &request); err != nil || request.AutoEnableServices || request.Token != r.token {
		return nil, nil, fmt.Errorf("invalid protected Ubuntu Pro attachment input")
	}
	r.sawProtectedToken = true
	r.attached = true
	return globalUbuntuProEnvelope("FullTokenAttachResult", `{"enabled":[],"reboot_required":false}`), nil, nil
}

func globalUbuntuProEnvelope(resultType, attributes string) []byte {
	return []byte(fmt.Sprintf(`{"_schema_version":"v1","data":{"attributes":%s,"meta":{"environment_vars":[]},"type":%q},"errors":[],"result":"success","version":"32.3ubuntu0","warnings":[]}`, attributes, resultType))
}
