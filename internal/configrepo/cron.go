package configrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/DavidHoenisch/remotr/internal/safepath"
)

// FleetCronArtifact loads fleets/<fleet>/crons.yaml from a configuration repository directory.
func FleetCronArtifact(repoRoot, fleet string) (yaml []byte, digest string, err error) {
	if err := ValidateFleetName(fleet); err != nil {
		return nil, "", err
	}
	data, err := safepath.ReadUnderRoot(repoRoot, "fleets", fleet, "crons.yaml")
	if err != nil {
		return nil, "", fmt.Errorf("read fleet crons artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

// EndpointCronArtifact loads endpoints/<endpoint-id>/crons.yaml from a configuration repository.
func EndpointCronArtifact(repoRoot, endpointID string) (yaml []byte, digest string, err error) {
	if err := ValidateEndpointID(endpointID); err != nil {
		return nil, "", err
	}
	data, err := safepath.ReadUnderRoot(repoRoot, "endpoints", endpointID, "crons.yaml")
	if err != nil {
		return nil, "", fmt.Errorf("read endpoint crons artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

// ResolveCronArtifact returns the endpoint override crons artifact when present,
// otherwise the fleet crons artifact. Missing files are not an error.
func ResolveCronArtifact(repoRoot, fleet, endpointID string) (yaml []byte, digest string, ok bool, err error) {
	if err := ValidateEndpointID(endpointID); err == nil {
		yaml, digest, err = EndpointCronArtifact(repoRoot, endpointID)
		if err == nil {
			return yaml, digest, true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", false, err
		}
	}
	yaml, digest, err = FleetCronArtifact(repoRoot, fleet)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	return yaml, digest, true, nil
}
