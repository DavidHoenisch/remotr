package registry

import (
	"context"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

type memDriftReport struct {
	releaseRef string
	digest     string
	reportedAt DriftSummary
	report     StateReportPayload
}

// SetEndpointDriftReport stores full drift report JSON for tests and dev.
func (m *Memory) SetEndpointDriftReport(id string, summary DriftSummary, reportJSON []byte) error {
	report, err := ParseStateReportPayload(reportJSON)
	if err != nil {
		return err
	}
	m.SetEndpointStateReport(id, summary, report)
	return nil
}

// SetEndpointStateReport stores an already-admitted classified report.
func (m *Memory) SetEndpointStateReport(id string, summary DriftSummary, report StateReportPayload) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drift[id] = &summary
	m.driftReports[id] = &memDriftReport{
		releaseRef: summary.ReleaseRef,
		digest:     summary.Digest,
		reportedAt: summary,
		report:     cloneStateReportPayload(report),
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
		report.ApplyFailure = cloneApplyFailureSummary(failure)
	}
	if stored := m.driftReports[id]; stored != nil {
		report.ReleaseRef = stored.releaseRef
		report.Digest = stored.digest
		report.ReportedAt = stored.reportedAt.ReportedAt
		parsed := cloneStateReportPayload(stored.report)
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
			report.ApplyFailure = cloneApplyFailureSummary(failure)
		}
		if stored := m.driftReports[ep.ID]; stored != nil {
			report.ReleaseRef = stored.releaseRef
			report.Digest = stored.digest
			report.ReportedAt = stored.reportedAt.ReportedAt
			parsed := cloneStateReportPayload(stored.report)
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

func cloneStateReportPayload(report StateReportPayload) StateReportPayload {
	clone := report
	clone.Items = append([]StateReportItem(nil), report.Items...)
	for i := range clone.Items {
		clone.Items[i].DesiredSummary = report.Items[i].DesiredSummary.Clone()
		clone.Items[i].ObservedSummary = report.Items[i].ObservedSummary.Clone()
		clone.Items[i].Subresults = append([]StateReportSubresult(nil), report.Items[i].Subresults...)
		for j := range clone.Items[i].Subresults {
			clone.Items[i].Subresults[j].DesiredSummary = report.Items[i].Subresults[j].DesiredSummary.Clone()
			clone.Items[i].Subresults[j].ObservedSummary = report.Items[i].Subresults[j].ObservedSummary.Clone()
		}
	}
	clone.Apply = append([]StateReportApplyItem(nil), report.Apply...)
	for i := range clone.Apply {
		clone.Apply[i].DesiredSummary = report.Apply[i].DesiredSummary.Clone()
		clone.Apply[i].ObservedSummary = report.Apply[i].ObservedSummary.Clone()
		clone.Apply[i].Activation = append([]StateReportActivation(nil), report.Apply[i].Activation...)
		clone.Apply[i].Diagnostics = append([]executor.SafeSummary(nil), report.Apply[i].Diagnostics...)
		for j := range clone.Apply[i].Diagnostics {
			clone.Apply[i].Diagnostics[j] = report.Apply[i].Diagnostics[j].Clone()
		}
	}
	clone.ScheduleRuntime = append([]StateReportScheduleRuntime(nil), report.ScheduleRuntime...)
	return clone
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
