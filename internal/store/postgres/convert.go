package postgres

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

const (
	RemediationAuto   = "auto"
	RemediationReport = "report"
)

func endpointFromRow(row db.Endpoint) (registry.Endpoint, error) {
	id, err := uuidString(row.ID)
	if err != nil {
		return registry.Endpoint{}, err
	}
	fp := ""
	if row.CertFingerprint.Valid {
		fp = row.CertFingerprint.String
	}
	return registry.Endpoint{
		ID:              id,
		Fleet:           row.Fleet,
		CertFingerprint: fp,
	}, nil
}

func uuidFromString(s string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid endpoint id %q: %w", s, err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func uuidString(u pgtype.UUID) (string, error) {
	if !u.Valid {
		return "", fmt.Errorf("invalid endpoint id: not set")
	}
	return uuid.UUID(u.Bytes).String(), nil
}

func textFingerprint(fp string) pgtype.Text {
	return pgtype.Text{String: fp, Valid: fp != ""}
}
