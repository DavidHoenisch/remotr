package server

import (
	"bytes"
	"context"
	"fmt"

	"github.com/DavidHoenisch/remotr/internal/configcompose"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func (s *Server) deriveBaselineAdoptionPlan(ctx context.Context, fleet string) (configcompose.DerivedFleetPlan, error) {
	if s.cfg.ChangePlanProviders == nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("server-derived Change plan provider selection is unavailable")
	}
	releaseRef := s.releaseRef(ctx)
	artifact, digest, err := resolveFleetDesiredArtifact(ctx, s.cfg.ArtifactStore, s.cfg.ConfigRepoPath, fleet, releaseRef)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("resolve composed Fleet artifact: %w", err)
	}
	state, err := models.ParseState(bytes.NewReader(artifact))
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("parse composed Fleet artifact: %w", err)
	}
	providers, err := s.cfg.ChangePlanProviders.SelectChangePlanProviders(ctx, fleet, releaseRef, state)
	if err != nil {
		return configcompose.DerivedFleetPlan{}, fmt.Errorf("select Change plan providers: %w", err)
	}
	return configcompose.DeriveFleetPlan(ctx, fleet, releaseRef, digest, state, providers, s.cfg.Secrets)
}
