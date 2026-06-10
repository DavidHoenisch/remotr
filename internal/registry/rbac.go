package registry

import "time"

// Operator is a registered operator identity with assigned RBAC roles.
type Operator struct {
	ID              string
	CertFingerprint string
	Roles           []string
	CreatedAt       time.Time
}
