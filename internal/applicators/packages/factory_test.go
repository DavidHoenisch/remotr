package packages_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/facts"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

func TestSelectPackageApplicator_reportsArchRepositoryPackagesOnlyAsPacman(t *testing.T) {
	for _, pkg := range []models.Package{
		{Name: "curl", PM: types.Pacman},
		{Name: "curl"},
	} {
		provider, err := packages.SelectPackageApplicator(types.Arch, pkg, facts.Facts{}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := []string{provider.Name(), provider.Description()}; !slices.Equal(got, []string{"pacman:curl", "pacman package curl"}) {
			t.Fatalf("Arch repository provider identity = %q, want Pacman-only identity", got)
		}
	}
}

func TestSelectPackageApplicator_YayOnWrongPlatformReturnsTypedUnsupported(t *testing.T) {
	for _, distro := range []types.Distro{types.Ubuntu, ""} {
		t.Run(string(distro), func(t *testing.T) {
			runner := &executil.MockRunner{}
			provider, err := packages.SelectPackageApplicator(distro, models.Package{Name: "aur-package", PM: types.Yay, AURBuildUser: "aur-builder"}, facts.Facts{}, runner, nil)
			if err != nil {
				t.Fatalf("SelectPackageApplicator() = %v, want constructed truthful AUR boundary", err)
			}
			check := executor.Check(t.Context(), provider)
			if check.Status != executor.Unsupported || check.ReasonCode != executor.ReasonProviderUnavailable {
				t.Fatalf("Yay wrong-platform Check() = %+v, want typed unsupported", check)
			}
			if len(runner.Calls) != 0 || len(runner.UserCalls) != 0 {
				t.Fatalf("wrong-platform AUR check crossed a process boundary: calls=%+v userCalls=%+v", runner.Calls, runner.UserCalls)
			}
		})
	}
}

func TestSelectPackageApplicator_rejectsDeferredDNF(t *testing.T) {
	_, err := packages.SelectPackageApplicator(types.Ubuntu, models.Package{Name: "curl", PM: types.Dnf}, facts.Facts{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "future RPM-family") {
		t.Fatalf("SelectPackageApplicator() error = %v, want DNF roadmap diagnostic", err)
	}
}

func TestSelectPackageApplicator_rejectsUnknownPackageManager(t *testing.T) {
	_, err := packages.SelectPackageApplicator(types.Ubuntu, models.Package{Name: "curl", PM: types.PackageManager("unknown")}, facts.Facts{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported package manager") {
		t.Fatalf("SelectPackageApplicator() error = %v, want unsupported package manager", err)
	}
}
