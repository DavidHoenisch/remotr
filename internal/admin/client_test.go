package admin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/server"
)

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
