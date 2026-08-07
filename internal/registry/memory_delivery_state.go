package registry

import (
	"context"
	"time"
)

func (m *Memory) StoreEndpointDeliveryState(_ context.Context, state EndpointDeliveryState) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[state.EndpointID]; !ok {
		return false, ErrEndpointNotFound
	}
	if existing, ok := m.deliveryStates[state.EndpointID]; ok && sameDeliverySemantics(existing, state) {
		return false, nil
	}
	state.MissingRequirements = append([]MissingRequirement(nil), state.MissingRequirements...)
	state.UpdatedAt = time.Now().UTC()
	m.deliveryStates[state.EndpointID] = state
	return true, nil
}

func sameDeliverySemantics(left, right EndpointDeliveryState) bool {
	if left.TargetReleaseRef != right.TargetReleaseRef || left.OfferedReleaseRef != right.OfferedReleaseRef ||
		left.OfferedDigest != right.OfferedDigest || left.OfferedSchemaVersion != right.OfferedSchemaVersion ||
		left.ActiveReleaseRef != right.ActiveReleaseRef || left.ActiveDigest != right.ActiveDigest ||
		left.ActiveSchemaVersion != right.ActiveSchemaVersion || left.CapabilityBlockedTargetRef != right.CapabilityBlockedTargetRef ||
		left.Unmanaged != right.Unmanaged || len(left.MissingRequirements) != len(right.MissingRequirements) {
		return false
	}
	for index := range left.MissingRequirements {
		if left.MissingRequirements[index] != right.MissingRequirements[index] {
			return false
		}
	}
	return true
}

func (m *Memory) GetEndpointDeliveryState(_ context.Context, endpointID string) (EndpointDeliveryState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.deliveryStates[endpointID]
	state.MissingRequirements = append([]MissingRequirement(nil), state.MissingRequirements...)
	return state, ok, nil
}
