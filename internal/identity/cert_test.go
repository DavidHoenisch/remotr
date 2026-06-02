package identity

import (
	"crypto/x509"
	"net/url"
	"testing"
)

func TestEndpointIDFromCert(t *testing.T) {
	uri, _ := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	cert := &x509.Certificate{URIs: []*url.URL{uri}}

	got, err := EndpointIDFromCert(cert)
	if err != nil {
		t.Fatal(err)
	}
	if got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("got %q", got)
	}
}
