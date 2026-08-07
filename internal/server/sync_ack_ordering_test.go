package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/documenthash"
	"github.com/DavidHoenisch/remotr/internal/registry"
)

type failingCapabilityDocuments struct{}

func (failingCapabilityDocuments) StoreEndpointCapabilityDocument(context.Context, registry.CapabilityDocumentRecord) (bool, error) {
	return false, errors.New("postgres unavailable")
}
func (failingCapabilityDocuments) GetEndpointCapabilityDocument(context.Context, string) (registry.CapabilityDocumentRecord, bool, error) {
	return registry.CapabilityDocumentRecord{}, false, nil
}

func TestSyncDoesNotAcknowledgeCapabilityBeforeDurablePersistence(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "modern"}); err != nil {
		t.Fatal(err)
	}
	document, err := (capabilitydoc.Document{
		DocumentVersion: 1, ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{{ID: "resource:package", Revision: "package-v1"}},
		Facts:        []capabilitydoc.Fact{{Key: "architecture", Value: "x86"}}, AgentVersion: "v1.2.3",
	}).WithCanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := document.CanonicalBody()
	hash, _ := documenthash.Digest(documenthash.Capability, canonical)
	body, _ := json.Marshal(map[string]any{
		"agentVersion": document.AgentVersion, "capabilityDocument": document,
		"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Capability: hash}},
	})
	identityURI, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identityURI}}}}
	rec := httptest.NewRecorder()
	New(Config{Registry: reg, CapabilityDocuments: failingCapabilityDocuments{}}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || bytes.Contains(rec.Body.Bytes(), []byte("acceptedDocumentHashes")) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

type failingDeliveryStates struct {
	state      registry.EndpointDeliveryState
	storeCalls int
}

func (f *failingDeliveryStates) StoreEndpointDeliveryState(context.Context, registry.EndpointDeliveryState) (bool, error) {
	f.storeCalls++
	return false, errors.New("postgres unavailable")
}
func (f *failingDeliveryStates) GetEndpointDeliveryState(context.Context, string) (registry.EndpointDeliveryState, bool, error) {
	return f.state, true, nil
}

func TestSyncDoesNotAcknowledgeDeliveryBeforeDurableTransition(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "legacy"}); err != nil {
		t.Fatal(err)
	}
	delivery := &failingDeliveryStates{state: registry.EndpointDeliveryState{
		EndpointID: endpointID, OfferedReleaseRef: "release-offered", OfferedDigest: "digest-offered", OfferedSchemaVersion: 1,
	}}
	canonical, _ := documenthash.CanonicalDelivery("release-offered", "digest-offered")
	hash, _ := documenthash.Digest(documenthash.Delivery, canonical)
	body, _ := json.Marshal(map[string]any{
		"agentVersion": "v0.1.12", "lastReleaseRef": "release-offered", "lastDigest": "digest-offered",
		"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Delivery: hash}},
	})
	identityURI, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identityURI}}}}
	rec := httptest.NewRecorder()
	New(Config{Registry: reg, DeliveryStates: delivery}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || delivery.storeCalls != 1 {
		t.Fatalf("status=%d storeCalls=%d body=%s", rec.Code, delivery.storeCalls, rec.Body.String())
	}
	if delivery.state.ActiveDigest != "" || delivery.state.OfferedDigest != "digest-offered" {
		t.Fatalf("failed transition mutated durable fixture: %+v", delivery.state)
	}
}

type failingTargetingDocuments struct{}

func (failingTargetingDocuments) StoreEndpointTargeting(context.Context, string, map[string]string, []string) (bool, error) {
	return false, errors.New("postgres unavailable")
}

func TestSyncDoesNotAcknowledgeTargetingBeforeDurablePersistence(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	reg := registry.NewMemory()
	if err := reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "legacy"}); err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{"distro": "ubuntu", "arch": "x86"}
	usernames := []string{"alice"}
	canonical, _ := documenthash.CanonicalTargeting(labels, usernames)
	hash, _ := documenthash.Digest(documenthash.Targeting, canonical)
	body, _ := json.Marshal(map[string]any{
		"agentVersion": "v0.1.12", "labels": labels, "usernames": usernames,
		"documentHashes": documenthash.Summary{Version: 1, Documents: map[string]string{documenthash.Targeting: hash}},
	})
	identityURI, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{identityURI}}}}
	rec := httptest.NewRecorder()
	New(Config{Registry: reg, TargetingDocuments: failingTargetingDocuments{}}).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || bytes.Contains(rec.Body.Bytes(), []byte("acceptedDocumentHashes")) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
