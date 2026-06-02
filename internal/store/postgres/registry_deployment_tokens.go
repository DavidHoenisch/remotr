package postgres

import (
	"context"
	"time"

	"github.com/DavidHoenisch/remotr/internal/registry"
)

// RegistryDeploymentTokens adapts Store to registry.DeploymentTokens.
type RegistryDeploymentTokens struct {
	*Store
}

var _ registry.DeploymentTokens = (*RegistryDeploymentTokens)(nil)

func (r *RegistryDeploymentTokens) CreateDeploymentToken(label, fleet string, expiresAt time.Time) (registry.DeploymentToken, string, error) {
	return r.Store.CreateDeploymentToken(context.Background(), label, fleet, expiresAt)
}

func (r *RegistryDeploymentTokens) ListDeploymentTokens() ([]registry.DeploymentToken, error) {
	return r.Store.ListDeploymentTokens(context.Background())
}

func (r *RegistryDeploymentTokens) GetDeploymentTokenByLabel(label string) (registry.DeploymentToken, bool, error) {
	tok, err := r.Store.GetDeploymentTokenByLabel(context.Background(), label)
	if err != nil {
		if err == ErrDeploymentTokenNotFound {
			return registry.DeploymentToken{}, false, nil
		}
		return registry.DeploymentToken{}, false, err
	}
	return tok, true, nil
}

func (r *RegistryDeploymentTokens) RevokeDeploymentToken(label string) (bool, error) {
	return r.Store.RevokeDeploymentToken(context.Background(), label)
}
