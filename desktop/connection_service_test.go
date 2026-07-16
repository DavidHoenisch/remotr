package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
)

var connectionTestTime = time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)

const connectionBootstrapTokenCanary = "connection-bootstrap-token-canary"

func TestConnectionServiceVerifiesOperatorIdentityWithRealAdminClient(t *testing.T) {
	fixture := newConnectionTLSFixture(t)
	server := fixture.startServer(t, http.StatusOK, `{"operator_id":"operator-from-controlled-server","roles":["operator","auditor"]}`)
	stateDir := fixture.saveClientState(t, "operator-from-certificate", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	service := NewConnectionService()

	got, err := service.Connect(t.Context(), connectionProfileForServer(t, "Production", server.URL, stateDir))
	if err != nil {
		t.Fatalf("connect profile: %v", err)
	}
	want := ConnectionView{
		ProfileName: "Production",
		ServerURL:   server.URL,
		OperatorID:  "operator-from-controlled-server",
		Roles:       []string{"operator", "auditor"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connection view = %#v, want %#v", got, want)
	}
}

func TestConnectionServiceClassifiesAuthenticationFailures(t *testing.T) {
	const forbiddenCanary = "forbidden-response-secret-canary"

	t.Run("missing credentials", func(t *testing.T) {
		stateDir := t.TempDir()
		profile := connectionProfileForServer(t, "Missing", "https://missing.example:8443", stateDir)

		_, err := NewConnectionService().Connect(t.Context(), profile)
		assertConnectionFailure(t, err, ConnectionCredentialsMissing, true, stateDir, forbiddenCanary)
	})

	t.Run("unknown CA", func(t *testing.T) {
		fixture := newConnectionTLSFixture(t)
		server := fixture.startServer(t, http.StatusOK, `{"operator_id":"operator-never-returned","roles":[]}`)
		untrusted := newConnectionTLSFixture(t)
		stateDir := fixture.saveClientState(t, "operator-unknown-ca", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), untrusted.caPEM)

		_, err := NewConnectionService().Connect(t.Context(), connectionProfileForServer(t, "Unknown CA", server.URL, stateDir))
		assertConnectionFailure(t, err, ConnectionUnknownCA, false, stateDir, forbiddenCanary)
	})

	t.Run("expired credential", func(t *testing.T) {
		fixture := newConnectionTLSFixture(t)
		server := fixture.startServer(t, http.StatusOK, `{"operator_id":"operator-never-returned","roles":[]}`)
		stateDir := fixture.saveClientState(t, "operator-expired", connectionTestTime.Add(-2*time.Hour), connectionTestTime.Add(-time.Hour), fixture.caPEM)

		_, err := NewConnectionService().Connect(t.Context(), connectionProfileForServer(t, "Expired", server.URL, stateDir))
		assertConnectionFailure(t, err, ConnectionCredentialExpired, false, stateDir, forbiddenCanary)
	})

	t.Run("unreachable server", func(t *testing.T) {
		fixture := newConnectionTLSFixture(t)
		stateDir := fixture.saveClientState(t, "operator-unreachable", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve unreachable address: %v", err)
		}
		serverURL := "https://" + listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatalf("close unreachable address: %v", err)
		}

		_, err = NewConnectionService().Connect(t.Context(), connectionProfileForServer(t, "Unreachable", serverURL, stateDir))
		assertConnectionFailure(t, err, ConnectionServerUnreachable, false, stateDir, forbiddenCanary)
	})

	t.Run("forbidden identity", func(t *testing.T) {
		fixture := newConnectionTLSFixture(t)
		server := fixture.startServer(t, http.StatusForbidden, forbiddenCanary)
		stateDir := fixture.saveClientState(t, "operator-forbidden", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)

		_, err := NewConnectionService().Connect(t.Context(), connectionProfileForServer(t, "Forbidden", server.URL, stateDir))
		assertConnectionFailure(t, err, ConnectionIdentityForbidden, false, stateDir, forbiddenCanary)
	})
}

