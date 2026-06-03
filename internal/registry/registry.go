package registry

import (
	"errors"
	"time"
)

// ErrEndpointNotFound is returned when an endpoint id is unknown.
var ErrEndpointNotFound = errors.New("endpoint not found")

// DriftSummary is the most recent drift report for an endpoint (admin queries).
type DriftSummary struct {
	ReleaseRef string
	Digest     string
	ReportedAt time.Time
}

// ApplyFailureSummary is the most recent apply failure for an endpoint (admin queries).
type ApplyFailureSummary struct {
	ReleaseRef      string
	ResourceAddress string
	Message         string
	ReportedAt      time.Time
}

// AgentUpgradeStatus is the last upgrade report from an endpoint on sync.
type AgentUpgradeStatus struct {
	Desired   string
	Phase     string
	Message   string
	ReportedAt time.Time
}

// Endpoint is server-side enrollment state (Server registry).
type Endpoint struct {
	ID              string
	Fleet           string
	CertFingerprint string
	Labels          map[string]string
	LastDrift       *DriftSummary
	LastApplyFailure *ApplyFailureSummary
	DesiredAgentVersion   string
	DesiredAgentVersionAt time.Time
	ReportedAgentVersion  string
	AgentUpgrade          *AgentUpgradeStatus
}

// Registry resolves authenticated endpoints to fleet assignment.
type Registry interface {
	EndpointByCertFingerprint(fp string) (Endpoint, bool)
	EndpointByID(id string) (Endpoint, bool)
}

// Enroller supports enrollment token exchange and endpoint registration.
type Enroller interface {
	Registry
	RedeemEnrollmentToken(token string) (fleet string, ok bool)
	RegisterEndpoint(e Endpoint) error
}
