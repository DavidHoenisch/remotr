package capabilitymatrix_test

import (
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/capabilitymatrix"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestServiceRequirementsTrackExplicitProvider(t *testing.T) {
	resource := &models.ServiceResource{Provider: models.ServiceProviderOpenRC}
	got := capabilitymatrix.Requirements(models.ResourceKindService, resource)
	if !slices.Contains(got, "provider:init/openrc") || slices.Contains(got, "provider:init/systemd") {
		t.Fatalf("Requirements() = %v", got)
	}
	if err := capabilitymatrix.CheckRuntime(resource, facts.Facts{Init: facts.InitOpenRC}); err != nil {
		t.Fatalf("OpenRC runtime match = %v", err)
	}
	if err := capabilitymatrix.CheckRuntime(resource, facts.Facts{Init: facts.InitSystemd}); err == nil {
		t.Fatal("systemd facts unexpectedly satisfied explicit OpenRC provider")
	}
}
