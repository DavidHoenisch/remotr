//go:build providercontainer

package aur_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/aur"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/types"
)

const (
	containerAURPackage   = "remotr-aur-fixture"
	containerAURVersion   = "1.0.0-1"
	containerAURBuildUser = "remotr-aur-builder"
	containerAURWorkspace = "/var/tmp/remotr-aur-provider"
)

// OS-PRM-019 and OS-PRM-024 through OS-PRM-026: qualify the AUR provider in
// the pinned Arch image. The controlled yay executable replaces the live AUR
// source service only; real makepkg and Pacman boundaries remain in use.
func TestProviderContainerAURContract(t *testing.T) {
	assertAURArchContainerIdentity(t)
	assertAURContainerTooling(t)
	ensureAURBuildUser(t)
	if err := os.MkdirAll(containerAURWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	ensureAURFixtureAbsent(t)
	assertAURWorkspaceClean(t)

	t.Run("drifted apply second check and compliant no change", func(t *testing.T) {
		provider := newRealAURProvider(t, models.Package{
			Name: containerAURPackage, Present: true, Version: containerAURVersion,
			AURBuildUser: containerAURBuildUser,
			ResourceMeta: models.ResourceMeta{Notifications: []models.Notification{{
				Type: models.NotificationTryRestart, Target: "fixture.service",
			}}},
		})
		assertAURCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		result := provider.Apply(t.Context())
		wantActivation := []executor.ActivationSignal{{Kind: executor.ActivationTryRestart, Target: "fixture.service"}}
		if result.Status != contract.Changed || result.Err != nil || !slices.Equal(result.Activation, wantActivation) || len(result.Diagnostics) != 2 {
			t.Fatalf("AUR Apply() = %+v, want changed with activation and source/artifact evidence", result)
		}
		assertAURInstalledVersion(t, containerAURVersion)
		assertAURCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertAURApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
		assertAURWorkspaceClean(t)
	})

	t.Run("absence converges through Pacman", func(t *testing.T) {
		provider := newRealAURProvider(t, models.Package{
			Name: containerAURPackage, AURBuildUser: containerAURBuildUser,
			ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
		})
		assertAURCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertAURApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertAURCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertAURApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
		assertAURWorkspaceClean(t)
	})

	t.Run("unavailable exact version never builds or installs another version", func(t *testing.T) {
		provider := newRealAURProvider(t, models.Package{
			Name: containerAURPackage, Present: true, Version: "9.9.9-1", AURBuildUser: containerAURBuildUser,
		})
		result := provider.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "version 9.9.9-1 is unavailable") {
			t.Fatalf("unavailable AUR version Apply() = %+v, want retained failure", result)
		}
		if len(result.Diagnostics) != 1 || !strings.Contains(string(result.Diagnostics[0]), containerAURVersion) {
			t.Fatalf("unavailable AUR version diagnostics = %q, want sanitized source identity", result.Diagnostics)
		}
		assertAURFixtureAbsent(t)
		assertAURWorkspaceClean(t)
	})

	t.Run("native database lock failure preserves absence and cleanup", func(t *testing.T) {
		const lockPath = "/var/lib/pacman/db.lck"
		if err := os.WriteFile(lockPath, []byte("controlled contention\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		provider := newRealAURProvider(t, models.Package{
			Name: containerAURPackage, Present: true, Version: containerAURVersion, AURBuildUser: containerAURBuildUser,
		})
		result := provider.Apply(t.Context())
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
		if result.Status != contract.Failed || result.Err == nil || len(result.Diagnostics) != 3 || result.Diagnostics[2] != "AUR post-failure native state absent" {
			t.Fatalf("locked AUR Apply() = %+v, want failed with consistent native state", result)
		}
		assertAURFixtureAbsent(t)
		assertAURWorkspaceClean(t)
	})
}

func assertAURArchContainerIdentity(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ID=arch") {
		t.Fatalf("container os-release is not Arch: %s", data)
	}
	wantRelease := os.Getenv("REMOTR_PROVIDER_RELEASE")
	if wantRelease != "2026-07-06" || os.Getenv("REMOTR_PROVIDER_IMAGE_RELEASE") != wantRelease {
		t.Fatalf("container release identity = %q/%q, want pinned %q", os.Getenv("REMOTR_PROVIDER_IMAGE_RELEASE"), wantRelease, "2026-07-06")
	}
	out, err := exec.Command("uname", "-m").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "x86_64" {
		t.Fatalf("container architecture = %q, err %v; want x86_64", strings.TrimSpace(string(out)), err)
	}
}

func assertAURContainerTooling(t *testing.T) {
	t.Helper()
	for _, name := range []string{"yay", "makepkg", "fakeroot", "pacman", "useradd"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("pinned Arch AUR prerequisite %q is unavailable: %v", name, err)
		}
	}
}

func ensureAURBuildUser(t *testing.T) {
	t.Helper()
	if err := exec.Command("id", "-u", containerAURBuildUser).Run(); err == nil {
		return
	}
	if out, err := exec.Command("useradd", "--create-home", "--shell", "/bin/bash", containerAURBuildUser).CombinedOutput(); err != nil {
		t.Fatalf("create controlled AUR build user: %v: %s", err, out)
	}
}

func newRealAURProvider(t *testing.T, pkg models.Package) contract.Provider {
	t.Helper()
	provider := aur.New(types.Arch, pkg, nil)
	provider.WorkspaceRoot = containerAURWorkspace
	wrapped, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func ensureAURFixtureAbsent(t *testing.T) {
	t.Helper()
	provider := newRealAURProvider(t, models.Package{
		Name: containerAURPackage, AURBuildUser: containerAURBuildUser,
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	result := provider.Apply(t.Context())
	if result.Status != contract.Changed && result.Status != contract.NoChange {
		t.Fatalf("ensure AUR fixture absent: %+v", result)
	}
	assertAURFixtureAbsent(t)
}

func assertAURFixtureAbsent(t *testing.T) {
	t.Helper()
	out, err := exec.Command("pacman", "-Q", containerAURPackage).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "was not found") {
		t.Fatalf("AUR fixture unexpectedly installed: err=%v output=%s", err, out)
	}
}

func assertAURInstalledVersion(t *testing.T, want string) {
	t.Helper()
	out, err := exec.Command("pacman", "-Q", containerAURPackage).CombinedOutput()
	if err != nil {
		t.Fatalf("query installed AUR fixture: %v: %s", err, out)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 || fields[0] != containerAURPackage || fields[1] != want {
		t.Fatalf("installed AUR fixture = %q, want %s %s", out, containerAURPackage, want)
	}
	out, err = exec.Command(containerAURPackage).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != want {
		t.Fatalf("AUR fixture executable = %q, err %v; want %q", strings.TrimSpace(string(out)), err, want)
	}
}

func assertAURWorkspaceClean(t *testing.T) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(containerAURWorkspace, "remotr-aur-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("transient AUR workspaces remain: %v", matches)
	}
}

func assertAURCheckStatus(t *testing.T, result contract.Observation, want contract.CheckStatus) {
	t.Helper()
	if result.Status != want {
		t.Fatalf("Check() = %+v, want %q", result, want)
	}
}

func assertAURApplyStatus(t *testing.T, result contract.ApplyResult, want contract.ApplyStatus) {
	t.Helper()
	if result.Status != want || result.Err != nil {
		t.Fatalf("Apply() = %+v, want %q without error", result, want)
	}
}
