package registry

import "context"

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
		parsed, err := ParseStateReportPayload(stored.reportJSON)
		if err != nil {
			return StateReport{}, false, err
		}
		report.InCompliance = parsed.InCompliance
		report.Items = parsed.Items
		report.Apply = parsed.Apply
		report.ScheduleRuntime = parsed.ScheduleRuntime
		report.RebootRequired = parsed.RebootRequired
	}
	report.Status = ClassifyStateReport(report)
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
			parsed, err := ParseStateReportPayload(stored.reportJSON)
			if err != nil {
				return FleetStateReport{}, err
			}
			report.InCompliance = parsed.InCompliance
			report.Items = parsed.Items
			report.Apply = parsed.Apply
			report.ScheduleRuntime = parsed.ScheduleRuntime
			report.RebootRequired = parsed.RebootRequired
		}
		out.Endpoints = append(out.Endpoints, report)
		report.Status = ClassifyStateReport(report)
		out.Endpoints[len(out.Endpoints)-1] = report
		AddToFleetStateSummary(&out.Summary, report.Status)
	}
	return out, nil
}

func (m *Memory) SetEndpointFirewallAudit(id string, report *FirewallAuditReport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if report == nil {
		delete(m.firewallAudit, id)
		return
	}
	m.firewallAudit[id] = report
}

func (m *Memory) GetEndpointFirewallAudit(_ context.Context, id string) (FirewallAuditReport, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	report, ok := m.firewallAudit[id]
	if !ok {
		return FirewallAuditReport{}, false, nil
	}
	return *report, true, nil
}
