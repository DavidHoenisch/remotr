package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/artifactrequirements"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

type DeliveryStateQuerier interface {
	UpsertEndpointDeliveryState(context.Context, db.UpsertEndpointDeliveryStateParams) (db.EndpointDeliveryState, error)
	GetEndpointDeliveryState(context.Context, string) (db.EndpointDeliveryState, error)
}

func (s *Store) StoreEndpointDeliveryState(ctx context.Context, state registry.EndpointDeliveryState) (bool, error) {
	endpointID, err := parseEndpointID(state.EndpointID)
	if err != nil {
		return false, err
	}
	if s.deliveryStateQ == nil {
		return false, fmt.Errorf("delivery state persistence is not configured")
	}
	if len(state.MissingRequirements) > artifactrequirements.MaxRequirements {
		return false, fmt.Errorf("delivery state missing requirement count exceeds bound")
	}
	missing, err := json.Marshal(state.MissingRequirements)
	if err != nil {
		return false, err
	}
	_, err = s.deliveryStateQ.UpsertEndpointDeliveryState(ctx, db.UpsertEndpointDeliveryStateParams{
		EndpointID: endpointID, TargetReleaseRef: state.TargetReleaseRef,
		OfferedReleaseRef: state.OfferedReleaseRef, OfferedDigest: state.OfferedDigest,
		OfferedSchemaVersion: int32(state.OfferedSchemaVersion), OfferedAt: timestamp(state.OfferedAt),
		ActiveReleaseRef: state.ActiveReleaseRef, ActiveDigest: state.ActiveDigest,
		ActiveSchemaVersion: int32(state.ActiveSchemaVersion), ActiveAt: timestamp(state.ActiveAt),
		CapabilityBlockedTargetRef: state.CapabilityBlockedTargetRef,
		MissingRequirements:        missing, Unmanaged: state.Unmanaged,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) GetEndpointDeliveryState(ctx context.Context, endpointID string) (registry.EndpointDeliveryState, bool, error) {
	parsed, err := parseEndpointID(endpointID)
	if err != nil {
		return registry.EndpointDeliveryState{}, false, err
	}
	if s.deliveryStateQ == nil {
		return registry.EndpointDeliveryState{}, false, fmt.Errorf("delivery state persistence is not configured")
	}
	row, err := s.deliveryStateQ.GetEndpointDeliveryState(ctx, parsed)
	if err != nil {
		if err == pgx.ErrNoRows {
			return registry.EndpointDeliveryState{}, false, nil
		}
		return registry.EndpointDeliveryState{}, false, err
	}
	var missing []registry.MissingRequirement
	if err := json.Unmarshal(row.MissingRequirements, &missing); err != nil || len(missing) > artifactrequirements.MaxRequirements {
		return registry.EndpointDeliveryState{}, false, fmt.Errorf("invalid stored delivery state missing requirements")
	}
	return registry.EndpointDeliveryState{
		EndpointID: row.EndpointID, TargetReleaseRef: row.TargetReleaseRef,
		OfferedReleaseRef: row.OfferedReleaseRef, OfferedDigest: row.OfferedDigest,
		OfferedSchemaVersion: int(row.OfferedSchemaVersion), OfferedAt: row.OfferedAt.Time,
		ActiveReleaseRef: row.ActiveReleaseRef, ActiveDigest: row.ActiveDigest,
		ActiveSchemaVersion: int(row.ActiveSchemaVersion), ActiveAt: row.ActiveAt.Time,
		CapabilityBlockedTargetRef: row.CapabilityBlockedTargetRef,
		MissingRequirements:        missing, Unmanaged: row.Unmanaged, UpdatedAt: row.UpdatedAt.Time,
	}, true, nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
}
