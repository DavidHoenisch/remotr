package postgres

import (
	"context"
	"errors"

	"github.com/DavidHoenisch/remotr/internal/registry"
	"github.com/jackc/pgx/v5"
)

type parsedDriftReport struct {
	InCompliance bool
	Items        []registry.StateReportItem
	Apply        []registry.StateReportApplyItem
}

func (s *Store) GetEndpointStateReport(ctx context.Context, id string) (registry.StateReport, bool, error) {
	ep, ok, err := s.GetEndpoint(ctx, id)
	if err != nil {
		return registry.StateReport{}, false, err
	}
	if !ok {
		return registry.StateReport{}, false, nil
	}
	report, err := s.buildStateReport(ctx, ep)
	if err != nil {
		return registry.StateReport{}, false, err
	}
	return report, true, nil
}

func (s *Store) ListFleetStateReports(ctx context.Context, fleet string) (registry.FleetStateReport, error) {
	eps, err := s.ListEndpoints(ctx)
	if err != nil {
		return registry.FleetStateReport{}, err
	}

	out := registry.FleetStateReport{Fleet: fleet}
	for _, ep := range eps {
		if ep.Fleet != fleet {
			continue
		}
		report, err := s.buildStateReport(ctx, ep)
		if err != nil {
			return registry.FleetStateReport{}, err
		}
		out.Endpoints = append(out.Endpoints, report)
		registry.AddToFleetStateSummary(&out.Summary, report.Status)
	}
	return out, nil
}

func (s *Store) buildStateReport(ctx context.Context, ep registry.Endpoint) (registry.StateReport, error) {
	report := registry.StateReport{
		EndpointID: ep.ID,
		Fleet:      ep.Fleet,
		Items:      []registry.StateReportItem{},
	}
	report.Status = registry.ClassifyStateReport(report)
	if ep.LastApplyFailure != nil {
		report.ApplyFailure = ep.LastApplyFailure
	}

	parsedID, err := parseEndpointID(ep.ID)
	if err != nil {
		return registry.StateReport{}, err
	}
	row, err := s.q.GetLatestDriftReport(ctx, parsedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return report, nil
		}
		return registry.StateReport{}, err
	}
	if !row.ReportedAt.Valid {
		return report, nil
	}

	report.ReleaseRef = row.ReleaseRef
	report.Digest = row.Digest
	report.ReportedAt = row.ReportedAt.Time

	parsed, err := parseDriftReportJSON(row.ReportJson)
	if err != nil {
		return registry.StateReport{}, err
	}
	report.InCompliance = parsed.InCompliance
	if len(parsed.Items) > 0 {
		report.Items = parsed.Items
	}
	if len(parsed.Apply) > 0 {
		report.Apply = parsed.Apply
	}
	report.Status = registry.ClassifyStateReport(report)
	return report, nil
}

func parseDriftReportJSON(raw []byte) (parsedDriftReport, error) {
	payload, err := registry.ParseStateReportPayload(raw)
	if err != nil {
		return parsedDriftReport{}, err
	}
	return parsedDriftReport{
		InCompliance: payload.InCompliance,
		Items:        payload.Items,
		Apply:        payload.Apply,
	}, nil
}
