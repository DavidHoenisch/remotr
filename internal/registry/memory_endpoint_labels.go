package registry

import (
	"github.com/DavidHoenisch/remotr/internal/endpointlabel"
)

func (m *Memory) SetEndpointLabel(id, key, value string) (map[string]string, error) {
	if err := endpointlabel.ValidateKey(key); err != nil {
		return nil, err
	}
	if err := endpointlabel.ValidateValue(value); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[id]; !ok {
		return nil, ErrEndpointNotFound
	}
	if m.labels[id] == nil {
		m.labels[id] = make(map[string]string)
	}
	m.labels[id][key] = value
	return copyLabels(m.labels[id]), nil
}

func (m *Memory) DeleteEndpointLabel(id, key string) (bool, error) {
	if err := endpointlabel.ValidateKey(key); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[id]; !ok {
		return false, ErrEndpointNotFound
	}
	labels, ok := m.labels[id]
	if !ok {
		return false, nil
	}
	if _, ok := labels[key]; !ok {
		return false, nil
	}
	delete(labels, key)
	if len(labels) == 0 {
		delete(m.labels, id)
	}
	return true, nil
}
