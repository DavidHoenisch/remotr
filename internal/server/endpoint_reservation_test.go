package server

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func (h *reservationHarness) enroll(token, requestedID string) (*httptest.ResponseRecorder, enrollResponse) {
	h.t.Helper()
	_, csrPEM, err := generateTestCSR(h.t)
	if err != nil {
		h.t.Fatal(err)
	}
	body, _ := json.Marshal(enrollRequest{Token: token, CSRPEM: string(csrPEM), EndpointID: requestedID})
	req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	var out enrollResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&out); err != nil {
			h.t.Fatal(err)
		}
	}
	return rec, out
}

// OS-ENR-005
func TestEndpointReservation_boundEnrollmentClaimsReservedID(t *testing.T) {
	h := newReservationHarness(t)
	_, res := h.reserve("ubuntu-servers", "gitlab-runner-01")

	rec, out := h.enroll(res.Token, "myhost-1a2b3c4d")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if out.EndpointID != "gitlab-runner-01" {
		t.Fatalf("endpoint_id = %q, want bound gitlab-runner-01", out.EndpointID)
	}
	block, _ := pem.Decode([]byte(out.CertPEM))
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := identity.EndpointIDFromCert(cert); got != "gitlab-runner-01" {
		t.Fatalf("cert endpoint id = %q", got)
	}
	ep, ok := h.reg.EndpointByCertFingerprint(identity.Fingerprint(cert))
	if !ok || ep.ID != "gitlab-runner-01" || ep.Fleet != "ubuntu-servers" || ep.Reserved() {
		t.Fatalf("endpoint by cert = %+v, %v", ep, ok)
	}
	if _, ok := h.reg.EndpointByID("myhost-1a2b3c4d"); ok {
		t.Fatal("agent-supplied id must be ignored")
	}
}

func (h *reservationHarness) assertReserved(id, fleet string) {
	h.t.Helper()
	ep, ok := h.reg.EndpointByID(id)
	if !ok || !ep.Reserved() || ep.Fleet != fleet || ep.CertFingerprint != "" {
		h.t.Fatalf("endpoint %s = %+v (found %v), want reserved in %s", id, ep, ok, fleet)
	}
}

