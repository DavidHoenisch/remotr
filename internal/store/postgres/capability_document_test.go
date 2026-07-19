package postgres

import (
	"bytes"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

func TestCapabilityDocumentPersistenceBindsAuthenticatedEndpoint(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	receivedAt := time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC)
	querier := &fakeQuerier{
		byID:                map[string]db.Endpoint{endpointID: {ID: endpointID, Fleet: "engineering"}},
		capabilityDocuments: make(map[string]db.EndpointCapabilityDocument),
	}
	store := NewFromQueries(querier)
	record := validCapabilityDocumentRecord(t, endpointID, receivedAt)

	changed, err := store.StoreEndpointCapabilityDocument(t.Context(), record)
	if err != nil || !changed {
		t.Fatalf("StoreEndpointCapabilityDocument() changed=%t err=%v", changed, err)
	}
	got, ok, err := store.GetEndpointCapabilityDocument(t.Context(), endpointID)
	if err != nil || !ok {
		t.Fatalf("GetEndpointCapabilityDocument() ok=%t err=%v", ok, err)
	}
	if got.EndpointID != endpointID || got.Digest != record.Digest || !bytes.Equal(got.CanonicalDocument, record.CanonicalDocument) || !got.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("persisted capability document = %+v", got)
	}
}

func TestCapabilityDocumentPersistenceSkipsUnchangedDigest(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	first := time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC)
	querier := &fakeQuerier{
		byID:                map[string]db.Endpoint{endpointID: {ID: endpointID, Fleet: "engineering"}},
		capabilityDocuments: make(map[string]db.EndpointCapabilityDocument),
	}
	store := NewFromQueries(querier)
	record := validCapabilityDocumentRecord(t, endpointID, first)
	changed, err := store.StoreEndpointCapabilityDocument(t.Context(), record)
	if err != nil || !changed {
		t.Fatalf("first store changed=%t err=%v", changed, err)
	}
	record.ReceivedAt = first.Add(time.Hour)
	changed, err = store.StoreEndpointCapabilityDocument(t.Context(), record)
	if err != nil || changed {
		t.Fatalf("unchanged store changed=%t err=%v", changed, err)
	}
	if querier.capabilityUpserts != 1 {
		t.Fatalf("capability upserts = %d, want 1", querier.capabilityUpserts)
	}
	stored, ok, err := store.GetEndpointCapabilityDocument(t.Context(), endpointID)
	if err != nil || !ok || !stored.ReceivedAt.Equal(first) {
		t.Fatalf("stored unchanged record = %+v, ok=%t err=%v", stored, ok, err)
	}
}

func validCapabilityDocumentRecord(t *testing.T, endpointID string, receivedAt time.Time) registry.CapabilityDocumentRecord {
	t.Helper()
	document := capabilitydoc.Document{
		DocumentVersion:        capabilitydoc.CurrentDocumentVersion,
		ArtifactSchemaVersions: []int{0, 1},
		Capabilities: []capabilitydoc.Capability{
			{ID: "resource:package", Revision: "package-v1"},
		},
		Facts: []capabilitydoc.Fact{
			{Key: "architecture", Value: "x86"},
		},
		AgentVersion: "v1.2.3",
	}
	canonical, err := document.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := document.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	return registry.CapabilityDocumentRecord{
		EndpointID: endpointID, Digest: digest, CanonicalDocument: canonical, ReceivedAt: receivedAt,
	}
}

func TestCapabilityDocumentPersistenceRejectsMalformedStoredState(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{capabilityDocuments: map[string]db.EndpointCapabilityDocument{
		endpointID: {
			EndpointID: endpointID, Digest: "sha256:not-canonical",
			CanonicalDocument: []byte(`{"documentVersion":1,"facts":[{"key":"architecture","value":"secret-canary"}]`),
			ReceivedAt:        pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}}
	store := NewFromQueries(querier)
	if record, ok, err := store.GetEndpointCapabilityDocument(t.Context(), endpointID); err == nil || ok {
		t.Fatalf("malformed stored record = %+v, ok=%t err=%v", record, ok, err)
	}
}
