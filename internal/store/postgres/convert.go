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
	ep := registry.Endpoint{
		ID:              row.ID,
		Fleet:           row.Fleet,
		CertFingerprint: fp,
	}
	if row.DesiredAgentVersion.Valid {
		ep.DesiredAgentVersion = row.DesiredAgentVersion.String
	}
	if row.DesiredAgentVersionAt.Valid {
		ep.DesiredAgentVersionAt = row.DesiredAgentVersionAt.Time
	}
	if row.ReportedAgentVersion.Valid {
		ep.ReportedAgentVersion = row.ReportedAgentVersion.String
	}
	if row.AgentUpgradePhase.Valid || row.AgentUpgradeMessage.Valid || row.AgentUpgradeReportedAt.Valid {
		st := registry.AgentUpgradeStatus{}
		if row.DesiredAgentVersion.Valid {
			st.Desired = row.DesiredAgentVersion.String
		} else if row.ReportedAgentVersion.Valid {
			st.Desired = row.ReportedAgentVersion.String
		}
		if row.AgentUpgradePhase.Valid {
			st.Phase = row.AgentUpgradePhase.String
		}
		if row.AgentUpgradeMessage.Valid {
			st.Message = row.AgentUpgradeMessage.String
		}
		if row.AgentUpgradeReportedAt.Valid {
			st.ReportedAt = row.AgentUpgradeReportedAt.Time
		}
		ep.AgentUpgrade = &st
	}
	return ep, nil
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
