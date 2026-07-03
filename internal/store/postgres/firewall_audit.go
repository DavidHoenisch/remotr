package postgres

import (
	"context"
	"errors"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) InsertFirewallAuditReport(ctx context.Context, endpointID, digest string, reportJSON []byte) error {
	if len(reportJSON) == 0 {
		return nil
	}
	endpointID, err := parseEndpointID(endpointID)
	if err != nil {
		return err
	}
	return s.q.InsertFirewallAuditReport(ctx, db.InsertFirewallAuditReportParams{
		EndpointID: endpointID,
		Digest:     pgtype.Text{String: digest, Valid: digest != ""},
		ReportJson: reportJSON,
	})
}

func (s *Store) GetEndpointFirewallAudit(ctx context.Context, id string) (registry.FirewallAuditReport, bool, error) {
	ep, ok, err := s.GetEndpoint(ctx, id)
	if err != nil {
		return registry.FirewallAuditReport{}, false, err
	}
	if !ok {
		return registry.FirewallAuditReport{}, false, nil
	}

	parsedID, err := parseEndpointID(ep.ID)
	if err != nil {
		return registry.FirewallAuditReport{}, false, err
	}

	row, err := s.q.GetLatestFirewallAuditReport(ctx, parsedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.FirewallAuditReport{}, false, nil
		}
		return registry.FirewallAuditReport{}, false, err
	}
	if !row.ReportedAt.Valid {
		return registry.FirewallAuditReport{}, false, nil
	}

	report := registry.FirewallAuditReport{
		EndpointID: ep.ID,
		ReportedAt: row.ReportedAt.Time,
	}
	if row.Digest.Valid {
		report.Digest = row.Digest.String
	}
	if row.ReportJson != nil {
		report.Report = row.ReportJson
	}
	return report, true, nil
}
