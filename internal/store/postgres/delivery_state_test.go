package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type fakeDeliveryStateQuerier struct {
	row db.EndpointDeliveryState
}

func (f *fakeDeliveryStateQuerier) UpsertEndpointDeliveryState(_ context.Context, params db.UpsertEndpointDeliveryStateParams) (db.EndpointDeliveryState, error) {
	f.row = db.EndpointDeliveryState{
		EndpointID: params.EndpointID, TargetReleaseRef: params.TargetReleaseRef,
		OfferedReleaseRef: params.OfferedReleaseRef, OfferedDigest: params.OfferedDigest,
		OfferedSchemaVersion: params.OfferedSchemaVersion, OfferedAt: params.OfferedAt,
		ActiveReleaseRef: params.ActiveReleaseRef, ActiveDigest: params.ActiveDigest,
		ActiveSchemaVersion: params.ActiveSchemaVersion, ActiveAt: params.ActiveAt,
		CapabilityBlockedTargetRef: params.CapabilityBlockedTargetRef,
		MissingRequirements:        append([]byte(nil), params.MissingRequirements...), Unmanaged: params.Unmanaged,
		UpdatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}
	return f.row, nil
}

func (f *fakeDeliveryStateQuerier) GetEndpointDeliveryState(context.Context, string) (db.EndpointDeliveryState, error) {
	return f.row, nil
}

func TestEndpointDeliveryStatePersistsOfferedAndActiveSeparately(t *testing.T) {
	queries := &fakeDeliveryStateQuerier{}
	store := NewFromDeliveryStateQueries(queries)
	state := registry.EndpointDeliveryState{
		EndpointID: "11111111-1111-1111-1111-111111111111", TargetReleaseRef: "release-target",
		OfferedReleaseRef: "release-target", OfferedDigest: "digest-offered", OfferedSchemaVersion: 1,
		ActiveReleaseRef: "release-active", ActiveDigest: "digest-active", ActiveSchemaVersion: 0,
		CapabilityBlockedTargetRef: "release-target",
		MissingRequirements:        []registry.MissingRequirement{{ID: "provider:package/apt", Revision: "1"}},
	}
	if err := store.StoreEndpointDeliveryState(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetEndpointDeliveryState(t.Context(), state.EndpointID)
	if err != nil || !ok {
		t.Fatalf("delivery state ok=%t err=%v", ok, err)
	}
	if got.OfferedDigest != "digest-offered" || got.ActiveDigest != "digest-active" || got.TargetReleaseRef != "release-target" || len(got.MissingRequirements) != 1 {
		t.Fatalf("delivery state = %+v", got)
	}
}
