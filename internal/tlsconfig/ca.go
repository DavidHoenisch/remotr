package tlsconfig

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/safepath"
)

// LoadCAPEM reads a PEM-encoded CA certificate file.
func LoadCAPEM(path string) ([]byte, *x509.Certificate, error) {
	pemBytes, err := safepath.ReadConfigFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read ca cert: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("parse ca cert pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ca cert: %w", err)
	}
	return pemBytes, cert, nil
}

// LoadCAKeyPair loads the Remotr CA certificate and private key for issuing endpoint credentials.
func LoadCAKeyPair(certFile, keyFile string) (cert *x509.Certificate, key crypto.PrivateKey, certPEM []byte, err error) {
	certPEM, cert, err = LoadCAPEM(certFile)
	if err != nil {
		return nil, nil, nil, err
	}

	keyPEM, err := safepath.ReadConfigFile(keyFile)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read ca key: %w", err)
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, nil, nil, fmt.Errorf("parse ca key pem")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		err = fmt.Errorf("unsupported ca key type %q", block.Type)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse ca key: %w", err)
	}

	return cert, key, certPEM, nil
}
