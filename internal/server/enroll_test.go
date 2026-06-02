package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type mockEnrollRegistry struct {
	tokens map[string]string
	byID   map[string]registry.Endpoint
}

func newMockEnrollRegistry() *mockEnrollRegistry {
	return &mockEnrollRegistry{
		tokens: map[string]string{"enroll-secret": "test-fleet"},
		byID:   make(map[string]registry.Endpoint),
	}
}

func (m *mockEnrollRegistry) ConsumeEnrollmentToken(token string) (string, bool) {
	fleet, ok := m.tokens[token]
	if !ok {
		return "", false
	}
	delete(m.tokens, token)
	return fleet, true
}

func (m *mockEnrollRegistry) RegisterEndpoint(e registry.Endpoint) error {
	m.byID[e.ID] = e
	return nil
}

func (m *mockEnrollRegistry) EndpointByID(id string) (registry.Endpoint, bool) {
	e, ok := m.byID[id]
	return e, ok
}

func (m *mockEnrollRegistry) EndpointByCertFingerprint(fp string) (registry.Endpoint, bool) {
	for _, e := range m.byID {
		if e.CertFingerprint == fp {
			return e, true
		}
	}
	return registry.Endpoint{}, false
}

func TestEnroll_signsCSRAndRegistersEndpoint(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := newMockEnrollRegistry()

	srv := New(Config{
		Enroller:  reg,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	_, csrPEM, err := generateTestCSR(t)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(enrollRequest{Token: "enroll-secret", CSRPEM: string(csrPEM)})
	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp enrollResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.EndpointID == "" || resp.CertPEM == "" || resp.CAPEM == "" {
		t.Fatalf("incomplete response: %+v", resp)
	}
	if resp.KeyPEM != "" {
		t.Fatalf("csr enroll must not return key_pem, got %+v", resp)
	}

	ep, ok := reg.EndpointByID(resp.EndpointID)
	if !ok {
		t.Fatal("endpoint not registered")
	}
	if ep.Fleet != "test-fleet" {
		t.Fatalf("fleet = %q", ep.Fleet)
	}

	block, _ := pem.Decode([]byte(resp.CertPEM))
	if block == nil {
		t.Fatal("invalid cert pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := identity.EndpointIDFromCert(cert)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != resp.EndpointID {
		t.Fatalf("cert endpoint id = %q", gotID)
	}
	if identity.Fingerprint(cert) != ep.CertFingerprint {
		t.Fatal("fingerprint mismatch")
	}
}

func TestEnroll_rejectsInvalidCSR(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	srv := New(Config{
		Enroller:  newMockEnrollRegistry(),
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	body, _ := json.Marshal(enrollRequest{Token: "enroll-secret", CSRPEM: "not-a-csr"})
	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func generateTestCSR(t *testing.T) (*rsa.PrivateKey, []byte, error) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "remotr-enroll-test"},
	}, key)
	if err != nil {
		return nil, nil, err
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func TestEnroll_issuesCredentialAndRegistersEndpoint(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := newMockEnrollRegistry()

	srv := New(Config{
		Enroller:  reg,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	body, _ := json.Marshal(enrollRequest{Token: "enroll-secret"})
	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp enrollResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.EndpointID == "" || resp.CertPEM == "" || resp.KeyPEM == "" || resp.CAPEM == "" {
		t.Fatalf("incomplete response: %+v", resp)
	}

	ep, ok := reg.EndpointByID(resp.EndpointID)
	if !ok {
		t.Fatal("endpoint not registered")
	}
	if ep.Fleet != "test-fleet" {
		t.Fatalf("fleet = %q", ep.Fleet)
	}
	if ep.CertFingerprint == "" {
		t.Fatal("missing cert fingerprint")
	}

	block, _ := pem.Decode([]byte(resp.CertPEM))
	if block == nil {
		t.Fatal("invalid cert pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := identity.EndpointIDFromCert(cert)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != resp.EndpointID {
		t.Fatalf("cert endpoint id = %q", gotID)
	}
	if identity.Fingerprint(cert) != ep.CertFingerprint {
		t.Fatal("fingerprint mismatch")
	}
}

func TestEnroll_rejectsInvalidToken(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	srv := New(Config{
		Enroller:  newMockEnrollRegistry(),
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	body, _ := json.Marshal(enrollRequest{Token: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestEnroll_rejectsMissingToken(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	srv := New(Config{
		Enroller:  newMockEnrollRegistry(),
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func testCAForEnroll(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, []byte) {
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
