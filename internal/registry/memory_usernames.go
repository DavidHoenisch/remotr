package registry

import "context"

// UpdateEndpointUsernames implements server.SyncTelemetry for tests.
func (m *Memory) UpdateEndpointUsernames(_ context.Context, endpointID string, usernames []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[endpointID]
	if !ok {
		return ErrEndpointNotFound
	}
	if len(usernames) == 0 {
		e.Usernames = nil
	} else {
		e.Usernames = append([]string(nil), usernames...)
	}
	m.byID[endpointID] = e
	return nil
}
