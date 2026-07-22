package admin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/server"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

// OS-AEC-086: the operator Admin API client must preserve the endpoint's
// canonical plan and preflight evidence so JSON output can drive change control.
func TestClientStateReportPreservesCanonicalPlanEvidence(t *testing.T) {
	const response = `{
  "endpoint_id":"endpoint-1",
  "fleet":"engineering",
  "schema_version":9,
  "reported_at":"2026-07-22T07:34:07Z",
  "in_compliance":false,
  "status":"drifted",
  "items":[{
    "address":"subscriptions/primary",
    "name":"primary",
    "description":"ubuntuPro",
    "provider":"ubuntu-pro",
    "providerRevision":"ubuntu-pro-v1",
    "effectiveHash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "status":"drifted",
    "reasonCode":"state_drift",
    "preflightStatus":"blocked",
    "preflightReason":"preflight_failed"
  }],
  "apply":[{
    "address":"subscriptions/primary",
    "name":"primary",
    "provider":"ubuntu-pro",
    "providerRevision":"ubuntu-pro-v1",
    "effectiveHash":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "status":"failed"
  }]
}`
	client := &Client{
		BaseURL: "https://remotr.example.test",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet || request.URL.Path != "/v1/admin/endpoints/endpoint-1/state-report" {
				t.Fatalf("state report request = %s %s", request.Method, request.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(response)),
				Request:    request,
			}, nil
		})},
	}

	report, err := client.GetEndpointStateReport("endpoint-1")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if output["schema_version"] != float64(9) {
		t.Fatalf("schema_version = %#v in %s, want 9", output["schema_version"], encoded)
	}
	items, ok := output["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items = %#v in %s", output["items"], encoded)
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["providerRevision"] != "ubuntu-pro-v1" || item["effectiveHash"] == nil || item["preflightStatus"] != "blocked" || item["preflightReason"] != "preflight_failed" {
		t.Fatalf("canonical item evidence = %#v, want provider revision, hash, and blocked preflight", items[0])
	}
	apply, ok := output["apply"].([]any)
	if !ok || len(apply) != 1 {
		t.Fatalf("apply = %#v in %s", output["apply"], encoded)
	}
	applyItem, ok := apply[0].(map[string]any)
	if !ok || applyItem["providerRevision"] != "ubuntu-pro-v1" || applyItem["effectiveHash"] == nil {
		t.Fatalf("canonical apply evidence = %#v, want provider revision and hash", apply[0])
	}
}

func TestEndpointJSONCompatibilityFixtures(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		check func(*testing.T, Endpoint)
	}{
		{
			name: "legacy record omits delivery state",
			path: "testdata/endpoint_legacy.json",
			check: func(t *testing.T, endpoint Endpoint) {
				t.Helper()
				if endpoint.TargetReleaseRef != "" || endpoint.OfferedReleaseRef != "" || endpoint.ActiveReleaseRef != "" || endpoint.CapabilityDigest != "" || endpoint.Unmanaged {
					t.Fatalf("legacy endpoint invented delivery state: %#v", endpoint)
				}
				encoded, err := json.Marshal(endpoint)
				if err != nil {
					t.Fatalf("marshal legacy endpoint: %v", err)
				}
				for _, field := range []string{"target_release_ref", "offered_release_ref", "active_release_ref", "capability_digest", "missing_requirements", "unmanaged"} {
					if strings.Contains(string(encoded), field) {
						t.Errorf("legacy endpoint encoded absent field %q: %s", field, encoded)
					}
				}
			},
		},
		{
			name: "modern record preserves distinct delivery state",
			path: "testdata/endpoint_capability_delivery.json",
			check: func(t *testing.T, endpoint Endpoint) {
				t.Helper()
				if endpoint.TargetReleaseRef != "release-target" || endpoint.OfferedReleaseRef != "release-offered" || endpoint.ActiveReleaseRef != "release-active" {
					t.Fatalf("delivery releases were conflated: %#v", endpoint)
				}
				if endpoint.OfferedDigest != "digest-offered" || endpoint.ActiveDigest != "digest-active" {
					t.Fatalf("delivery digests were conflated: %#v", endpoint)
				}
				if endpoint.OfferedSchemaVersion == nil || *endpoint.OfferedSchemaVersion != 1 || endpoint.ActiveSchemaVersion == nil || *endpoint.ActiveSchemaVersion != 0 {
					t.Fatalf("schema versions were not preserved, including schema 0: %#v", endpoint)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			var endpoint Endpoint
			if err := json.Unmarshal(raw, &endpoint); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			test.check(t, endpoint)
		})
	}
}

