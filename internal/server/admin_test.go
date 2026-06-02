package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type mockAdmin struct {
	operators map[string]struct{}
	endpoints []registry.Endpoint
	tokens    map[string]string
}

func newMockAdmin() *mockAdmin {
	return &mockAdmin{
		operators: make(map[string]struct{}),
		tokens:    make(map[string]string),
	}
}

func (m *mockAdmin) HasOperators() bool { return len(m.operators) > 0 }

func (m *mockAdmin) RegisterOperatorCredential(fp string) error {
	m.operators[fp] = struct{}{}
	return nil
}

func (m *mockAdmin) IsOperatorCredential(fp string) bool {
	_, ok := m.operators[fp]
	return ok
}

func (m *mockAdmin) ListOperatorCredentials() ([]registry.OperatorCredential, error) {
	out := make([]registry.OperatorCredential, 0, len(m.operators))
	for fp := range m.operators {
		out = append(out, registry.OperatorCredential{CertFingerprint: fp})
	}
	return out, nil
}

func (m *mockAdmin) ListEndpoints() ([]registry.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockAdmin) GetEndpoint(id string) (registry.Endpoint, bool, error) {
	for _, ep := range m.endpoints {
		if ep.ID == id {
			return ep, true, nil
		}
	}
	return registry.Endpoint{}, false, nil
}

func (m *mockAdmin) CreateEnrollmentToken(token, fleet string, _ time.Time) error {
	m.tokens[token] = fleet
	return nil
}

func (m *mockAdmin) DeleteEndpoint(id string) (bool, error) {
	for i, ep := range m.endpoints {
		if ep.ID == id {
			m.endpoints = append(m.endpoints[:i], m.endpoints[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func TestBootstrap_exchangesTokenForOperatorCredential(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	bootstrap := NewBootstrap("")
	if err := bootstrap.generate(); err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		Admin:     admin,
		Bootstrap: bootstrap,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	body, _ := json.Marshal(bootstrapRequest{Token: bootstrap.token})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/bootstrap", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp bootstrapResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.OperatorID == "" || resp.CertPEM == "" || resp.KeyPEM == "" {
		t.Fatalf("incomplete response: %+v", resp)
	}

	if !admin.HasOperators() {
		t.Fatal("expected operator registered")
	}
	if bootstrap.Valid("anything") {
		t.Fatal("bootstrap token should be invalidated")
	}

	block, _ := pem.Decode([]byte(resp.CertPEM))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := identity.OperatorIDFromCert(cert)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != resp.OperatorID {
		t.Fatalf("operator id = %q", gotID)
	}
	if !admin.IsOperatorCredential(identity.Fingerprint(cert)) {
		t.Fatal("operator fingerprint not registered")
	}
}

func TestBootstrap_rejectsInvalidToken(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	bootstrap := NewBootstrap("")
	_ = bootstrap.generate()

	srv := New(Config{
		Admin:     newMockAdmin(),
		Bootstrap: bootstrap,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	body, _ := json.Marshal(bootstrapRequest{Token: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/bootstrap", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireOperator_rejectsEndpointCert(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()

	endpointCred, err := pki.IssueEndpointCredential(caCert, caKey, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints", nil)
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{endpointCred.Cert},
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequireOperator_allowsRegisteredOperator(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.endpoints = []registry.Endpoint{{ID: "ep-1", Fleet: "demo"}}

	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	fp := identity.Fingerprint(opCred.Cert)
	_ = admin.RegisterOperatorCredential(fp)

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var out []endpointListItem
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "ep-1" {
		t.Fatalf("endpoints = %+v", out)
	}
}

func TestCreateEnrollToken_requiresOperator(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	body, _ := json.Marshal(createEnrollTokenRequest{Fleet: "demo", TTL: 3600})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/enroll-tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp createEnrollTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" || resp.Fleet != "demo" {
		t.Fatalf("response = %+v", resp)
	}
	if admin.tokens[resp.Token] != "demo" {
		t.Fatal("token not stored")
	}
}

func TestDeleteEndpoint_removesRegisteredEndpoint(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.endpoints = []registry.Endpoint{{ID: "ep-remove", Fleet: "demo"}}

	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/endpoints/ep-remove", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(admin.endpoints) != 0 {
		t.Fatalf("endpoints = %+v", admin.endpoints)
	}
}

func TestDeleteEndpoint_notFound(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/endpoints/missing-endpoint", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}
