package registry

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/DavidHoenisch/remotr/internal/deploytoken"
	"github.com/DavidHoenisch/remotr/internal/identity"
)

type Memory struct {
	mu                sync.Mutex
	byID              map[string]Endpoint
	byFP              map[string]string
	tokens            map[string]string
	deploymentByID    map[string]*memDeploymentToken
	deploymentByLabel map[string]string
	operators         map[string]struct{}
	operatorIDs       map[string]string
	operatorRoles     map[string][]string
	fleets            map[string]struct{}
	policies          map[string]string
	labels            map[string]map[string]string
	drift             map[string]*DriftSummary
	driftReports      map[string]*memDriftReport
	applyFailures     map[string]*ApplyFailureSummary
}

type memDeploymentToken struct {
	id         string
	label      string
	fleet      string
	secretHash string
	expiresAt  time.Time
	revokedAt  *time.Time
	createdAt  time.Time
	lastUsedAt *time.Time
}

func NewMemory() *Memory {
	return &Memory{
		byID:              make(map[string]Endpoint),
		byFP:              make(map[string]string),
		tokens:            make(map[string]string),
		deploymentByID:    make(map[string]*memDeploymentToken),
		deploymentByLabel: make(map[string]string),
		operators:         make(map[string]struct{}),
		operatorIDs:       make(map[string]string),
		operatorRoles:     make(map[string][]string),
		fleets:            make(map[string]struct{}),
		policies:          make(map[string]string),
		labels:            make(map[string]map[string]string),
		drift:             make(map[string]*DriftSummary),
		driftReports:      make(map[string]*memDriftReport),
		applyFailures:     make(map[string]*ApplyFailureSummary),
	}
}

// AddEnrollmentToken registers a one-time enrollment token for the given fleet (tests and dev).
func (m *Memory) AddEnrollmentToken(token, fleet string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordFleetLocked(fleet)
	m.tokens[token] = fleet
}

func (m *Memory) RedeemEnrollmentToken(token string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fleet, ok := m.tokens[token]; ok {
		delete(m.tokens, token)
		return fleet, true
	}
	return m.redeemDeploymentTokenLocked(token)
}

func (m *Memory) redeemDeploymentTokenLocked(presented string) (string, bool) {
	id, secret, err := deploytoken.Parse(presented)
	if err != nil {
		return "", false
	}
	entry, ok := m.deploymentByID[id.String()]
	if !ok || !memDeploymentTokenActive(entry) {
		return "", false
	}
	if !deploytoken.VerifySecret(entry.secretHash, secret) {
		return "", false
	}
	now := time.Now().UTC()
	entry.lastUsedAt = &now
	return entry.fleet, true
}

func memDeploymentTokenActive(entry *memDeploymentToken) bool {
	if entry.revokedAt != nil {
		return false
	}
	return time.Now().Before(entry.expiresAt)
}

func (m *Memory) RegisterEndpoint(e Endpoint) error {
	if err := identity.ValidateEndpointID(e.ID); err != nil {
		return fmt.Errorf("invalid endpoint id %q: %w", e.ID, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordFleetLocked(e.Fleet)
	m.byID[e.ID] = e
	if e.CertFingerprint != "" {
		m.byFP[e.CertFingerprint] = e.ID
	}
	return nil
}

func (m *Memory) BindCertFingerprint(fp, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byFP[fp] = id
}

func (m *Memory) EndpointByCertFingerprint(fp string) (Endpoint, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byFP[fp]
	if !ok {
		return Endpoint{}, false
	}
	return m.endpointByIDLocked(id)
}

func (m *Memory) EndpointByID(id string) (Endpoint, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.endpointByIDLocked(id)
}

func (m *Memory) endpointByIDLocked(id string) (Endpoint, bool) {
	e, ok := m.byID[id]
	if !ok {
		return Endpoint{}, false
	}
	e.Labels = copyLabels(m.labels[id])
	return e, true
}

// SetRemediationPolicy sets fleet remediation policy for tests and dev.
func (m *Memory) SetRemediationPolicy(fleet, policy string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordFleetLocked(fleet)
	m.policies[fleet] = policy
}

// RemediationPolicy implements server.FleetSettings for in-memory registry.
func (m *Memory) RemediationPolicy(_ context.Context, fleet string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.policies[fleet]; ok {
		return p, nil
	}
	return "auto", nil
}

func (m *Memory) HasOperators() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.operators) > 0
}

func (m *Memory) RegisterOperatorCredential(fp string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operators[fp] = struct{}{}
	return nil
}

func (m *Memory) RegisterOperator(operatorID, fp string, roles []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operators[fp] = struct{}{}
	if operatorID != "" {
		m.operatorIDs[fp] = operatorID
		m.operatorRoles[operatorID] = append([]string(nil), roles...)
	}
	return nil
}

func (m *Memory) IsOperatorCredential(fp string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.operators[fp]
	return ok
}

