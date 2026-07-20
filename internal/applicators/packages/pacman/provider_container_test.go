//go:build providercontainer

package pacman_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pacman"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

const (
	fixtureSigningFingerprint = "8DDFCCB89FC8A63796554F956177FE96142F67AB"
	containerPacmanPackage    = "remotr-fixture"
	containerPacmanV1         = "1.0.0-1"
	containerPacmanV2         = "2.0.0-1"
)

// OS-PRM-001 through OS-PRM-005, OS-PRM-010, OS-PRM-017,
// OS-PRM-027, and OS-PRM-028: qualify Pacman through its public provider
// contract against the real database and controlled signed repository.
func TestProviderContainerPacmanContract(t *testing.T) {
	assertArchContainerIdentity(t)
	configurePacmanTrust(t)
	selectPacmanFixtureRepository(t, "v1")
	ensurePacmanFixtureAbsent(t)
	unrelatedVersion := installedPacmanVersion(t, "filesystem")

	t.Run("drifted apply second check and compliant no change", func(t *testing.T) {
		provider := newRealPacmanProvider(t, models.Package{
			Name: containerPacmanPackage, Present: true, Version: containerPacmanV1, RefreshCache: true,
		})
		assertPacmanCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertPacmanApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertPacmanInstalledVersion(t, containerPacmanV1)
		assertPacmanCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertPacmanApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
	})

	t.Run("exact upgrade and downgrade policy", func(t *testing.T) {
		selectPacmanFixtureRepository(t, "v2")
		upgrade := newRealPacmanProvider(t, models.Package{Name: containerPacmanPackage, Present: true, Version: containerPacmanV2})
		assertPacmanApplyStatus(t, upgrade.Apply(t.Context()), contract.Changed)
		assertPacmanInstalledVersion(t, containerPacmanV2)
		assertPacmanCheckStatus(t, upgrade.Check(t.Context()), contract.Compliant)

		selectPacmanFixtureRepository(t, "v1")
		blocked := newRealPacmanProvider(t, models.Package{Name: containerPacmanPackage, Present: true, Version: containerPacmanV1})
		result := blocked.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "downgrade") {
			t.Fatalf("blocked downgrade Apply() = %+v, want retained policy failure", result)
		}
		assertPacmanInstalledVersion(t, containerPacmanV2)

		allow := true
		permitted := newRealPacmanProvider(t, models.Package{
			Name: containerPacmanPackage, Present: true, Version: containerPacmanV1, AllowDowngrade: &allow,
		})
		assertPacmanApplyStatus(t, permitted.Apply(t.Context()), contract.Changed)
		assertPacmanInstalledVersion(t, containerPacmanV1)
		assertPacmanCheckStatus(t, permitted.Check(t.Context()), contract.Compliant)
	})

	t.Run("unavailable version fails without changing package database", func(t *testing.T) {
		provider := newRealPacmanProvider(t, models.Package{Name: containerPacmanPackage, Present: true, Version: "9.9.9-1"})
		result := provider.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil {
			t.Fatalf("unavailable version Apply() = %+v, want retained failure", result)
		}
		assertPacmanInstalledVersion(t, containerPacmanV1)
		assertPacmanCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
	})

	t.Run("native database lock contention fails without mutation", func(t *testing.T) {
		ensurePacmanFixtureAbsent(t)
		const lockPath = "/var/lib/pacman/db.lck"
		if err := os.WriteFile(lockPath, []byte("controlled contention\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		provider := newRealPacmanProvider(t, models.Package{Name: containerPacmanPackage, Present: true})
		result := provider.Apply(t.Context())
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
		if result.Status != contract.Failed || result.Err == nil {
			t.Fatalf("contended Apply() = %+v, want retained lock failure", result)
		}
		assertPacmanFixtureAbsent(t)
	})

	t.Run("activation is observable without implicit execution", func(t *testing.T) {
		provider := newRealPacmanProvider(t, models.Package{
			Name: containerPacmanPackage, Present: true, Version: containerPacmanV1,
			ResourceMeta: models.ResourceMeta{Notifications: []models.Notification{{
				Type: models.NotificationTryRestart, Target: "fixture.service",
			}}},
		})
		result := provider.Apply(t.Context())
		want := []executor.ActivationSignal{{Kind: executor.ActivationTryRestart, Target: "fixture.service"}}
		if result.Status != contract.Changed || !slices.Equal(result.Activation, want) {
			t.Fatalf("activation Apply() = %+v, want %v", result, want)
		}
		assertPacmanInstalledVersion(t, containerPacmanV1)
	})

	t.Run("removal converges and preserves unrelated package state", func(t *testing.T) {
		provider := newRealPacmanProvider(t, models.Package{
			Name: containerPacmanPackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
		})
		assertPacmanCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertPacmanApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertPacmanCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertPacmanApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
		if got := installedPacmanVersion(t, "filesystem"); got != unrelatedVersion {
			t.Fatalf("unrelated filesystem package version = %q, want preserved %q", got, unrelatedVersion)
		}
	})

	assertPacmanFixtureAbsent(t)
}

func assertArchContainerIdentity(t *testing.T) {
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

func configurePacmanTrust(t *testing.T) {
	t.Helper()
	runContainerCommand(t, "pacman-key", "--init")
	runContainerCommand(t, "pacman-key", "--add", "/fixtures/keys/signing-public.asc")
	runContainerCommand(t, "pacman-key", "--lsign-key", fixtureSigningFingerprint)
}

func selectPacmanFixtureRepository(t *testing.T, version string) {
	t.Helper()
	if version != "v1" && version != "v2" {
		t.Fatalf("unknown fixture repository %q", version)
	}
	configuration := `[options]
Architecture = auto
SigLevel = Required DatabaseOptional
LocalFileSigLevel = Required

[remotr-fixture]
SigLevel = Required DatabaseRequired
Server = file:///fixtures/pacman/` + version + "\n"
	if err := os.WriteFile("/etc/pacman.conf", []byte(configuration), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"/var/lib/pacman/sync/remotr-fixture*", "/var/cache/pacman/pkg/remotr-fixture*"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				t.Fatal(err)
			}
		}
	}
	provider := pacman.New(models.Package{Name: containerPacmanPackage, Present: true}, nil)
	if err := provider.RefreshCache(t.Context()); err != nil {
		t.Fatalf("refresh signed Pacman fixture repository: %v", err)
	}
}

