package registry

import (
	"context"
	"time"
)

func (m *Memory) StoreEndpointDeliveryState(_ context.Context, state EndpointDeliveryState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[state.EndpointID]; !ok {
		return ErrEndpointNotFound
	}
	state.MissingRequirements = append([]MissingRequirement(nil), state.MissingRequirements...)
	state.UpdatedAt = time.Now().UTC()
	m.deliveryStates[state.EndpointID] = state
	return nil
}

func (m *Memory) GetEndpointDeliveryState(_ context.Context, endpointID string) (EndpointDeliveryState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.deliveryStates[endpointID]
	state.MissingRequirements = append([]MissingRequirement(nil), state.MissingRequirements...)
	return state, ok, nil
}
