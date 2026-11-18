package server

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/rbac"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type reservationHarness struct {
	t      *testing.T
	reg    *registry.Memory
	srv    *Server
	caCert *x509.Certificate
	caKey  *rsa.PrivateKey
	opCert *x509.Certificate
}

func newReservationHarness(t *testing.T, mutate ...func(*Config)) *reservationHarness {
	t.Helper()
	caCert, caKey, caPEM := testCAForEnroll(t)
	reg := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert)); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Admin: reg, Enroller: reg, CACert: caCert, CAKey: caKey, CACertPEM: caPEM}
	for _, m := range mutate {
		m(&cfg)
	}
	return &reservationHarness{t: t, reg: reg, srv: New(cfg), caCert: caCert, caKey: caKey, opCert: opCred.Cert}
}

func (h *reservationHarness) operator(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			h.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{h.opCert}}
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *reservationHarness) reserve(fleet, id string) (*httptest.ResponseRecorder, endpointReservationResponse) {
	h.t.Helper()
	rec := h.operator(http.MethodPost, "/v1/admin/endpoint-reservations", map[string]any{"fleet": fleet, "endpoint_id": id})
	var out endpointReservationResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&out); err != nil {
			h.t.Fatal(err)
		}
	}
	return rec, out
}

// OS-ENR-001
func TestEndpointReservation_reservesIDWithBoundToken(t *testing.T) {
	h := newReservationHarness(t)

	rec, out := h.reserve("ubuntu-servers", "gitlab-runner-01")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if out.EndpointID != "gitlab-runner-01" || out.Fleet != "ubuntu-servers" || out.Token == "" || out.ExpiresAt.IsZero() {
		t.Fatalf("response = %+v", out)
	}

	list := h.operator(http.MethodGet, "/v1/admin/endpoints", nil)
	var items []endpointListItem
	if err := json.NewDecoder(list.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "gitlab-runner-01" || items[0].Fleet != "ubuntu-servers" || items[0].EnrollmentState != "reserved" {
		t.Fatalf("endpoints = %+v", items)
	}
}

// OS-ENR-002
func TestEndpointReservation_rejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name, fleet, id string
	}{
		{"empty id", "ubuntu-servers", ""},
		{"too short id", "ubuntu-servers", "abc"},
		{"id with dot", "ubuntu-servers", "runner.example"},
		{"id too long", "ubuntu-servers", "a123456789012345678901234567890123456789012345678901234567890123"},
		{"empty fleet", "", "gitlab-runner-01"},
		{"fleet with slash", "../etc", "gitlab-runner-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newReservationHarness(t)
			rec, _ := h.reserve(tc.fleet, tc.id)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if eps, _ := h.reg.ListEndpoints(); len(eps) != 0 {
				t.Fatalf("endpoints created: %+v", eps)
			}
		})
	}
}

// OS-ENR-004
func TestEndpointReservation_deniesUnauthenticatedAndUnauthorized(t *testing.T) {
	operatorID := "11111111-2222-3333-4444-555555555555"
	h := newReservationHarness(t, func(c *Config) {
		c.RBAC = &mockRBAC{roles: map[string][]string{operatorID: {rbac.RoleReadOnly}}}
	})

	rec, _ := h.reserve("ubuntu-servers", "gitlab-runner-01")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only operator status = %d, body = %s", rec.Code, rec.Body.String())
	}

	body := bytes.NewBufferString(`{"fleet":"ubuntu-servers","endpoint_id":"gitlab-runner-01"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/endpoint-reservations", body)
	anon := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(anon, req)
	if anon.Code != http.StatusUnauthorized && anon.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated status = %d", anon.Code)
	}
	if eps, _ := h.reg.ListEndpoints(); len(eps) != 0 {
		t.Fatalf("endpoints created: %+v", eps)
	}
}
