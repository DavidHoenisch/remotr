package registry

import (
	"context"
	"time"
)

type Memory struct {
	byID      map[string]Endpoint
	byFP      map[string]string
	tokens    map[string]string
	operators map[string]struct{}
	policies  map[string]string
	labels    map[string]map[string]string
	drift     map[string]*DriftSummary
}

func NewMemory() *Memory {
	return &Memory{
		byID:      make(map[string]Endpoint),
		byFP:      make(map[string]string),
		tokens:    make(map[string]string),
		operators: make(map[string]struct{}),
		policies:  make(map[string]string),
		labels:    make(map[string]map[string]string),
		drift:     make(map[string]*DriftSummary),
	}
}

// AddEnrollmentToken registers a one-time enrollment token for the given fleet (tests and dev).
func (m *Memory) AddEnrollmentToken(token, fleet string) {
	m.tokens[token] = fleet
}

func (m *Memory) ConsumeEnrollmentToken(token string) (string, bool) {
	fleet, ok := m.tokens[token]
	if !ok {
		return "", false
	}
	delete(m.tokens, token)
	return fleet, true
}

func (m *Memory) RegisterEndpoint(e Endpoint) error {
	m.byID[e.ID] = e
	if e.CertFingerprint != "" {
		m.byFP[e.CertFingerprint] = e.ID
	}
	return nil
}

func (m *Memory) BindCertFingerprint(fp, id string) {
	m.byFP[fp] = id
}

func (m *Memory) EndpointByCertFingerprint(fp string) (Endpoint, bool) {
	id, ok := m.byFP[fp]
	if !ok {
		return Endpoint{}, false
	}
	return m.EndpointByID(id)
}

func (m *Memory) EndpointByID(id string) (Endpoint, bool) {
	e, ok := m.byID[id]
	return e, ok
}

// SetRemediationPolicy sets fleet remediation policy for tests and dev.
func (m *Memory) SetRemediationPolicy(fleet, policy string) {
	m.policies[fleet] = policy
}

// RemediationPolicy implements server.FleetSettings for in-memory registry.
func (m *Memory) RemediationPolicy(_ context.Context, fleet string) (string, error) {
	if p, ok := m.policies[fleet]; ok {
		return p, nil
	}
	return "auto", nil
}

func (m *Memory) HasOperators() bool {
	return len(m.operators) > 0
}

func (m *Memory) RegisterOperatorCredential(fp string) error {
	m.operators[fp] = struct{}{}
	return nil
}

func (m *Memory) IsOperatorCredential(fp string) bool {
	_, ok := m.operators[fp]
	return ok
}

func (m *Memory) ListOperatorCredentials() ([]OperatorCredential, error) {
	out := make([]OperatorCredential, 0, len(m.operators))
	for fp := range m.operators {
		out = append(out, OperatorCredential{CertFingerprint: fp})
	}
	return out, nil
}

func (m *Memory) ListEndpoints() ([]Endpoint, error) {
	out := make([]Endpoint, 0, len(m.byID))
	for _, e := range m.byID {
		e.Labels = copyLabels(m.labels[e.ID])
		out = append(out, e)
	}
	return out, nil
}

func (m *Memory) GetEndpoint(id string) (Endpoint, bool, error) {
	e, ok := m.byID[id]
	if !ok {
		return Endpoint{}, false, nil
	}
	e.Labels = copyLabels(m.labels[id])
	e.LastDrift = m.drift[id]
	return e, true, nil
}

// SetEndpointLabels stores inventory labels for tests and dev.
func (m *Memory) SetEndpointLabels(id string, labels map[string]string) {
	if len(labels) == 0 {
		delete(m.labels, id)
		return
	}
	m.labels[id] = copyLabels(labels)
}

// SetEndpointDrift stores the latest drift summary for tests and dev.
func (m *Memory) SetEndpointDrift(id string, drift *DriftSummary) {
	m.drift[id] = drift
}

func copyLabels(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (m *Memory) CreateEnrollmentToken(token, fleet string, expiresAt time.Time) error {
	m.tokens[token] = fleet
	return nil
}

var _ Admin = (*Memory)(nil)
