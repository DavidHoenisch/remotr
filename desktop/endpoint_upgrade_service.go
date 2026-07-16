package main

import (
	"context"
	"errors"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/agentversion"
	"github.com/DavidHoenisch/remotr/internal/identity"
)

type EndpointUpgradeRequest struct {
	EndpointID string `json:"endpointId"`
	Version    string `json:"version"`
}

type EndpointUpgradeResult struct {
	Status           string   `json:"status"`
	EndpointID       string   `json:"endpointId"`
	Version          string   `json:"version"`
	AffectedEvidence []string `json:"affectedEvidence"`
}

type EndpointUpgradeService struct{}

func NewEndpointUpgradeService() *EndpointUpgradeService {
	return &EndpointUpgradeService{}
}

func (s *EndpointUpgradeService) RequestConnected(ctx context.Context, client *admin.Client, request EndpointUpgradeRequest) (EndpointUpgradeResult, error) {
	if request.EndpointID == "" || strings.TrimSpace(request.EndpointID) != request.EndpointID {
		return EndpointUpgradeResult{}, endpointUpgradeValidationFailure("Select one exact Endpoint before requesting an upgrade.")
	}
	if err := identity.ValidateEndpointID(request.EndpointID); err != nil {
		return EndpointUpgradeResult{}, endpointUpgradeValidationFailure("Select a valid Endpoint from the current inventory.")
	}
	version, err := agentversion.Normalize(request.Version)
	if err != nil {
		return EndpointUpgradeResult{}, endpointUpgradeValidationFailure("Enter a semantic agent version such as v2.2.0.")
	}
	if client == nil {
		return EndpointUpgradeResult{}, ErrSessionNotConnected
	}

	acceptedVersion, err := client.RequestEndpointAgentUpgradeContext(ctx, request.EndpointID, version)
	if err != nil {
		return EndpointUpgradeResult{}, err
	}
	if acceptedVersion != version {
		return EndpointUpgradeResult{}, errors.New("server returned a different Endpoint upgrade version")
	}
	return EndpointUpgradeResult{
		Status:           "requested",
		EndpointID:       request.EndpointID,
		Version:          acceptedVersion,
		AffectedEvidence: []string{"desired_agent_version", "reported_agent_version", "activity"},
	}, nil
}

func endpointUpgradeValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The Endpoint upgrade request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
