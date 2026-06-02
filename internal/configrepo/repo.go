package configrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/safepath"
)

// FleetArtifact loads fleets/<fleet>/desired.yaml from a configuration repository directory.
func FleetArtifact(repoRoot, fleet string) (yaml []byte, digest string, err error) {
	if err := ValidateFleetName(fleet); err != nil {
		return nil, "", err
	}
	data, err := safepath.ReadUnderRoot(repoRoot, "fleets", fleet, "desired.yaml")
	if err != nil {
		return nil, "", fmt.Errorf("read fleet artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

// ValidateFleetName checks a fleet directory name (fleets/<name>/desired.yaml).
func ValidateFleetName(fleet string) error {
	if fleet == "" {
		return fmt.Errorf("invalid fleet name")
	}
	for _, r := range fleet {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			continue
		default:
			return fmt.Errorf("invalid fleet name")
		}
	}
	return nil
}

// EndpointArtifact loads endpoints/<endpoint-id>/desired.yaml from a configuration repository.
func EndpointArtifact(repoRoot, endpointID string) (yaml []byte, digest string, err error) {
	if err := ValidateEndpointID(endpointID); err != nil {
		return nil, "", err
	}
	data, err := safepath.ReadUnderRoot(repoRoot, "endpoints", endpointID, "desired.yaml")
	if err != nil {
		return nil, "", fmt.Errorf("read endpoint artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

// ResolveArtifact returns the endpoint override artifact when present, otherwise the fleet artifact.
func ResolveArtifact(repoRoot, fleet, endpointID string) (yaml []byte, digest string, err error) {
	if err := ValidateEndpointID(endpointID); err == nil {
		yaml, digest, err = EndpointArtifact(repoRoot, endpointID)
		if err == nil {
			return yaml, digest, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
	}
	return FleetArtifact(repoRoot, fleet)
}

// ValidateEndpointID checks an endpoint identifier used in endpoints/<id>/desired.yaml paths.
func ValidateEndpointID(endpointID string) error {
	if endpointID == "" {
		return fmt.Errorf("invalid endpoint id")
	}
	if err := identity.ValidateEndpointID(endpointID); err != nil {
		return fmt.Errorf("invalid endpoint id")
	}
	return nil
}
