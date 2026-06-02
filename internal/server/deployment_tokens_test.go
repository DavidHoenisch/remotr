package server

import (
	"bytes"
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

func TestCreateDeploymentToken_viewOnceAndList(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	mem := registry.NewMemory()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = mem.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))

	srv := New(Config{
		Admin:            mem,
		DeploymentTokens: mem,
		CACert:           caCert,
		CAKey:            caKey,
		CACertPEM:        caPEM,
	})

	body, _ := json.Marshal(createDeploymentTokenRequest{
		Label: "prod-agents",
		Fleet: "demo",
		TTL:   int64(time.Hour.Seconds()),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/deployment-tokens", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var created createDeploymentTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Token == "" || created.Label != "prod-agents" {
		t.Fatalf("create response = %+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/admin/deployment-tokens", nil)
	listReq.TLS = req.TLS
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}

	var listed []deploymentTokenItem
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Label != "prod-agents" {
		t.Fatalf("listed = %+v", listed)
	}

	showReq := httptest.NewRequest(http.MethodGet, "/v1/admin/deployment-tokens/prod-agents", nil)
	showReq.TLS = req.TLS
	showRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(showRec, showReq)
	if showRec.Code != http.StatusOK {
		t.Fatalf("show status = %d", showRec.Code)
	}

	revokeReq := httptest.NewRequest(http.MethodDelete, "/v1/admin/deployment-tokens/prod-agents", nil)
	revokeReq.TLS = req.TLS
	revokeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d", revokeRec.Code)
	}
}

func TestEnroll_reusesDeploymentToken(t *testing.T) {
	caCert, caKey, caPEM := testCAForEnroll(t)
	mem := registry.NewMemory()
	_, raw, err := mem.CreateDeploymentToken("agents", "test-fleet", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		Enroller:  mem,
		CACert:    caCert,
		CAKey:     caKey,
		CACertPEM: caPEM,
	})

	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(enrollRequest{Token: raw})
		req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("enroll %d status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}
}
