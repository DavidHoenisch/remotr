package registry

import (
	"errors"
	"time"
)

// ErrEndpointNotFound is returned when an endpoint id is unknown.
var ErrEndpointNotFound = errors.New("endpoint not found")

// ErrEndpointExists is returned when registering an endpoint id that is already present.
var ErrEndpointExists = errors.New("endpoint already exists")

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

// CheckInSummary is the most recent successful sync check-in for an endpoint.
type CheckInSummary struct {
	ReleaseRef string
	Digest     string
	At         time.Time
}

// AgentUpgradeStatus is the last upgrade report from an endpoint on sync.
type AgentUpgradeStatus struct {
	Desired    string
	Phase      string
	Message    string
	ReportedAt time.Time
}

// SystemInfoSummary is the latest machine inventory report for an endpoint.
type SystemInfoSummary struct {
	Digest     string
	ReportedAt time.Time
	ReportJSON []byte
}

// Endpoint is server-side enrollment state (Server registry).
type Endpoint struct {
	ID                    string
	Fleet                 string
	CertFingerprint       string
	Labels                map[string]string
	LastCheckIn           *CheckInSummary
	LastDrift             *DriftSummary
	LastApplyFailure      *ApplyFailureSummary
	SystemInfo            *SystemInfoSummary
	DesiredAgentVersion   string
	DesiredAgentVersionAt time.Time
	ReportedAgentVersion  string
	AgentUpgrade          *AgentUpgradeStatus
	Usernames             []string
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
