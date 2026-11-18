package registry

import (
	"fmt"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
)

type memBoundToken struct {
	endpointID string
	expiresAt  time.Time
	used       bool
}

var _ EndpointReservations = (*Memory)(nil)

func (m *Memory) ReserveEndpoint(id, fleet, token string, expiresAt time.Time) error {
	if err := identity.ValidateEndpointID(id); err != nil {
		return fmt.Errorf("invalid endpoint id %q: %w", id, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.byID[id]; ok {
		if !existing.Reserved() {
			return ErrEndpointEnrolled
		}
		if existing.Fleet != fleet {
			return ErrReservationFleetConflict
		}
	}
	m.recordFleetLocked(fleet)
	m.revokeBoundTokensLocked(id)
	m.byID[id] = Endpoint{ID: id, Fleet: fleet, EnrollmentState: EnrollmentStateReserved}
	m.boundTokens[token] = &memBoundToken{endpointID: id, expiresAt: expiresAt}
	return nil
}

func (m *Memory) UnreserveEndpoint(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.byID[id]
	if !ok {
		return ErrReservationNotFound
	}
	if !existing.Reserved() {
		return ErrEndpointEnrolled
	}
	m.revokeBoundTokensLocked(id)
	delete(m.byID, id)
	delete(m.labels, id)
	return nil
}

func (m *Memory) revokeBoundTokensLocked(id string) {
	for tok, entry := range m.boundTokens {
		if entry.endpointID == id {
			delete(m.boundTokens, tok)
		}
	}
}

func (m *Memory) EnrollmentTokenBinding(token string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if entry, ok := m.boundTokens[token]; ok {
		return entry.endpointID, true
	}
	return "", false
}

func (m *Memory) ClaimReservedEndpoint(token, id, certFingerprint string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.boundTokens[token]
	if !ok || entry.used || entry.endpointID != id || !time.Now().Before(entry.expiresAt) {
		return "", ErrBoundTokenInvalid
	}
	ep, ok := m.byID[id]
	if !ok || !ep.Reserved() {
		return "", ErrBoundTokenInvalid
	}
	entry.used = true
	ep.EnrollmentState = ""
	ep.CertFingerprint = certFingerprint
	m.byID[id] = ep
	m.byFP[certFingerprint] = id
	return ep.Fleet, nil
}
