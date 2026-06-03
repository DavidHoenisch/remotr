package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

func TestGetEndpoint_returnsLastApplyFailure(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.endpoints = []registry.Endpoint{{
		ID:    "phalanx-acae925c",
		Fleet: "engineering",
		LastApplyFailure: &registry.ApplyFailureSummary{
			ReleaseRef:      "edf7176",
			ResourceAddress: "base-packages/true",
			Message:         "exit status 1",
			ReportedAt:      time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
		},
	}}

	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints/phalanx-acae925c", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var item endpointDetailItem
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.LastApplyFailure == nil {
		t.Fatal("expected last_apply_failure in response")
	}
	if item.LastApplyFailure.ResourceAddress != "base-packages/true" {
		t.Fatalf("resource_address = %q", item.LastApplyFailure.ResourceAddress)
	}
}
