package identity

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"
)

func FuzzEndpointIDFromCert(f *testing.F) {
	f.Add("11111111-1111-1111-1111-111111111111")
	f.Add("")
	f.Add("../../../etc/passwd")

	f.Fuzz(func(t *testing.T, id string) {
		if len(id) > 512 {
			return
		}
		_, _ = EndpointIDFromCert(nil)

		// url.Parse treats # and ? as reserved; endpoint IDs must not contain them.
		if strings.ContainsAny(id, "#?%") {
			return
		}

		urn, err := url.Parse(endpointURNPrefix + id)
		if err != nil {
			return
		}
		cert := &x509.Certificate{URIs: []*url.URL{urn}}
		got, err := EndpointIDFromCert(cert)
		if err != nil {
			return
		}
		if got != id {
			t.Fatalf("got %q want %q", got, id)
		}
	})
}

func FuzzFingerprintFromCertRoundTrip(f *testing.F) {
	f.Add([]byte("fuzz-seed"))
	f.Fuzz(func(t *testing.T, seed []byte) {
		if len(seed) > 64 {
			return
		}
		cert, err := fuzzMinimalCert(seed)
		if err != nil {
			return
		}
		if Fingerprint(cert) == "" {
			t.Fatal("empty fingerprint")
		}
	})
}

func fuzzMinimalCert(seed []byte) (*x509.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial := new(big.Int).SetBytes(seed)
	if serial.Sign() == 0 {
		serial = big.NewInt(1)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "fuzz"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}
