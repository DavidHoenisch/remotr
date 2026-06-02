package postgres

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

const (
	RemediationAuto   = "auto"
	RemediationReport = "report"
)

func endpointFromRow(row db.Endpoint) (registry.Endpoint, error) {
	if err := identity.ValidateEndpointID(row.ID); err != nil {
		return registry.Endpoint{}, fmt.Errorf("invalid endpoint id in database: %w", err)
	}
	fp := ""
	if row.CertFingerprint.Valid {
		fp = row.CertFingerprint.String
	}
	return registry.Endpoint{
		ID:              row.ID,
		Fleet:           row.Fleet,
		CertFingerprint: fp,
	}, nil
}

func parseEndpointID(id string) (string, error) {
	if err := identity.ValidateEndpointID(id); err != nil {
		return "", fmt.Errorf("invalid endpoint id %q: %w", id, err)
	}
	return id, nil
}

func textFingerprint(fp string) pgtype.Text {
	return pgtype.Text{String: fp, Valid: fp != ""}
}

func uuidString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid id: not set")
	}
	return uuid.UUID(u.Bytes).String(), nil
}
