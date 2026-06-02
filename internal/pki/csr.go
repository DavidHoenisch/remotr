package pki

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

// SignedEndpointCSR is a CSR signed into an endpoint client certificate.
type SignedEndpointCSR struct {
	CertPEM []byte
	Cert    *x509.Certificate
}

// GenerateEndpointCSR creates a local RSA key and enrollment CSR. The CSR uses a
// random CN; the server assigns the endpoint ID and embeds it in the signed cert SAN.
func GenerateEndpointCSR() (keyPEM, csrPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	var cnRand [16]byte
	if _, err := rand.Read(cnRand[:]); err != nil {
		return nil, nil, fmt.Errorf("random cn: %w", err)
	}
	cn := "remotr-enroll-" + hex.EncodeToString(cnRand[:])

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: cn},
	}, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create csr: %w", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return keyPEM, csrPEM, nil
}

// SignEndpointCSR validates csrPEM, signs it with the Remotr CA, and embeds endpointID
// in the certificate SAN as urn:remotr:endpoint:<id>.
func SignEndpointCSR(caCert *x509.Certificate, caKey crypto.PrivateKey, csrPEM []byte, endpointID string) (*SignedEndpointCSR, error) {
	if caCert == nil || caKey == nil {
		return nil, fmt.Errorf("ca cert and key required")
	}
	if err := validateEndpointID(endpointID); err != nil {
		return nil, err
	}

	block, _ := pem.Decode(csrPEM)
	if block == nil {
		return nil, fmt.Errorf("invalid csr pem")
	}
	if block.Type != "CERTIFICATE REQUEST" && block.Type != "NEW CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("invalid csr pem type %q", block.Type)
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse csr: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("csr signature: %w", err)
	}

	pub, ok := csr.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("csr public key must be rsa")
	}
	if pub.N.BitLen() < 2048 {
		return nil, fmt.Errorf("rsa key must be at least 2048 bits")
	}

	for _, u := range csr.URIs {
		if u != nil && strings.HasPrefix(u.String(), "urn:remotr:endpoint:") {
			return nil, fmt.Errorf("csr must not include endpoint urn")
		}
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("serial: %w", err)
	}

	urn, err := url.Parse("urn:remotr:endpoint:" + endpointID)
	if err != nil {
		return nil, fmt.Errorf("endpoint urn: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "remotr-endpoint-" + endpointID,
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, endpointClientValidityDays),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{urn},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, pub, caKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	return &SignedEndpointCSR{
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		Cert:    cert,
	}, nil
}
