package identity

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
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
	f.Add([]byte{})
	f.Add([]byte("fuzz-seed"))
	f.Add([]byte{0, 0xff, 0x7f})
	f.Fuzz(func(t *testing.T, seed []byte) {
		if len(seed) > 1<<16 {
			return
		}
		cert := &x509.Certificate{Raw: append([]byte(nil), seed...)}
		sum := sha256.Sum256(seed)
		if got, want := Fingerprint(cert), hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("fingerprint = %q, want %q", got, want)
		}
	})
}