func TestClient_BootstrapAndAdminCalls(t *testing.T) {
	caCert, caKey, caPEM := testCA(t)
	admin := registry.NewMemory()

	dir := t.TempDir()
	bootstrapFile := dir + "/bootstrap.token"
	bootstrap := server.NewBootstrap(bootstrapFile)
	if err := bootstrap.MaybeInit(admin); err != nil {
		t.Fatal(err)
	}
	tokenBytes, err := os.ReadFile(bootstrapFile)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	srv := server.New(server.Config{
		Admin:     admin,
		Bootstrap: bootstrap,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{testServerCert(t, caCert, caKey)},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    mustPool(t, caPEM),
		MinVersion:   tls.VersionTLS12,
	}
	ts.StartTLS()
	defer ts.Close()

	trustClient, err := NewClient(ts.URL, t.TempDir(), &tls.Config{
		RootCAs:            mustPool(t, caPEM),
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		ServerName:         "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := trustClient.Bootstrap(token)
	if err != nil {
		t.Fatal(err)
	}

	stateDir := t.TempDir()
	if err := opcreds.Save(stateDir, resp.OperatorID, resp.CertPEM, resp.KeyPEM, resp.CAPEM); err != nil {
		t.Fatal(err)
	}

	adminClient, err := NewClientFromState(ts.URL, stateDir)
	if err != nil {
		t.Fatal(err)
	}

	tokResp, err := adminClient.CreateEnrollToken("demo-fleet", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if tokResp.Token == "" {
		t.Fatal("expected enroll token")
	}

	_ = admin.RegisterEndpoint(registry.Endpoint{ID: "ep-remove-test", Fleet: "demo-fleet"})

	eps, err := adminClient.ListEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 || eps[0].ID != "ep-remove-test" {
		t.Fatalf("endpoints = %+v", eps)
	}

	if err := adminClient.RemoveEndpoint("ep-remove-test"); err != nil {
		t.Fatal(err)
	}
	eps, err = adminClient.ListEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 0 {
		t.Fatalf("endpoints after remove = %+v", eps)
	}

	fleetClient, ok := any(adminClient).(interface {
		ListFleets() ([]string, error)
	})
	if !ok {
		t.Fatal("admin client missing ListFleets")
	}
	fleets, err := fleetClient.ListFleets()
	if err != nil {
		t.Fatal(err)
	}
	if len(fleets) != 1 || fleets[0] != "demo-fleet" {
		t.Fatalf("fleets = %+v", fleets)
	}
}

func TestClient_TriggerGitSync(t *testing.T) {
	caCert, caKey, caPEM := testCA(t)
	admin := registry.NewMemory()

	dir := t.TempDir()
	bootstrapFile := dir + "/bootstrap.token"
	bootstrap := server.NewBootstrap(bootstrapFile)
	if err := bootstrap.MaybeInit(admin); err != nil {
		t.Fatal(err)
	}
	tokenBytes, err := os.ReadFile(bootstrapFile)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	synced := false
	srv := server.New(server.Config{
		Admin:     admin,
		Bootstrap: bootstrap,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
		GitSync: func(context.Context) error {
			synced = true
			return nil
		},
	})

	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{testServerCert(t, caCert, caKey)},
		ClientAuth:   tls.VerifyClientCertIfGiven,
		ClientCAs:    mustPool(t, caPEM),
		MinVersion:   tls.VersionTLS12,
	}
	ts.StartTLS()
	defer ts.Close()

	trustClient, err := NewClient(ts.URL, t.TempDir(), &tls.Config{
		RootCAs:    mustPool(t, caPEM),
		MinVersion: tls.VersionTLS12,
		ServerName: "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := trustClient.Bootstrap(token)
	if err != nil {
		t.Fatal(err)
	}

	stateDir := t.TempDir()
	if err := opcreds.Save(stateDir, resp.OperatorID, resp.CertPEM, resp.KeyPEM, resp.CAPEM); err != nil {
		t.Fatal(err)
	}

	adminClient, err := NewClientFromState(ts.URL, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminClient.TriggerGitSync(); err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("expected git sync to run")
	}
}

func testCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Remotr Test CA"},
		NotBefore:             now,
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return cert, key, pemBytes
}

func testServerCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "remotr-server-test"},
		NotBefore:    now,
		NotAfter:     now.AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return tlsCert
}

func mustPool(t *testing.T, caPEM []byte) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("parse ca pem")
	}
	return pool
}
