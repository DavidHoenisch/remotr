package registry

import "time"

// OperatorCredential is server-side operator mTLS registration.
type OperatorCredential struct {
	CertFingerprint string
}

// Admin supports operator bootstrap and admin API registry operations.
type Admin interface {
	HasOperators() bool
	RegisterOperatorCredential(fp string) error
	IsOperatorCredential(fp string) bool
	ListOperatorCredentials() ([]OperatorCredential, error)
	ListEndpoints() ([]Endpoint, error)
	GetEndpoint(id string) (Endpoint, bool, error)
	DeleteEndpoint(id string) (bool, error)
	CreateEnrollmentToken(token, fleet string, expiresAt time.Time) error
	RequestAgentUpgrade(id, version string) error
	RequestFleetAgentUpgrade(fleet, version string) (int, error)
	ClearAgentUpgrade(id string) error
}
