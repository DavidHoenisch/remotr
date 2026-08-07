package registry

import (
	"context"
	"slices"
)

// StoreEndpointTargeting implements changed-only targeting persistence for
// tests and the in-memory registry.
func (m *Memory) StoreEndpointTargeting(_ context.Context, endpointID string, labels map[string]string, usernames []string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	endpoint, ok := m.byID[endpointID]
	if !ok {
		return false, ErrEndpointNotFound
	}
	if sameLabels(m.labels[endpointID], labels) && slices.Equal(endpoint.Usernames, usernames) {
		return false, nil
	}
	m.labels[endpointID] = copyLabels(labels)
	endpoint.Usernames = append([]string(nil), usernames...)
	m.byID[endpointID] = endpoint
	return true, nil
}

func sameLabels(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
