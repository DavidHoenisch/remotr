package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type fakeDeliveryStateQuerier struct {
	row     db.EndpointDeliveryState
	updates int
}

func (f *fakeDeliveryStateQuerier) UpsertEndpointDeliveryState(_ context.Context, params db.UpsertEndpointDeliveryStateParams) (db.EndpointDeliveryState, error) {
	if f.updates > 0 && sameDeliveryState(f.row, params) {
		return db.EndpointDeliveryState{}, pgx.ErrNoRows
	}
	f.updates++
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

func sameDeliveryState(row db.EndpointDeliveryState, params db.UpsertEndpointDeliveryStateParams) bool {
	return row.TargetReleaseRef == params.TargetReleaseRef && row.OfferedReleaseRef == params.OfferedReleaseRef &&
		row.OfferedDigest == params.OfferedDigest && row.OfferedSchemaVersion == params.OfferedSchemaVersion &&
		row.ActiveReleaseRef == params.ActiveReleaseRef && row.ActiveDigest == params.ActiveDigest &&
		row.ActiveSchemaVersion == params.ActiveSchemaVersion && row.CapabilityBlockedTargetRef == params.CapabilityBlockedTargetRef &&
		string(row.MissingRequirements) == string(params.MissingRequirements) && row.Unmanaged == params.Unmanaged
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
	if changed, err := store.StoreEndpointDeliveryState(t.Context(), state); err != nil || !changed {
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

func TestEndpointDeliveryStateSkipsTimestampOnlyReplay(t *testing.T) {
	queries := &fakeDeliveryStateQuerier{}
	store := NewFromDeliveryStateQueries(queries)
	first := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	state := registry.EndpointDeliveryState{
		EndpointID: "11111111-1111-1111-1111-111111111111", TargetReleaseRef: "release-target",
		OfferedReleaseRef: "release-target", OfferedDigest: "digest-target", OfferedSchemaVersion: 1, OfferedAt: first,
		ActiveReleaseRef: "release-active", ActiveDigest: "digest-active", ActiveSchemaVersion: 1, ActiveAt: first,
	}
	if changed, err := store.StoreEndpointDeliveryState(t.Context(), state); err != nil || !changed {
		t.Fatalf("initial store changed=%t err=%v", changed, err)
	}
	state.OfferedAt = first.Add(time.Hour)
	state.ActiveAt = first.Add(time.Hour)
	state.UpdatedAt = first.Add(time.Hour)
	if changed, err := store.StoreEndpointDeliveryState(t.Context(), state); err != nil || changed {
		t.Fatalf("timestamp replay changed=%t err=%v", changed, err)
	}
	if queries.updates != 1 {
		t.Fatalf("delivery updates = %d, want one", queries.updates)
	}
}

func TestEndpointDeliveryStatePersistsAcknowledgementTransition(t *testing.T) {
	queries := &fakeDeliveryStateQuerier{}
	store := NewFromDeliveryStateQueries(queries)
	state := registry.EndpointDeliveryState{
		EndpointID: "11111111-1111-1111-1111-111111111111", TargetReleaseRef: "release-target",
		OfferedReleaseRef: "release-target", OfferedDigest: "digest-target", OfferedSchemaVersion: 1,
	}
	if changed, err := store.StoreEndpointDeliveryState(t.Context(), state); err != nil || !changed {
		t.Fatalf("offer changed=%t err=%v", changed, err)
	}
	state.ActiveReleaseRef, state.ActiveDigest, state.ActiveSchemaVersion = state.OfferedReleaseRef, state.OfferedDigest, state.OfferedSchemaVersion
	state.OfferedReleaseRef, state.OfferedDigest, state.OfferedSchemaVersion = "", "", 0
	if changed, err := store.StoreEndpointDeliveryState(t.Context(), state); err != nil || !changed {
		t.Fatalf("acknowledgement changed=%t err=%v", changed, err)
	}
	if queries.updates != 2 {
		t.Fatalf("delivery updates = %d, want offer plus acknowledgement", queries.updates)
	}
}
