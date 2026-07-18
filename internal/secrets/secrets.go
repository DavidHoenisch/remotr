// Package secrets defines provider-neutral secret resolution contracts.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secretref"
)

const (
	ProviderLocalFile = secretref.ProviderLocalFile
	ProviderRemotr    = secretref.ProviderRemotr

	// MaxMaterialBytes bounds one resolved value before it enters an endpoint.
	MaxMaterialBytes = 1 << 20
)

var (
	ErrInvalidReference = secretref.ErrInvalid
	ErrUnauthorized     = errors.New("secret resolution unauthorized")
)

// ResolveRequest binds a reference to the authenticated endpoint and exact
// active desired-state use that authorized it.
type ResolveRequest struct {
	Reference       string `json:"reference"`
	EndpointID      string `json:"endpointId,omitempty"`
	Fleet           string `json:"fleet,omitempty"`
	ArtifactDigest  string `json:"artifactDigest"`
	ResourceAddress string `json:"resourceAddress"`
	Purpose         string `json:"purpose"`
}

// Resolved carries bounded material plus metadata safe for reports and audit.
type Resolved struct {
	Provider             string `json:"provider"`
	Version              string `json:"version,omitempty"`
	ActivationGeneration uint64 `json:"activationGeneration,omitempty"`
	Fingerprint          string `json:"fingerprint,omitempty"`
	Material             []byte `json:"material"`
}

// Resolver supplies secret material at a trusted provider boundary.
type Resolver interface {
	Resolve(context.Context, ResolveRequest) (Resolved, error)
}

// ParseReference returns the provider and provider-owned identifier. The
// legacy file: spelling remains readable as local-file during migration.
func ParseReference(raw string) (provider, identifier string, err error) {
	return secretref.Parse(raw)
}

// ValidateRequest validates non-secret scoping fields before provider access.
func ValidateRequest(request ResolveRequest) error {
	if _, _, err := ParseReference(request.Reference); err != nil {
		return err
	}
	if strings.TrimSpace(request.ResourceAddress) == "" || strings.TrimSpace(request.ResourceAddress) != request.ResourceAddress || !strings.Contains(request.ResourceAddress, "/") {
		return fmt.Errorf("resource address is required")
	}
	if strings.TrimSpace(request.Purpose) == "" || strings.TrimSpace(request.Purpose) != request.Purpose || strings.ContainsAny(request.Purpose, "\x00\r\n") {
		return fmt.Errorf("secret purpose is required")
	}
	if len(request.Purpose) > 128 || len(request.ResourceAddress) > 512 || len(request.ArtifactDigest) > 256 {
		return fmt.Errorf("secret resolution scope exceeds bounds")
	}
	return nil
}

// ArtifactAuthorizes reports whether the exact resource in a parsed artifact
// declares the requested reference. Values are never returned by this helper.
func ArtifactAuthorizes(state models.State, resourceAddress, reference, purpose string) bool {
	for i := range state.Configurations {
		configuration := &state.Configurations[i]
		match := func(name string) bool {
			return models.ResourceAddress(configuration.Name, name) == resourceAddress
		}
		for _, repository := range configuration.APTRepositories {
			if match(repository.Name) && repository.CredentialRef == reference && purpose == "repository-credential" {
				return true
			}
		}
		for _, user := range configuration.Users {
			if match(user.Name) && user.PasswordHashRef == reference && purpose == "password-hash" {
				return true
			}
		}
		for _, schedule := range configuration.EndpointSchedules {
			if !match(schedule.Name) {
				continue
			}
			for _, variable := range schedule.Environment {
				if variable.SecretRef == reference && purpose == "schedule-environment" {
					return true
				}
			}
		}
		for _, profile := range configuration.NetworkProfiles {
			if match(profile.Name) && profile.CredentialRef == reference && purpose == "network-credential" {
				return true
			}
		}
	}
	return false
}
