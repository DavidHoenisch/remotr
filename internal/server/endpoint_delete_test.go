package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

// failingDeleteAdmin simulates a persistence failure when deleting an endpoint.
type failingDeleteAdmin struct {
	*mockAdmin
	err error
}

func (f failingDeleteAdmin) DeleteEndpoint(string) (bool, error) { return false, f.err }

// A failed endpoint delete keeps its generic response but logs the underlying
// persistence error so operators can see why it failed.
func TestDeleteEndpointLogsPersistenceFailure(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	admin.endpoints = []registry.Endpoint{{ID: "stale-endpoint", Fleet: "servers"}}
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	failing := failingDeleteAdmin{mockAdmin: admin, err: errors.New(`violates foreign key constraint "compiled_artifacts_endpoint_id_fkey"`)}

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	srv := New(Config{Admin: failing, CACert: caCert, CAKey: caKey, CACertPEM: caPEM})
	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/endpoints/stale-endpoint", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != "delete failed" {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"delete endpoint", "stale-endpoint", "compiled_artifacts_endpoint_id_fkey"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log missing %q:\n%s", want, logs.String())
		}
	}
}
