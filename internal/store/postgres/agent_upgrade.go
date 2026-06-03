package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/DavidHoenisch/remotr/internal/store/postgres/db"
)

// RequestAgentUpgrade taints an endpoint to self-upgrade on next sync.
func (s *Store) RequestAgentUpgrade(ctx context.Context, id, version string) error {
	parsedID, err := parseEndpointID(id)
	if err != nil {
		return err
	}
	ver, err := agentversion.Normalize(version)
	if err != nil {
		return err
	}
	if _, err := s.q.GetEndpointByID(ctx, parsedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.ErrEndpointNotFound
		}
		return err
	}
	_, err = s.q.SetEndpointDesiredAgentVersion(ctx, db.SetEndpointDesiredAgentVersionParams{
		ID:                  parsedID,
		DesiredAgentVersion: pgtype.Text{String: ver, Valid: true},
	})
	return err
}

// RequestFleetAgentUpgrade taints all endpoints in a fleet.
func (s *Store) RequestFleetAgentUpgrade(ctx context.Context, fleet, version string) (int, error) {
	ver, err := agentversion.Normalize(version)
	if err != nil {
		return 0, err
	}
	n, err := s.q.SetFleetDesiredAgentVersion(ctx, db.SetFleetDesiredAgentVersionParams{
		Fleet:               fleet,
		DesiredAgentVersion: pgtype.Text{String: ver, Valid: true},
	})
	return int(n), err
}

// ClearAgentUpgrade removes pending upgrade intent for an endpoint.
func (s *Store) ClearAgentUpgrade(ctx context.Context, id string) error {
	parsedID, err := parseEndpointID(id)
	if err != nil {
		return err
	}
	_, err = s.q.ClearEndpointDesiredAgentVersion(ctx, parsedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return registry.ErrEndpointNotFound
	}
	return err
}

// UpdateAgentUpgradeReport persists agent-reported version and optional status; clears taint when complete.
func (s *Store) UpdateAgentUpgradeReport(ctx context.Context, endpointID, reportedVersion, phase, message string, clearDesired bool) error {
	parsedID, err := parseEndpointID(endpointID)
	if err != nil {
		return err
	}
	row, err := s.q.GetEndpointByID(ctx, parsedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return registry.ErrEndpointNotFound
		}
		return err
	}
	if !clearDesired && reportedVersion != "" && row.DesiredAgentVersion.Valid {
		if agentversion.Match(row.DesiredAgentVersion.String, reportedVersion) && phase != "failed" && phase != "installing" {
			clearDesired = true
		}
	}
	rep := pgtype.Text{}
	if reportedVersion != "" {
		n, err := agentversion.Normalize(reportedVersion)
		if err != nil {
			return err
		}
		rep = pgtype.Text{String: n, Valid: true}
	}
	_, err = s.q.UpdateEndpointAgentUpgradeReport(ctx, db.UpdateEndpointAgentUpgradeReportParams{
		ID:                   parsedID,
		ReportedAgentVersion: rep,
		AgentUpgradePhase:    pgText(phase),
		AgentUpgradeMessage:  pgText(message),
		ClearDesired: clearDesired,
	})
	return err
}

func pgText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
