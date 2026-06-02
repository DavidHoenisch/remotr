package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
)

func TestIssueEndpointCredential(t *testing.T) {
	caCert, caKey := testCA(t)
	endpointID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	cred, err := IssueEndpointCredential(caCert, caKey, endpointID)
	if err != nil {
		t.Fatal(err)
	}

	gotID, err := identity.EndpointIDFromCert(cred.Cert)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != endpointID {
		t.Fatalf("endpoint id = %q", gotID)
	}

	fp := identity.Fingerprint(cred.Cert)
	if fp == "" {
		t.Fatal("empty fingerprint")
	}
	if len(cred.CertPEM) == 0 || len(cred.KeyPEM) == 0 {
		t.Fatal("expected PEM material")
	}
}

func TestIssueEndpointCredential_rejectsInvalidID(t *testing.T) {
	caCert, caKey := testCA(t)
	for _, id := range []string{"", " ", "bad/id", "has#fragment"} {
		if _, err := IssueEndpointCredential(caCert, caKey, id); err == nil {
			t.Fatalf("expected error for id %q", id)
		}
	}
}

func TestIssueOperatorCredential(t *testing.T) {
	caCert, caKey := testCA(t)
	operatorID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	cred, err := IssueOperatorCredential(caCert, caKey, operatorID)
	if err != nil {
		t.Fatal(err)
	}

	gotID, err := identity.OperatorIDFromCert(cred.Cert)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != operatorID {
		t.Fatalf("operator id = %q", gotID)
	}
	if identity.Fingerprint(cred.Cert) == "" {
		t.Fatal("empty fingerprint")
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