func (m *Memory) ListOperatorCredentials() ([]OperatorCredential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]OperatorCredential, 0, len(m.operators))
	for fp := range m.operators {
		out = append(out, OperatorCredential{CertFingerprint: fp})
	}
	return out, nil
}

func (m *Memory) ListEndpoints() ([]Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Endpoint, 0, len(m.byID))
	for _, e := range m.byID {
		e.Labels = copyLabels(m.labels[e.ID])
		out = append(out, e)
	}
	return out, nil
}

func (m *Memory) ListFleets() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.fleets))
	for fleet := range m.fleets {
		out = append(out, fleet)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Memory) GetEndpoint(id string) (Endpoint, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[id]
	if !ok {
		return Endpoint{}, false, nil
	}
	e.Labels = copyLabels(m.labels[id])
	e.LastDrift = m.drift[id]
	e.LastApplyFailure = m.applyFailures[id]
	return e, true, nil
}

func (m *Memory) DeleteEndpoint(id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.byID[id]
	if !ok {
		return false, nil
	}
	delete(m.byID, id)
	if e.CertFingerprint != "" {
		delete(m.byFP, e.CertFingerprint)
	}
	delete(m.labels, id)
	delete(m.drift, id)
	delete(m.driftReports, id)
	delete(m.applyFailures, id)
	return true, nil
}

// SetEndpointLabels stores inventory labels for tests and dev.
func (m *Memory) SetEndpointLabels(id string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(labels) == 0 {
		delete(m.labels, id)
		return
	}
	m.labels[id] = copyLabels(labels)
}

// SetEndpointDrift stores the latest drift summary for tests and dev.
func (m *Memory) SetEndpointDrift(id string, drift *DriftSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drift[id] = drift
}

// SetEndpointApplyFailure stores the latest apply failure summary for tests and dev.
func (m *Memory) SetEndpointApplyFailure(id string, failure *ApplyFailureSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if failure == nil {
		delete(m.applyFailures, id)
		return
	}
	m.applyFailures[id] = failure
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordFleetLocked(fleet)
	m.tokens[token] = fleet
	return nil
}

func (m *Memory) CreateDeploymentToken(label, fleet string, expiresAt time.Time) (DeploymentToken, string, error) {
	if err := deploytoken.ValidateLabel(label); err != nil {
		return DeploymentToken{}, "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.deploymentByLabel[label]; exists {
		return DeploymentToken{}, "", ErrDeploymentTokenLabelTaken
	}
	m.recordFleetLocked(fleet)

	raw, id, err := deploytoken.Issue()
	if err != nil {
		return DeploymentToken{}, "", err
	}
	_, secret, err := deploytoken.Parse(raw)
	if err != nil {
		return DeploymentToken{}, "", err
	}
	hash, err := deploytoken.HashSecret(secret)
	if err != nil {
		return DeploymentToken{}, "", err
	}

	now := time.Now().UTC()
	entry := &memDeploymentToken{
		id:         id.String(),
		label:      label,
		fleet:      fleet,
		secretHash: hash,
		expiresAt:  expiresAt,
		createdAt:  now,
	}
	m.deploymentByID[entry.id] = entry
	m.deploymentByLabel[label] = entry.id
	return memDeploymentTokenToRegistry(entry), raw, nil
}

func (m *Memory) ListDeploymentTokens() ([]DeploymentToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]DeploymentToken, 0, len(m.deploymentByID))
	for _, entry := range m.deploymentByID {
		out = append(out, memDeploymentTokenToRegistry(entry))
	}
	return out, nil
}

func (m *Memory) GetDeploymentTokenByLabel(label string) (DeploymentToken, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.deploymentByLabel[label]
	if !ok {
		return DeploymentToken{}, false, nil
	}
	entry, ok := m.deploymentByID[id]
	if !ok {
		return DeploymentToken{}, false, nil
	}
	return memDeploymentTokenToRegistry(entry), true, nil
}

func (m *Memory) RevokeDeploymentToken(label string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.deploymentByLabel[label]
	if !ok {
		return false, nil
	}
	entry, ok := m.deploymentByID[id]
	if !ok || entry.revokedAt != nil {
		return false, nil
	}
	now := time.Now().UTC()
	entry.revokedAt = &now
	return true, nil
}

func memDeploymentTokenToRegistry(entry *memDeploymentToken) DeploymentToken {
	return DeploymentToken{
		ID:         entry.id,
		Label:      entry.label,
		Fleet:      entry.fleet,
		ExpiresAt:  entry.expiresAt,
		CreatedAt:  entry.createdAt,
		RevokedAt:  entry.revokedAt,
		LastUsedAt: entry.lastUsedAt,
	}
}

func (m *Memory) recordFleetLocked(fleet string) {
	if fleet == "" {
		return
	}
	m.fleets[fleet] = struct{}{}
}

var _ Admin = (*Memory)(nil)
var _ DeploymentTokens = (*Memory)(nil)
