package configcompose

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
)

// ValidateProviderReleases applies the server's provider-release gate to every
// composed desired artifact without writing or advancing release state.
func ValidateProviderReleases(repoRoot string, explicit *providermatrix.Matrix) (Result, error) {
	repoRoot, err := absRepoRoot(repoRoot)
	if err != nil {
		return Result{}, err
	}
	artifacts, err := RenderAll(repoRoot)
	if err != nil {
		return Result{}, err
	}
	matrix, err := providerReleaseMatrix(explicit)
	if err != nil {
		return Result{}, err
	}
	result := Result{RepoRoot: repoRoot}
	for _, artifact := range artifacts {
		if artifact.ArtifactType != "desired" {
			continue
		}
		path := filepath.Join(artifact.TargetType+"s", artifact.TargetID)
		if err := ValidateRenderedProviderRelease(artifact, matrix); err != nil {
			result.Issues = append(result.Issues, Issue{
				Path: path, Code: configrepo.ProviderReleaseErrorCode(err), Message: err.Error(),
			})
			continue
		}
		result.OK = append(result.OK, path)
	}
	return result, nil
}

// ValidateRenderedProviderRelease is the shared validation rule used by the
// configuration CLI and server composition boundary.
func ValidateRenderedProviderRelease(artifact RenderedArtifact, matrix providermatrix.Matrix) error {
	state, err := models.ParseState(bytes.NewReader(artifact.YAML))
	if err != nil {
		return fmt.Errorf("%s %s provider release validation: %w", artifact.TargetType, artifact.TargetID, err)
	}
	if err := configrepo.ValidateProviderRelease(state, matrix); err != nil {
		return fmt.Errorf("%s %s provider release validation: %w", artifact.TargetType, artifact.TargetID, err)
	}
	return nil
}

func providerReleaseMatrix(explicit *providermatrix.Matrix) (providermatrix.Matrix, error) {
	if explicit != nil {
		if err := providermatrix.Validate(*explicit); err != nil {
			return providermatrix.Matrix{}, err
		}
		return *explicit, nil
	}
	return providermatrix.Default()
}
