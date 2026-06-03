package registry

import (
	"context"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agentversion"
)

func (m *Memory) RequestAgentUpgrade(id, version string) error {
	ver, err := agentversion.Normalize(version)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[id]
	if !ok {
		return ErrEndpointNotFound
	}
	e.DesiredAgentVersion = ver
	e.DesiredAgentVersionAt = time.Now().UTC()
	m.byID[id] = e
	return nil
}

func (m *Memory) RequestFleetAgentUpgrade(fleet, version string) (int, error) {
	ver, err := agentversion.Normalize(version)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id, e := range m.byID {
		if e.Fleet != fleet {
			continue
		}
		e.DesiredAgentVersion = ver
		e.DesiredAgentVersionAt = time.Now().UTC()
		m.byID[id] = e
		n++
	}
	return n, nil
}

func (m *Memory) ClearAgentUpgrade(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[id]
	if !ok {
		return ErrEndpointNotFound
	}
	e.DesiredAgentVersion = ""
	e.DesiredAgentVersionAt = time.Time{}
	m.byID[id] = e
	return nil
}

// UpdateAgentUpgradeReport implements server.SyncTelemetry for tests.
func (m *Memory) UpdateAgentUpgradeReport(_ context.Context, endpointID, reportedVersion, phase, message string, clearDesired bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[endpointID]
	if !ok {
		return ErrEndpointNotFound
	}
	if reportedVersion != "" {
		n, err := agentversion.Normalize(reportedVersion)
		if err != nil {
			return err
		}
		e.ReportedAgentVersion = n
	}
	st := &AgentUpgradeStatus{
		Phase:      phase,
		Message:    message,
		ReportedAt: time.Now().UTC(),
	}
	if e.DesiredAgentVersion != "" {
		st.Desired = e.DesiredAgentVersion
	}
	e.AgentUpgrade = st
	if clearDesired {
		e.DesiredAgentVersion = ""
		e.DesiredAgentVersionAt = time.Time{}
	}
	m.byID[endpointID] = e
	return nil
}
