package registry

import "time"

// DriftSummary is the most recent drift report for an endpoint (admin queries).
type DriftSummary struct {
	ReleaseRef string
	Digest     string
	ReportedAt time.Time
}

// Endpoint is server-side enrollment state (Server registry).
type Endpoint struct {
	ID              string
	Fleet           string
	CertFingerprint string
	Labels          map[string]string
	LastDrift       *DriftSummary
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
