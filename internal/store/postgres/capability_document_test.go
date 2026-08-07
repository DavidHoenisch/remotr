package postgres

import (
	"bytes"
	"errors"
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

func TestCapabilityDocumentPersistenceRejectsDigestCanonicalMismatchBeforeUpdate(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{
		byID:                map[string]db.Endpoint{endpointID: {ID: endpointID, Fleet: "engineering"}},
		capabilityDocuments: make(map[string]db.EndpointCapabilityDocument),
	}
	store := NewFromQueries(querier)
	record := validCapabilityDocumentRecord(t, endpointID, time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC))
	if changed, err := store.StoreEndpointCapabilityDocument(t.Context(), record); err != nil || !changed {
		t.Fatalf("initial store changed=%t err=%v", changed, err)
	}
	record.CanonicalDocument = []byte(`{"documentVersion":1}`)
	record.ReceivedAt = record.ReceivedAt.Add(time.Hour)
	if changed, err := store.StoreEndpointCapabilityDocument(t.Context(), record); err == nil || changed {
		t.Fatalf("mismatched store changed=%t err=%v", changed, err)
	}
	if querier.capabilityUpserts != 1 {
		t.Fatalf("capability updates = %d, want original insert only", querier.capabilityUpserts)
	}
}

func TestCapabilityDocumentPersistenceUpdatesChangedContent(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{capabilityDocuments: make(map[string]db.EndpointCapabilityDocument)}
	store := NewFromQueries(querier)
	first := validCapabilityDocumentRecord(t, endpointID, time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC))
	if changed, err := store.StoreEndpointCapabilityDocument(t.Context(), first); err != nil || !changed {
		t.Fatalf("first store changed=%t err=%v", changed, err)
	}
	document, err := capabilitydoc.DecodeCanonical(first.CanonicalDocument, first.Digest)
	if err != nil {
		t.Fatal(err)
	}
	document.Facts = []capabilitydoc.Fact{{Key: "architecture", Value: "arm"}}
	canonical, err := document.CanonicalBody()
	if err != nil {
		t.Fatal(err)
	}
	digest, err := document.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	changedRecord := registry.CapabilityDocumentRecord{
		EndpointID: endpointID, Digest: digest, CanonicalDocument: canonical,
		ReceivedAt: first.ReceivedAt.Add(time.Hour),
	}
	if changed, err := store.StoreEndpointCapabilityDocument(t.Context(), changedRecord); err != nil || !changed {
		t.Fatalf("changed store changed=%t err=%v", changed, err)
	}
	if querier.capabilityUpserts != 2 {
		t.Fatalf("capability upserts = %d, want insert plus changed update", querier.capabilityUpserts)
	}
}

func TestCapabilityDocumentPersistenceReportsFailure(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{
		capabilityDocuments: make(map[string]db.EndpointCapabilityDocument),
		capabilityUpsertErr: errors.New("postgres unavailable"),
	}
	store := NewFromQueries(querier)
	if changed, err := store.StoreEndpointCapabilityDocument(t.Context(), validCapabilityDocumentRecord(t, endpointID, time.Now().UTC())); err == nil || changed {
		t.Fatalf("failed store changed=%t err=%v", changed, err)
	}
	if len(querier.capabilityDocuments) != 0 {
		t.Fatalf("failed store persisted %+v", querier.capabilityDocuments)
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
