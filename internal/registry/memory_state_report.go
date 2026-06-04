package registry

import (
	"context"
	"encoding/json"
)

type memDriftReport struct {
	releaseRef string
	digest     string
	reportedAt DriftSummary
	reportJSON []byte
}

// SetEndpointDriftReport stores full drift report JSON for tests and dev.
func (m *Memory) SetEndpointDriftReport(id string, summary DriftSummary, reportJSON []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drift[id] = &summary
	m.driftReports[id] = &memDriftReport{
		releaseRef: summary.ReleaseRef,
		digest:     summary.Digest,
		reportedAt: summary,
		reportJSON: append([]byte(nil), reportJSON...),
	}
}

func (m *Memory) GetEndpointStateReport(_ context.Context, id string) (StateReport, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ep, ok := m.byID[id]
	if !ok {
		return StateReport{}, false, nil
	}
	report := StateReport{
		EndpointID: id,
		Fleet:      ep.Fleet,
		Items:      []StateReportItem{},
	}
	if failure := m.applyFailures[id]; failure != nil {
		report.ApplyFailure = failure
	}
	if stored := m.driftReports[id]; stored != nil {
		report.ReleaseRef = stored.releaseRef
		report.Digest = stored.digest
		report.ReportedAt = stored.reportedAt.ReportedAt
		parsed, err := parseMemoryDriftReportJSON(stored.reportJSON)
		if err != nil {
			return StateReport{}, false, err
		}
		report.InCompliance = parsed.inCompliance
		report.Items = parsed.items
	}
	return report, true, nil
}

func (m *Memory) ListFleetStateReports(_ context.Context, fleet string) (FleetStateReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := FleetStateReport{Fleet: fleet}
	for _, ep := range m.byID {
		if ep.Fleet != fleet {
			continue
		}
		report := StateReport{
			EndpointID: ep.ID,
			Fleet:      ep.Fleet,
			Items:      []StateReportItem{},
		}
		if failure := m.applyFailures[ep.ID]; failure != nil {
			report.ApplyFailure = failure
		}
		if stored := m.driftReports[ep.ID]; stored != nil {
			report.ReleaseRef = stored.releaseRef
			report.Digest = stored.digest
			report.ReportedAt = stored.reportedAt.ReportedAt
			parsed, err := parseMemoryDriftReportJSON(stored.reportJSON)
			if err != nil {
				return FleetStateReport{}, err
			}
			report.InCompliance = parsed.inCompliance
			report.Items = parsed.items
		}
		out.Endpoints = append(out.Endpoints, report)
		out.Summary.Total++
		switch {
		case !report.HasReport():
			out.Summary.NoReport++
		case report.InCompliance:
			out.Summary.Compliant++
		default:
			out.Summary.Drift++
		}
	}
	return out, nil
}

type parsedMemoryDriftReport struct {
	inCompliance bool
	items        []StateReportItem
}

func parseMemoryDriftReportJSON(raw []byte) (parsedMemoryDriftReport, error) {
	if len(raw) == 0 {
		return parsedMemoryDriftReport{inCompliance: true, items: []StateReportItem{}}, nil
	}
	var payload struct {
		InCompliance bool              `json:"inCompliance"`
		Items        []StateReportItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return parsedMemoryDriftReport{}, err
	}
	if payload.Items == nil {
		payload.Items = []StateReportItem{}
	}
	return parsedMemoryDriftReport{
		inCompliance: payload.InCompliance,
		items:        payload.Items,
	}, nil
}
