package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/safepath"
)

// ServerTLSConfig loads server certificate and optional client CA for mTLS.
func ServerTLSConfig(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load server key pair: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		ClientAuth:   tls.VerifyClientCertIfGiven,
	}

	if clientCAFile != "" {
		pool, err := loadCAPool(clientCAFile)
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
	}

	return cfg, nil
}

// ClientTLSConfig loads client certificate and CA for connecting to the server.
func ClientTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client key pair: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if caFile != "" {
		pool, err := loadCAPool(caFile)
		if err != nil {
			return nil, err
		}
		cfg.RootCAs = pool
	}

	return cfg, nil
}

// TrustOnlyTLSConfig loads CA trust for server-authenticated TLS without a client certificate.
func TrustOnlyTLSConfig(caFile string) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if caFile == "" {
		return cfg, nil
	}
	pool, err := loadCAPool(caFile)
	if err != nil {
		return nil, err
	}
	cfg.RootCAs = pool
	return cfg, nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := safepath.ReadConfigFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("parse ca pem")
	}
	return pool, nil
}
