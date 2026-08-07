package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/capabilitydoc"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

const maxCanonicalCapabilityDocumentBytes = 65_536

// StoreEndpointCapabilityDocument persists validated evidence under the
// authenticated endpoint identity supplied by the Sync handler.
func (s *Store) StoreEndpointCapabilityDocument(ctx context.Context, record registry.CapabilityDocumentRecord) (bool, error) {
	if err := identity.ValidateEndpointID(record.EndpointID); err != nil {
		return false, fmt.Errorf("capability document endpoint: %w", err)
	}
	if strings.TrimSpace(record.Digest) == "" || len(record.CanonicalDocument) == 0 || len(record.CanonicalDocument) > maxCanonicalCapabilityDocumentBytes || record.ReceivedAt.IsZero() {
		return false, fmt.Errorf("capability document record is incomplete")
	}
	if _, err := capabilitydoc.DecodeCanonical(record.CanonicalDocument, record.Digest); err != nil {
		return false, fmt.Errorf("capability document record is invalid: %w", err)
	}
	_, err := s.q.UpsertEndpointCapabilityDocument(ctx, db.UpsertEndpointCapabilityDocumentParams{
		EndpointID: record.EndpointID, Digest: record.Digest,
		CanonicalDocument: append([]byte(nil), record.CanonicalDocument...),
		ReceivedAt:        pgtype.Timestamptz{Time: record.ReceivedAt.UTC(), Valid: true},
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) GetEndpointCapabilityDocument(ctx context.Context, endpointID string) (registry.CapabilityDocumentRecord, bool, error) {
	if err := identity.ValidateEndpointID(endpointID); err != nil {
		return registry.CapabilityDocumentRecord{}, false, err
	}
	row, err := s.q.GetEndpointCapabilityDocument(ctx, endpointID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return registry.CapabilityDocumentRecord{}, false, nil
		}
		return registry.CapabilityDocumentRecord{}, false, err
	}
	if _, err := capabilitydoc.DecodeCanonical(row.CanonicalDocument, row.Digest); err != nil {
		return registry.CapabilityDocumentRecord{}, false, fmt.Errorf("invalid stored capability document: %w", err)
	}
	return registry.CapabilityDocumentRecord{
		EndpointID: row.EndpointID, Digest: row.Digest,
		CanonicalDocument: append([]byte(nil), row.CanonicalDocument...),
		ReceivedAt:        row.ReceivedAt.Time.UTC(),
	}, true, nil
}

var _ registry.CapabilityDocuments = (*Store)(nil)
