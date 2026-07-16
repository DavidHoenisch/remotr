package main

import (
	"context"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/identity"
)

type EndpointRemovalRequest struct {
	EndpointID   string `json:"endpointId"`
	Confirmation string `json:"confirmation"`
}

type EndpointRemovalResult struct {
	Status           string   `json:"status"`
	EndpointID       string   `json:"endpointId"`
	CredentialStatus string   `json:"credentialStatus"`
	AffectedEvidence []string `json:"affectedEvidence"`
}

type EndpointRemovalService struct{}

func NewEndpointRemovalService() *EndpointRemovalService {
	return &EndpointRemovalService{}
}

func (s *EndpointRemovalService) RemoveConnected(ctx context.Context, client *admin.Client, request EndpointRemovalRequest) (EndpointRemovalResult, error) {
	if err := identity.ValidateEndpointID(request.EndpointID); err != nil {
		return EndpointRemovalResult{}, endpointRemovalValidationFailure("Select one exact Endpoint before requesting removal.")
	}
	if request.Confirmation != request.EndpointID {
		return EndpointRemovalResult{}, endpointRemovalValidationFailure("Type the exact case-sensitive Endpoint ID to confirm removal.")
	}
	if client == nil {
		return EndpointRemovalResult{}, ErrSessionNotConnected
	}
	if err := client.RemoveEndpointContext(ctx, request.EndpointID); err != nil {
		return EndpointRemovalResult{}, err
	}
	return EndpointRemovalResult{
		Status:           "removed",
		EndpointID:       request.EndpointID,
		CredentialStatus: "not_enrolled",
		AffectedEvidence: []string{"inventory", "activity"},
	}, nil
}

func endpointRemovalValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The Endpoint removal request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