func assertConnectionFailure(t *testing.T, err error, kind ConnectionFailureKind, bootstrapAvailable bool, stateDir string, canaries ...string) {
	t.Helper()
	var failure *ConnectionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("connection error = %v, want ConnectionFailure", err)
	}
	if failure.Kind != kind {
		t.Fatalf("connection failure kind = %q, want %q", failure.Kind, kind)
	}
	if strings.TrimSpace(failure.Message) == "" || strings.TrimSpace(failure.Guidance) == "" {
		t.Fatalf("connection failure lacks safe message or guidance: %#v", failure)
	}
	if failure.BootstrapAvailable != bootstrapAvailable {
		t.Fatalf("BootstrapAvailable = %t, want %t", failure.BootstrapAvailable, bootstrapAvailable)
	}
	safeText := failure.Error() + " " + failure.Guidance
	for _, forbidden := range append(canaries, stateDir, filepath.Join(stateDir, "operator.key"), "BEGIN PRIVATE KEY", "BEGIN EC PRIVATE KEY", connectionBootstrapTokenCanary) {
		if forbidden != "" && strings.Contains(safeText, forbidden) {
			t.Errorf("connection failure disclosed forbidden value %q: %s", forbidden, safeText)
		}
	}
}

func connectionProfileForServer(t *testing.T, name, serverURL, stateDir string) ConnectionProfile {
	t.Helper()
	return ConnectionProfile{
		Name:      name,
		ServerURL: serverURL,
		StateDir:  stateDir,
		CAPath:    filepath.Join(stateDir, "ca.crt"),
	}
}

type connectionTLSFixture struct {
	caCertificate *x509.Certificate
	caKey         *ecdsa.PrivateKey
	caPEM         []byte
	serverCert    tls.Certificate
}

func newConnectionTLSFixture(t *testing.T) connectionTLSFixture {
	t.Helper()
	caKey := newECDSAKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Remotr Desktop Test CA"},
		NotBefore:             time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create test CA: %v", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse test CA: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverCertPEM, serverKeyPEM := issueConnectionCertificate(
		t,
		caCertificate,
		caKey,
		big.NewInt(2),
		"remotr-controlled-server",
		time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC),
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		true,
	)
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("load controlled server certificate: %v", err)
	}
	return connectionTLSFixture{
		caCertificate: caCertificate,
		caKey:         caKey,
		caPEM:         caPEM,
		serverCert:    serverCert,
	}
}

func (f connectionTLSFixture) startServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/admin/me" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		_, _ = response.Write([]byte(body))
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{f.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    connectionCertPool(t, f.caPEM),
		MinVersion:   tls.VersionTLS12,
		Time: func() time.Time {
			return connectionTestTime
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func (f connectionTLSFixture) saveClientState(t *testing.T, operatorID string, notBefore, notAfter time.Time, caPEM []byte) string {
	t.Helper()
	certPEM, keyPEM := issueConnectionCertificate(
		t,
		f.caCertificate,
		f.caKey,
		big.NewInt(3),
		operatorID,
		notBefore,
		notAfter,
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		false,
	)
	stateDir := t.TempDir()
	if err := opcreds.Save(stateDir, operatorID, string(certPEM), string(keyPEM), string(caPEM)); err != nil {
		t.Fatalf("save controlled Operator state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "bootstrap.token"), []byte(connectionBootstrapTokenCanary), 0o600); err != nil {
		t.Fatalf("write unrelated token canary: %v", err)
	}
	return stateDir
}

func issueConnectionCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, serial *big.Int, commonName string, notBefore, notAfter time.Time, usages []x509.ExtKeyUsage, server bool) ([]byte, []byte) {
	t.Helper()
	key := newECDSAKey(t)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usages,
	}
	if server {
		template.DNSNames = []string{"localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("issue controlled certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal controlled private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

func newECDSAKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate controlled key: %v", err)
	}
	return key
}

func connectionCertPool(t *testing.T, caPEM []byte) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("parse controlled CA")
	}
	return pool
}
