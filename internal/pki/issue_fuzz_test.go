package pki

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
)

var (
	fuzzCAOnce sync.Once
	fuzzCACert *x509.Certificate
	fuzzCAKey  *rsa.PrivateKey
	fuzzCAErr  error
)

func initFuzzCA() {
	fuzzCAOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			fuzzCAErr = err
			return
		}
		now := time.Now()
		tmpl := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Fuzz CA"},
			NotBefore:             now,
			NotAfter:              now.AddDate(1, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			fuzzCAErr = err
			return
		}
		fuzzCACert, fuzzCAErr = x509.ParseCertificate(der)
		fuzzCAKey = key
	})
}

func FuzzIssueEndpointCredential(f *testing.F) {
	f.Add("11111111-1111-1111-1111-111111111111")
	f.Add("")
	f.Add("not-a-uuid-but-valid-string")

	f.Fuzz(func(t *testing.T, endpointID string) {
		if len(endpointID) > 256 {
			return
		}
		initFuzzCA()
		if fuzzCAErr != nil {
			t.Fatal(fuzzCAErr)
		}

		cred, err := IssueEndpointCredential(fuzzCACert, fuzzCAKey, endpointID)
		if err != nil {
			return
		}
		got, err := identity.EndpointIDFromCert(cred.Cert)
		if err != nil {
			t.Fatal(err)
		}
		if got != endpointID {
			t.Fatalf("endpoint id = %q want %q", got, endpointID)
		}
	})
}
