package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/releasecatalog"
)

func TestUpgradeInstructionIsNeverDisclosedBeforeAuthenticatedValidSync(t *testing.T) {
	const endpointID = "88888888-8888-8888-8888-888888888888"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "engineering", ReportedAgentVersion: "v0.5.1", DesiredAgentVersion: "v0.6.8"}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Registry: reg})
	known, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	unknown, _ := url.Parse("urn:remotr:endpoint:99999999-9999-9999-9999-999999999999")
	tests := []struct {
		name   string
		body   string
		uri    *url.URL
		status int
	}{
		{name: "unauthenticated", body: `{}`, status: http.StatusUnauthorized},
		{name: "unauthorized endpoint", body: `{}`, uri: unknown, status: http.StatusForbidden},
		{name: "malformed request", body: `{`, uri: known, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewBufferString(test.body))
			if test.uri != nil {
				req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{test.uri}}}}
			}
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != test.status || bytes.Contains(rec.Body.Bytes(), []byte("agentUpgrade")) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBlockedUpgradeEligibilityFailsClosed(t *testing.T) {
	x86 := capabilitydoc.Document{Facts: []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}}
	arm := capabilitydoc.Document{Facts: []capabilitydoc.Fact{{Key: "architecture", Value: "arm"}}}
	approved := releasecatalog.AgentRelease{
		Version: "v0.6.8", UpgradeEligible: true, Integrity: "sha256-manifest",
		Platforms: []string{"linux"}, Architectures: []string{"x86"},
	}
	tests := []struct {
		name     string
		release  releasecatalog.AgentRelease
		document *capabilitydoc.Document
		want     bool
	}{
		{name: "approved", release: approved, document: &x86, want: true},
		{name: "missing document", release: approved},
		{name: "platform mismatch", release: mutateAgentRelease(approved, func(r *releasecatalog.AgentRelease) { r.Platforms = []string{"darwin"} }), document: &x86},
		{name: "architecture mismatch", release: approved, document: &arm},
		{name: "revoked", release: mutateAgentRelease(approved, func(r *releasecatalog.AgentRelease) { r.Revoked = true }), document: &x86},
		{name: "integrity failure", release: mutateAgentRelease(approved, func(r *releasecatalog.AgentRelease) { r.Integrity = "" }), document: &x86},
		{name: "not approved", release: mutateAgentRelease(approved, func(r *releasecatalog.AgentRelease) { r.UpgradeEligible = false }), document: &x86},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := blockedUpgradeEligible(test.release, test.document); got != test.want {
				t.Fatalf("blockedUpgradeEligible() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestBlockedUpgradeRequiresExplicitKnownRelease(t *testing.T) {
	document := capabilitydoc.Document{Facts: []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}}
	server := New(Config{})
	for _, endpoint := range []registry.Endpoint{
		{ReportedAgentVersion: "v0.5.1"},
		{ReportedAgentVersion: "v0.5.1", DesiredAgentVersion: "not-a-version"},
		{ReportedAgentVersion: "v0.5.1", DesiredAgentVersion: "v9.9.9"},
	} {
		if instruction := server.compatibleBlockedUpgradeInstruction(endpoint, &document); instruction != nil {
			t.Fatalf("endpoint %+v received instruction %+v", endpoint, instruction)
		}
	}
}

func mutateAgentRelease(input releasecatalog.AgentRelease, mutate func(*releasecatalog.AgentRelease)) releasecatalog.AgentRelease {
	mutate(&input)
	return input
}