func newRealPacmanProvider(t *testing.T, pkg models.Package) contract.Provider {
	t.Helper()
	provider, err := contract.New(pacman.New(pkg, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func ensurePacmanFixtureAbsent(t *testing.T) {
	t.Helper()
	provider := newRealPacmanProvider(t, models.Package{
		Name: containerPacmanPackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	result := provider.Apply(t.Context())
	if result.Status != contract.Changed && result.Status != contract.NoChange {
		t.Fatalf("ensure fixture absent: %+v", result)
	}
	assertPacmanFixtureAbsent(t)
}

func assertPacmanFixtureAbsent(t *testing.T) {
	t.Helper()
	provider := newRealPacmanProvider(t, models.Package{
		Name: containerPacmanPackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	assertPacmanCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
}

func assertPacmanInstalledVersion(t *testing.T, want string) {
	t.Helper()
	if got := installedPacmanVersion(t, containerPacmanPackage); got != want {
		t.Fatalf("installed Pacman fixture version = %q, want %q", got, want)
	}
	out, err := exec.Command("remotr-fixture").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != want {
		t.Fatalf("fixture executable = %q, err %v; want %q", strings.TrimSpace(string(out)), err, want)
	}
}

func installedPacmanVersion(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("pacman", "-Q", name).CombinedOutput()
	if err != nil {
		t.Fatalf("query installed Pacman package %q: %v: %s", name, err, out)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 || fields[0] != name {
		t.Fatalf("installed Pacman package output = %q", out)
	}
	return fields[1]
}

func runContainerCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
}

func assertPacmanCheckStatus(t *testing.T, result contract.Observation, want contract.CheckStatus) {
	t.Helper()
	if result.Status != want {
		t.Fatalf("Check() = %+v, want %q", result, want)
	}
}

func assertPacmanApplyStatus(t *testing.T, result contract.ApplyResult, want contract.ApplyStatus) {
	t.Helper()
	if result.Status != want || result.Err != nil {
		t.Fatalf("Apply() = %+v, want %q without error", result, want)
	}
}
