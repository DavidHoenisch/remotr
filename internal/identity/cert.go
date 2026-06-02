package identity

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	endpointURNPrefix  = "urn:remotr:endpoint:"
	operatorURNPrefix  = "urn:remotr:operator:"
)

// Fingerprint returns the SHA-256 fingerprint of a certificate (hex).
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

// EndpointIDFromCert extracts the endpoint UUID from the client certificate SAN URI.
func EndpointIDFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", fmt.Errorf("no certificate")
	}
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		s := u.String()
		if strings.HasPrefix(s, endpointURNPrefix) {
			id := strings.TrimPrefix(s, endpointURNPrefix)
			if id == "" {
				return "", fmt.Errorf("empty endpoint id in cert")
			}
			return id, nil
		}
	}
	return "", fmt.Errorf("endpoint id not found in certificate")
}

// OperatorIDFromCert extracts the operator UUID from the client certificate SAN URI.
func OperatorIDFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil {
		return "", fmt.Errorf("no certificate")
	}
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		s := u.String()
		if strings.HasPrefix(s, operatorURNPrefix) {
			id := strings.TrimPrefix(s, operatorURNPrefix)
			if id == "" {
				return "", fmt.Errorf("empty operator id in cert")
			}
			return id, nil
		}
	}
	return "", fmt.Errorf("operator id not found in certificate")
}
