package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/inventory"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// OS-USF-003: the persistence boundary accepts the legacy digest emitted by a
// real agent even though semantic canonicalization reorders object fields.
func TestSystemInformationPersistenceAcceptsAgentInventoryDigest(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{systemInformation: make(map[string]db.EndpointSystemInfo)}
	store := NewFromQueries(querier)
	snapshot := inventory.Snapshot{
		OSRelease: inventory.OSReleaseInfo{Name: "Pop!_OS", ID: "pop"},
		CPU:       inventory.CPUInfo{ModelName: "Test CPU"},
	}
	report, err := inventory.MarshalJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := inventory.Digest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, digest, report); err != nil || !changed {
		t.Fatalf("agent inventory store changed=%t err=%v", changed, err)
	}
}

func TestSystemInformationPersistenceSkipsEqualSemanticDocument(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{systemInformation: make(map[string]db.EndpointSystemInfo)}
	store := NewFromQueries(querier)
	report := []byte(`{"cpu":{"model":"test"}}`)
	digest := systemInformationDigest(report)
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, digest, report); err != nil || !changed {
		t.Fatalf("initial store changed=%t err=%v", changed, err)
	}
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, digest, []byte(" { \"cpu\" : { \"model\" : \"test\" } } ")); err != nil || changed {
		t.Fatalf("equal semantic store changed=%t err=%v", changed, err)
	}
	if querier.systemInformationUpserts != 1 {
		t.Fatalf("system information updates = %d, want one", querier.systemInformationUpserts)
	}
}

func TestSystemInformationPersistenceChangesAndFailsClosed(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	querier := &fakeQuerier{systemInformation: make(map[string]db.EndpointSystemInfo)}
	store := NewFromQueries(querier)
	first := []byte(`{"cpu":"first"}`)
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, systemInformationDigest(first), first); err != nil || !changed {
		t.Fatalf("first store changed=%t err=%v", changed, err)
	}
	second := []byte(`{"cpu":"second"}`)
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, systemInformationDigest(second), second); err != nil || !changed {
		t.Fatalf("changed store changed=%t err=%v", changed, err)
	}
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, systemInformationDigest(first), second); err == nil || changed {
		t.Fatalf("hash mismatch changed=%t err=%v", changed, err)
	}
	querier.systemInformationErr = errors.New("postgres unavailable")
	third := []byte(`{"cpu":"third"}`)
	if changed, err := store.UpsertEndpointSystemInfo(t.Context(), endpointID, systemInformationDigest(third), third); err == nil || changed {
		t.Fatalf("persistence failure changed=%t err=%v", changed, err)
	}
}

func systemInformationDigest(report []byte) string {
	sum := sha256.Sum256(report)
	return hex.EncodeToString(sum[:])
}
