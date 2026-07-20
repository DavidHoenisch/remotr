package aur_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/aur"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-PRM-019: absence of the selected AUR executable is a typed unsupported
// result and never routes the package through Pacman.
func TestApplicator_MissingYayReturnsUnsupportedWithoutFallback(t *testing.T) {
	uid, gid := testUnprivilegedIdentity()
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"id [-u -- aur-builder]": {Stdout: []byte(fmt.Sprintf("%d\n", uid))},
		"id [-g -- aur-builder]": {Stdout: []byte(fmt.Sprintf("%d\n", gid))},
		"yay [--version]":        {Err: errors.New("executable not found")},
	}}
	provider := newAURContractProvider(t, types.Arch, models.Package{
		Name: "aur-package", Present: true, AURBuildUser: "aur-builder",
	}, runner)

	check := provider.Check(t.Context())
	if check.Status != contract.Unsupported || check.ReasonCode != contract.ReasonProviderUnavailable {
		t.Fatalf("missing-yay Check() = %+v, want typed unsupported", check)
	}
	wantCalls := []executil.MockCall{
		{Name: "id", Args: []string{"-u", "--", "aur-builder"}},
		{Name: "id", Args: []string{"-g", "--", "aur-builder"}},
	}
	if !slices.EqualFunc(runner.Calls, wantCalls, equalAURCall) {
		t.Fatalf("missing-yay process calls = %+v, want %+v", runner.Calls, wantCalls)
	}
	if len(runner.UserCalls) != 1 || runner.UserCalls[0].Name != "yay" || !slices.Equal(runner.UserCalls[0].Args, []string{"--version"}) ||
		runner.UserCalls[0].UID != uid || runner.UserCalls[0].GID != gid {
		t.Fatalf("missing-yay unprivileged calls = %+v, want declared user yay --version", runner.UserCalls)
	}
}

func TestApplicator_MissingBuildUserReturnsUnsupportedWithoutFallback(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"yay [--version]":                {Stdout: []byte("yay v12\n")},
		"id [-u -- missing-aur-builder]": {Err: errors.New("no such user")},
	}}
	provider := newAURContractProvider(t, types.Arch, models.Package{
		Name: "aur-package", Present: true, AURBuildUser: "missing-aur-builder",
	}, runner)

	check := provider.Check(t.Context())
	if check.Status != contract.Unsupported || check.ReasonCode != contract.ReasonProviderUnavailable {
		t.Fatalf("missing-build-user Check() = %+v, want typed unsupported", check)
	}
	wantCalls := []executil.MockCall{{Name: "id", Args: []string{"-u", "--", "missing-aur-builder"}}}
	if !slices.EqualFunc(runner.Calls, wantCalls, equalAURCall) {
		t.Fatalf("missing-build-user process calls = %+v, want %+v", runner.Calls, wantCalls)
	}
}

func TestApplicator_UndeclaredBuildUserFailsBeforeProcessBoundary(t *testing.T) {
	runner := &executil.MockRunner{}
	provider := newAURContractProvider(t, types.Arch, models.Package{Name: "aur-package", Present: true}, runner)

	check := provider.Check(t.Context())
	if check.Status != contract.Unsupported || check.ReasonCode != contract.ReasonProviderUnavailable {
		t.Fatalf("undeclared-build-user Check() = %+v, want typed unsupported", check)
	}
	if len(runner.Calls) != 0 || len(runner.UserCalls) != 0 {
		t.Fatalf("undeclared build user crossed process boundary: calls=%+v userCalls=%+v", runner.Calls, runner.UserCalls)
	}
}

func TestApplicator_PrivilegedBuildUserFailsBeforeWorkspaceOrBuildBoundary(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"id [-u -- root-builder]": {Stdout: []byte("0\n")},
	}}
	provider := newAURContractProvider(t, types.Arch, models.Package{
		Name: "aur-package", Present: true, AURBuildUser: "root-builder",
	}, runner)

	check := provider.Check(t.Context())
	if check.Status != contract.Unsupported || check.ReasonCode != contract.ReasonProviderUnavailable {
		t.Fatalf("privileged-build-user Check() = %+v, want typed unsupported", check)
	}
	wantCalls := []executil.MockCall{{Name: "id", Args: []string{"-u", "--", "root-builder"}}}
	if !slices.EqualFunc(runner.Calls, wantCalls, equalAURCall) || len(runner.UserCalls) != 0 {
		t.Fatalf("privileged build user calls = %+v userCalls=%+v, want UID probe only", runner.Calls, runner.UserCalls)
	}
}

func newAURContractProvider(t *testing.T, distro types.Distro, pkg models.Package, runner executil.Runner) contract.Provider {
	t.Helper()
	provider, err := contract.New(aur.New(distro, pkg, runner))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func equalAURCall(left, right executil.MockCall) bool {
	return left.Name == right.Name && slices.Equal(left.Args, right.Args)
}
