package postgres

import (
	"bytes"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestCapabilityDocumentPersistenceBindsAuthenticatedEndpoint(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	receivedAt := time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC)
	canonical := []byte(`{"documentVersion":1,"artifactSchemaVersions":[0,1],"capabilities":[{"id":"resource:package","revision":"package-v1"}],"facts":[{"key":"architecture","value":"x86"}],"agentVersion":"v1.2.3"}`)
	querier := &fakeQuerier{
		byID:                map[string]db.Endpoint{endpointID: {ID: endpointID, Fleet: "engineering"}},
		capabilityDocuments: make(map[string]db.EndpointCapabilityDocument),
	}
	store := NewFromQueries(querier)
	record := registry.CapabilityDocumentRecord{
		EndpointID: endpointID, Digest: "sha256:document", CanonicalDocument: canonical, ReceivedAt: receivedAt,
	}

	changed, err := store.StoreEndpointCapabilityDocument(t.Context(), record)
	if err != nil || !changed {
		t.Fatalf("StoreEndpointCapabilityDocument() changed=%t err=%v", changed, err)
	}
	got, ok, err := store.GetEndpointCapabilityDocument(t.Context(), endpointID)
	if err != nil || !ok {
		t.Fatalf("GetEndpointCapabilityDocument() ok=%t err=%v", ok, err)
	}
	if got.EndpointID != endpointID || got.Digest != record.Digest || !bytes.Equal(got.CanonicalDocument, canonical) || !got.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("persisted capability document = %+v", got)
	}
}
