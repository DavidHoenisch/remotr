package identity

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/safepath"
)

// FingerprintFromCertFile returns the SHA-256 fingerprint of the first PEM certificate in path.
func FingerprintFromCertFile(path string) (string, error) {
	pemBytes, err := safepath.ReadConfigFile(path)
	if err != nil {
		return "", fmt.Errorf("read cert file: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return "", fmt.Errorf("no pem block in %s", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse cert: %w", err)
	}
	return Fingerprint(cert), nil
}
