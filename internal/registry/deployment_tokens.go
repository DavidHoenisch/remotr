package registry

import (
	"errors"
	"time"
)

var ErrDeploymentTokenLabelTaken = errors.New("deployment token label already exists")

// DeploymentToken is server-side metadata for a long-lived enrollment token.
// The raw secret is never stored or returned after creation.
type DeploymentToken struct {
	ID         string
	Label      string
	Fleet      string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

// DeploymentTokens supports CRUD for reusable deployment enrollment tokens.
type DeploymentTokens interface {
	CreateDeploymentToken(label, fleet string, expiresAt time.Time) (DeploymentToken, string, error)
	ListDeploymentTokens() ([]DeploymentToken, error)
	GetDeploymentTokenByLabel(label string) (DeploymentToken, bool, error)
	RevokeDeploymentToken(label string) (bool, error)
}
