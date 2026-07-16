package main

import (
	"context"
	"errors"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/admin"
	"github.com/DavidHoenisch/remotr/internal/agentversion"
)

type FleetUpgradeRequest struct {
	Fleet   string `json:"fleet"`
	Version string `json:"version"`
}

type FleetUpgradeResult struct {
	Status            string `json:"status"`
	Fleet             string `json:"fleet"`
	Version           string `json:"version"`
	AcceptedEndpoints int    `json:"acceptedEndpoints"`
}

type FleetUpgradeService struct{}

func NewFleetUpgradeService() *FleetUpgradeService {
	return &FleetUpgradeService{}
}

func (s *FleetUpgradeService) RequestConnected(ctx context.Context, client *admin.Client, request FleetUpgradeRequest) (FleetUpgradeResult, error) {
	if request.Fleet == "" || strings.TrimSpace(request.Fleet) != request.Fleet {
		return FleetUpgradeResult{}, fleetUpgradeValidationFailure("Select one exact Fleet before requesting an upgrade.")
	}
	version, err := agentversion.Normalize(request.Version)
	if err != nil {
		return FleetUpgradeResult{}, fleetUpgradeValidationFailure("Enter a semantic agent version such as v2.2.0.")
	}
	if client == nil {
		return FleetUpgradeResult{}, ErrSessionNotConnected
	}

	accepted, err := client.RequestFleetAgentUpgradeContext(ctx, request.Fleet, version)
	if err != nil {
		return FleetUpgradeResult{}, err
	}
	if accepted.Version != version {
		return FleetUpgradeResult{}, errors.New("server returned a different Fleet upgrade version")
	}
	if accepted.Endpoints < 0 {
		return FleetUpgradeResult{}, errors.New("server returned an invalid Fleet upgrade count")
	}
	return FleetUpgradeResult{
		Status:            "requested",
		Fleet:             request.Fleet,
		Version:           accepted.Version,
		AcceptedEndpoints: accepted.Endpoints,
	}, nil
}

func fleetUpgradeValidationFailure(guidance string) error {
	return &ActionFailure{
		Kind:      ActionValidation,
		Message:   "The Fleet upgrade request is invalid.",
		Guidance:  guidance,
		Retryable: false,
	}
}