// OS-ENR-006
func TestEndpointReservation_boundTokenSingleUseAndExpiry(t *testing.T) {
	t.Run("reused", func(t *testing.T) {
		h := newReservationHarness(t)
		_, res := h.reserve("ubuntu-servers", "gitlab-runner-01")
		if rec, _ := h.enroll(res.Token, ""); rec.Code != http.StatusOK {
			t.Fatalf("first enroll = %d", rec.Code)
		}
		before, _ := h.reg.EndpointByID("gitlab-runner-01")
		if rec, _ := h.enroll(res.Token, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("reuse status = %d", rec.Code)
		}
		after, _ := h.reg.EndpointByID("gitlab-runner-01")
		if after.CertFingerprint != before.CertFingerprint || after.Reserved() {
			t.Fatalf("credentials changed: %+v -> %+v", before, after)
		}
	})
	t.Run("expired", func(t *testing.T) {
		h := newReservationHarness(t)
		if err := h.reg.ReserveEndpoint("gitlab-runner-01", "ubuntu-servers", "expired-token", time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if rec, _ := h.enroll("expired-token", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
		h.assertReserved("gitlab-runner-01", "ubuntu-servers")
	})
	t.Run("revoked by reissue", func(t *testing.T) {
		h := newReservationHarness(t)
		_, first := h.reserve("ubuntu-servers", "gitlab-runner-01")
		_, second := h.reserve("ubuntu-servers", "gitlab-runner-01")
		if rec, _ := h.enroll(first.Token, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoked token status = %d", rec.Code)
		}
		h.assertReserved("gitlab-runner-01", "ubuntu-servers")
		// OS-ENR-011: the re-issued token still enrolls.
		if rec, out := h.enroll(second.Token, ""); rec.Code != http.StatusOK || out.EndpointID != "gitlab-runner-01" {
			t.Fatalf("reissued token status = %d, id = %q", rec.Code, out.EndpointID)
		}
	})
}

// OS-ENR-007
func TestEndpointReservation_reservedIDNotClaimableByOtherCredentials(t *testing.T) {
	h := newReservationHarness(t)
	h.reserve("ubuntu-servers", "gitlab-runner-01")
	_, other := h.reserve("ubuntu-servers", "gitlab-runner-02")
	h.reg.AddEnrollmentToken("unbound-token", "ubuntu-servers")
	_, deploy, err := h.reg.CreateDeploymentToken("ci", "ubuntu-servers", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	for name, token := range map[string]string{"unbound": "unbound-token", "deployment": deploy} {
		if rec, _ := h.enroll(token, "gitlab-runner-01"); rec.Code != http.StatusConflict {
			t.Fatalf("%s token status = %d, body = %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec, out := h.enroll(other.Token, "gitlab-runner-01"); rec.Code != http.StatusOK || out.EndpointID != "gitlab-runner-02" {
		t.Fatalf("token bound to other id: status = %d, id = %q", rec.Code, out.EndpointID)
	}
	h.assertReserved("gitlab-runner-01", "ubuntu-servers")
}

// OS-ENR-003, OS-ENR-012
func TestEndpointReservation_conflicts(t *testing.T) {
	h := newReservationHarness(t)
	_, res := h.reserve("ubuntu-servers", "gitlab-runner-01")

	if rec, _ := h.reserve("workstations", "gitlab-runner-01"); rec.Code != http.StatusConflict {
		t.Fatalf("cross-fleet status = %d", rec.Code)
	}
	h.assertReserved("gitlab-runner-01", "ubuntu-servers")
	if rec, out := h.enroll(res.Token, ""); rec.Code != http.StatusOK || out.EndpointID != "gitlab-runner-01" {
		t.Fatalf("original token must survive cross-fleet conflict: %d", rec.Code)
	}

	before, _ := h.reg.EndpointByID("gitlab-runner-01")
	if rec, _ := h.reserve("ubuntu-servers", "gitlab-runner-01"); rec.Code != http.StatusConflict {
		t.Fatalf("enrolled status = %d", rec.Code)
	}
	after, _ := h.reg.EndpointByID("gitlab-runner-01")
	if after.Reserved() || after.CertFingerprint != before.CertFingerprint {
		t.Fatalf("enrolled endpoint changed: %+v", after)
	}
}

func (h *reservationHarness) unreserve(id string, force bool) *httptest.ResponseRecorder {
	h.t.Helper()
	path := "/v1/admin/endpoint-reservations/" + id
	if force {
		path += "?force=true"
	}
	return h.operator(http.MethodDelete, path, nil)
}

func configRepoWithOverride(t *testing.T, id string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "endpoints", id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dir, "manifest.yaml"): "kind: manifest\nmodules:\n  - modules/base-packages.yaml\n",
		filepath.Join(root, "modules", "base-packages.yaml"): `kind: module
schemaVersion: 1
configurations:
  - name: base-packages
    targetDistros: [Debian]
    targetArch: [x86]
    resources:
      - kind: package
        name: curl
        lifecycle: present
        present: true
`,
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// OS-ENR-013, OS-ENR-014
func TestEndpointReservation_unreserve(t *testing.T) {
	h := newReservationHarness(t)
	_, res := h.reserve("ubuntu-servers", "gitlab-runner-01")
	if rec := h.unreserve("gitlab-runner-01", false); rec.Code != http.StatusOK {
		t.Fatalf("unreserve status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := h.reg.EndpointByID("gitlab-runner-01"); ok {
		t.Fatal("reserved endpoint not removed")
	}
	if rec, _ := h.enroll(res.Token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("released token status = %d", rec.Code)
	}

	_, res2 := h.reserve("ubuntu-servers", "gitlab-runner-02")
	h.enroll(res2.Token, "")
	if rec := h.unreserve("gitlab-runner-02", true); rec.Code != http.StatusConflict {
		t.Fatalf("enrolled unreserve status = %d", rec.Code)
	}
	if ep, ok := h.reg.EndpointByID("gitlab-runner-02"); !ok || ep.Reserved() {
		t.Fatalf("enrolled endpoint changed: %+v", ep)
	}
	if rec := h.unreserve("never-reserved", false); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown unreserve status = %d", rec.Code)
	}
}

// OS-ENR-017, OS-ENR-018
func TestEndpointReservation_unreserveWithReleasedOverride(t *testing.T) {
	repo := configRepoWithOverride(t, "gitlab-runner-01")
	h := newReservationHarness(t, func(c *Config) {
		c.ConfigRepoPath = repo
		c.ReleaseRef = "rel-abc123"
	})
	_, res := h.reserve("ubuntu-servers", "gitlab-runner-01")

	rec := h.unreserve("gitlab-runner-01", false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	msg := rec.Body.String()
	for _, want := range []string{"gitlab-runner-01", "rel-abc123", "endpoints/gitlab-runner-01/", "block Git sync", "--force"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	h.assertReserved("gitlab-runner-01", "ubuntu-servers")

	forced := h.unreserve("gitlab-runner-01", true)
	if forced.Code != http.StatusOK {
		t.Fatalf("forced status = %d, body = %s", forced.Code, forced.Body.String())
	}
	var out endpointUnreserveResponse
	if err := json.NewDecoder(forced.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Warning, "endpoints/gitlab-runner-01/") || !strings.Contains(out.Warning, "block Git sync") {
		t.Fatalf("warning = %q", out.Warning)
	}
	if _, ok := h.reg.EndpointByID("gitlab-runner-01"); ok {
		t.Fatal("forced unreserve did not remove endpoint")
	}
	if rec, _ := h.enroll(res.Token, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("released token status = %d", rec.Code)
	}
}
