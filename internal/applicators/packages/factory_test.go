package packages_test

import (
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestSelectPackageApplicator_rejectsYayWithoutAURProvider(t *testing.T) {
	_, err := packages.SelectPackageApplicator(types.Arch, models.Package{Name: "aur-package", PM: types.Yay}, facts.Facts{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "yay") || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("SelectPackageApplicator() error = %v, want truthful yay unsupported error", err)
	}
}
