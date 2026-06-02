package pki

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
)

const (
	endpointClientValidityDays  = 825
	operatorClientValidityDays  = 825
)

// EndpointCredential is a newly issued client TLS identity for an endpoint.
type EndpointCredential struct {
	EndpointID string
	CertPEM    []byte
	KeyPEM     []byte
	Cert       *x509.Certificate
}

// OperatorCredential is a newly issued client TLS identity for an operator.
type OperatorCredential struct {
	OperatorID string
	CertPEM    []byte
	KeyPEM     []byte
	Cert       *x509.Certificate
}

// IssueEndpointCredential generates an RSA client key and certificate signed by the Remotr CA.
// The endpoint ID is encoded in the certificate SAN as urn:remotr:endpoint:<id>.
func IssueEndpointCredential(caCert *x509.Certificate, caKey crypto.PrivateKey, endpointID string) (*EndpointCredential, error) {
	if caCert == nil || caKey == nil {
		return nil, fmt.Errorf("ca cert and key required")
	}
	if err := validateEndpointID(endpointID); err != nil {
		return nil, err
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
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

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return &EndpointCredential{
		EndpointID: endpointID,
		CertPEM:    certPEM,
		KeyPEM:     keyPEM,
		Cert:       cert,
	}, nil
}

// IssueOperatorCredential generates an RSA client key and certificate signed by the Remotr CA.
// The operator UUID is encoded in the certificate SAN as urn:remotr:operator:<id>.
func IssueOperatorCredential(caCert *x509.Certificate, caKey crypto.PrivateKey, operatorID string) (*OperatorCredential, error) {
	if caCert == nil || caKey == nil {
		return nil, fmt.Errorf("ca cert and key required")
	}
	if err := validateID(operatorID); err != nil {
		return nil, err
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("serial: %w", err)
	}

	urn, err := url.Parse("urn:remotr:operator:" + operatorID)
	if err != nil {
		return nil, fmt.Errorf("operator urn: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "remotr-operator-" + operatorID,
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, operatorClientValidityDays),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{urn},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return &OperatorCredential{
		OperatorID: operatorID,
		CertPEM:    certPEM,
		KeyPEM:     keyPEM,
		Cert:       cert,
	}, nil
}

func validateEndpointID(id string) error {
	return identity.ValidateEndpointID(id)
}

func validateID(id string) error {
	return identity.ValidateEndpointID(id)
}
