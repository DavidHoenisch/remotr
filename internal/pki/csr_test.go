package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/url"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/identity"
)

func TestGenerateEndpointCSR(t *testing.T) {
	keyPEM, csrPEM, err := GenerateEndpointCSR()
	if err != nil {
		t.Fatal(err)
	}
	if len(keyPEM) == 0 || len(csrPEM) == 0 {
		t.Fatal("expected pem material")
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil || keyBlock.Type != "RSA PRIVATE KEY" {
		t.Fatalf("unexpected key pem type %q", keyBlock.Type)
	}

	csrBlock, _ := pem.Decode(csrPEM)
	if csrBlock == nil || csrBlock.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("unexpected csr pem type %q", csrBlock.Type)
	}
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatal(err)
	}
}

func TestSignEndpointCSR(t *testing.T) {
	caCert, caKey := testCA(t)
	endpointID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	_, csrPEM, err := GenerateEndpointCSR()
	if err != nil {
		t.Fatal(err)
	}

	signed, err := SignEndpointCSR(caCert, caKey, csrPEM, endpointID)
	if err != nil {
		t.Fatal(err)
	}

	gotID, err := identity.EndpointIDFromCert(signed.Cert)
	if err != nil {
		t.Fatal(err)
	}
	if gotID != endpointID {
		t.Fatalf("endpoint id = %q", gotID)
	}
	if identity.Fingerprint(signed.Cert) == "" {
		t.Fatal("empty fingerprint")
	}
	if len(signed.CertPEM) == 0 {
		t.Fatal("expected cert pem")
	}
}

func TestSignEndpointCSR_rejectsInvalidCSR(t *testing.T) {
	caCert, caKey := testCA(t)
	endpointID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	_, csrPEM, err := GenerateEndpointCSR()
	if err != nil {
		t.Fatal(err)
	}

	for _, csrPEM := range [][]byte{
		nil,
		[]byte("not pem"),
		[]byte("-----BEGIN CERTIFICATE-----\nZm9v\n-----END CERTIFICATE-----\n"),
	} {
		if _, err := SignEndpointCSR(caCert, caKey, csrPEM, endpointID); err == nil {
			t.Fatal("expected error for invalid csr pem")
		}
	}

	csrWithURN, err := csrWithEndpointURN(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignEndpointCSR(caCert, caKey, csrWithURN, endpointID); err == nil {
		t.Fatal("expected error for csr with endpoint urn")
	}

	weakKeyCSR, err := csrWithWeakRSAKey(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SignEndpointCSR(caCert, caKey, weakKeyCSR, endpointID); err == nil {
		t.Fatal("expected error for weak rsa key")
	}

	if _, err := SignEndpointCSR(caCert, caKey, csrPEM, ""); err == nil {
		t.Fatal("expected error for empty endpoint id")
	}
}

func csrWithEndpointURN(t *testing.T) ([]byte, error) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	urn, err := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	if err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "bad"},
		URIs:    []*url.URL{urn},
	}, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func csrWithWeakRSAKey(t *testing.T) ([]byte, error) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "weak"},
	}, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}
