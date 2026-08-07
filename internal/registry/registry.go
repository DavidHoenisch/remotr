package registry

import (
	"context"
	"errors"
	"time"

	"github.com/DavidHoenisch/remotr/internal/executor"
)

// ErrEndpointNotFound is returned when an endpoint id is unknown.
var ErrEndpointNotFound = errors.New("endpoint not found")

// ErrEndpointExists is returned when registering an endpoint id that is already present.
var ErrEndpointExists = errors.New("endpoint already exists")

// CapabilityDocumentRecord is the latest validated endpoint capability
// evidence retained for readiness and operator reporting.
type CapabilityDocumentRecord struct {
	EndpointID        string
	Digest            string
	CanonicalDocument []byte
	ReceivedAt        time.Time
}

// CapabilityDocuments persists and reads validated endpoint capability
// evidence. The bool result reports whether storage changed.
type CapabilityDocuments interface {
	StoreEndpointCapabilityDocument(ctx context.Context, record CapabilityDocumentRecord) (bool, error)
	GetEndpointCapabilityDocument(ctx context.Context, endpointID string) (CapabilityDocumentRecord, bool, error)
}

// TargetingDocuments atomically persists the complete endpoint inputs used by
// server-side targeting. The bool result reports whether semantic state changed.
type TargetingDocuments interface {
	StoreEndpointTargeting(ctx context.Context, endpointID string, labels map[string]string, usernames []string) (bool, error)
}

// MissingRequirement identifies one exact schema, resource, or provider
// contract preventing target delivery.
type MissingRequirement struct {
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
}

// EndpointDeliveryState separates the current target and last offer from the
// artifact the endpoint has acknowledged processing successfully.
type EndpointDeliveryState struct {
	EndpointID                 string
	TargetReleaseRef           string
	OfferedReleaseRef          string
	OfferedDigest              string
	OfferedSchemaVersion       int
	OfferedAt                  time.Time
	ActiveReleaseRef           string
	ActiveDigest               string
	ActiveSchemaVersion        int
	ActiveAt                   time.Time
	CapabilityBlockedTargetRef string
	MissingRequirements        []MissingRequirement
	Unmanaged                  bool
	UpdatedAt                  time.Time
}

// DeliveryStates persists endpoint target, offered, active, and blocked state.
type DeliveryStates interface {
	StoreEndpointDeliveryState(ctx context.Context, state EndpointDeliveryState) (bool, error)
	GetEndpointDeliveryState(ctx context.Context, endpointID string) (EndpointDeliveryState, bool, error)
}

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
	Failure         executor.SafeError
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
