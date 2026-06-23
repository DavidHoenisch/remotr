package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestAdmin_setEndpointLabel(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.endpoints = []registry.Endpoint{{ID: "laptop-01", Fleet: "dev"}}

	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	body, _ := json.Marshal(map[string]string{"value": "berlin"})
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/endpoints/laptop-01/labels/site", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp endpointLabelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Key != "site" || resp.Value != "berlin" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.Labels["site"] != "berlin" {
		t.Fatalf("labels = %+v", resp.Labels)
	}
}

func TestAdmin_deleteEndpointLabel(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.endpoints = []registry.Endpoint{{
		ID:     "laptop-01",
		Fleet:  "dev",
		Labels: map[string]string{"site": "berlin"},
	}}

	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/endpoints/laptop-01/labels/site", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	ep, ok, _ := admin.GetEndpoint("laptop-01")
	if !ok {
		t.Fatal("endpoint missing")
	}
	if len(ep.Labels) != 0 {
		t.Fatalf("labels = %+v", ep.Labels)
	}
}
