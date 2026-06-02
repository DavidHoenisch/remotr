package enroll

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/pki"
)

func TestClient_Enroll_success(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/enroll" {
			http.NotFound(w, r)
			return
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Token != "good-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if req.CSRPEM == "" {
			http.Error(w, "csr required", http.StatusBadRequest)
			return
		}

		caCert, caKey := testCA(t)
		signed, err := pki.SignEndpointCSR(caCert, caKey, []byte(req.CSRPEM), "11111111-1111-1111-1111-111111111111")
		if err != nil {
			http.Error(w, "bad csr", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(Response{
			EndpointID: "11111111-1111-1111-1111-111111111111",
			CertPEM:    string(signed.CertPEM),
			CAPEM:      "-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----\n",
		})
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // test server uses ephemeral cert
	})

	resp, err := client.Enroll("good-token", "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if resp.EndpointID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("endpoint id = %q", resp.EndpointID)
	}
	if resp.KeyPEM == "" {
		t.Fatal("expected locally retained key pem")
	}
}

func TestClient_EnrollWithServerKey_success(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.CSRPEM != "" {
			http.Error(w, "unexpected csr", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(Response{
			EndpointID: "11111111-1111-1111-1111-111111111111",
			CertPEM:    "-----BEGIN CERTIFICATE-----\ncert\n-----END CERTIFICATE-----\n",
			KeyPEM:     "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----\n",
			CAPEM:      "-----BEGIN CERTIFICATE-----\nca\n-----END CERTIFICATE-----\n",
		})
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec
	})

	resp, err := client.EnrollWithServerKey("good-token", "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	if resp.KeyPEM == "" {
		t.Fatal("expected server key pem")
	}
}

func TestClient_Enroll_rejectsBadToken(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec
	})

	_, err := client.Enroll("bad", "11111111-1111-1111-1111-111111111111")
	if err == nil {
		t.Fatal("expected error")
	}
}

func testCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
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
	return cert, key
}
