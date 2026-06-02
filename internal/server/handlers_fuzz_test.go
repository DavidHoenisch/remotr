package server

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

type openEnroller struct {
	reg *registry.Memory
}

func (o *openEnroller) ConsumeEnrollmentToken(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	return "test-fleet", true
}

func (o *openEnroller) RegisterEndpoint(e registry.Endpoint) error {
	return o.reg.RegisterEndpoint(e)
}

func (o *openEnroller) EndpointByID(id string) (registry.Endpoint, bool) {
	return o.reg.EndpointByID(id)
}

func (o *openEnroller) EndpointByCertFingerprint(fp string) (registry.Endpoint, bool) {
	return o.reg.EndpointByCertFingerprint(fp)
}

func fuzzSyncServer(t *testing.T) *Server {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "fleets", "test-fleet")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "desired.yaml"), []byte("configurations: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := registry.NewMemory()
	_ = reg.RegisterEndpoint(registry.Endpoint{
		ID:    "11111111-1111-1111-1111-111111111111",
		Fleet: "test-fleet",
	})
	return New(Config{
		ConfigRepoPath: repo,
		ReleaseRef:     "fuzz",
		Registry:       reg,
	})
}

func FuzzHandleSync(f *testing.F) {
	f.Add([]byte(`{"lastDigest":""}`))
	f.Add([]byte(`{`))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<16 {
			return
		}
		srv := fuzzSyncServer(t)
		uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
		req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}},
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code == 0 {
			t.Fatal("no status code")
		}
	})
}

var (
	fuzzEnrollCAOnce sync.Once
	fuzzEnrollCACert *x509.Certificate
	fuzzEnrollCAKey  *rsa.PrivateKey
	fuzzEnrollCAPEM  []byte
	fuzzEnrollCAErr  error
)

func initFuzzEnrollCA() {
	fuzzEnrollCAOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			fuzzEnrollCAErr = err
			return
		}
		now := time.Now()
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Fuzz Enroll CA"},
			NotBefore:             now,
			NotAfter:              now.AddDate(1, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			fuzzEnrollCAErr = err
			return
		}
		fuzzEnrollCACert, fuzzEnrollCAErr = x509.ParseCertificate(der)
		fuzzEnrollCAKey = key
		fuzzEnrollCAPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	})
}

func FuzzHandleEnroll(f *testing.F) {
	f.Add([]byte(`{"token":"tok"}`))
	f.Add([]byte(`{"token":""}`))
	f.Add([]byte("not json"))

	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<16 {
			return
		}
		initFuzzEnrollCA()
		if fuzzEnrollCAErr != nil {
			t.Fatal(fuzzEnrollCAErr)
		}
		reg := &openEnroller{reg: registry.NewMemory()}
		srv := New(Config{
			Enroller:  reg,
			CACert:    fuzzEnrollCACert,
			CAKey:     fuzzEnrollCAKey,
			CACertPEM: fuzzEnrollCAPEM,
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/enroll", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code == 0 {
			t.Fatal("no status code")
		}
	})
}
