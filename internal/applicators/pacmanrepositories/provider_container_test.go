//go:build providercontainer

package pacmanrepositories_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/pacman"
	"github.com/DavidHoenisch/remotr/internal/applicators/pacmankeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/pacmanrepositories"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

const (
	archFixtureFingerprint   = "8DDFCCB89FC8A63796554F956177FE96142F67AB"
	archUnrelatedFingerprint = "F9E2B9F7F04D8BB33EC7FB3431DD6980551A87F1"
)

// OS-PRM-018 and OS-PRM-020 through OS-PRM-022: qualify the complete Pacman
// trust/repository boundary against the actual pinned Arch keyring,
// configuration parser, and signed package database.
func TestProviderContainerPacmanRepositoryAndTrustContract(t *testing.T) {
	assertPacmanRepositoryContainerIdentity(t)
	assertPacmanRepositoryTooling(t)
	cleanupPacmanRepositoryFixture(t)
	defer cleanupPacmanRepositoryFixture(t)

	const unrelatedConfig = "# unrelated distribution-owned bytes\n[options]\nArchitecture = auto\nSigLevel = Required DatabaseOptional\nLocalFileSigLevel = Required\n"
	if err := os.WriteFile("/etc/pacman.conf", []byte(unrelatedConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	runPacmanRepositoryCommand(t, "pacman-key", "--init")
	runPacmanRepositoryCommand(t, "pacman-key", "--add", "/fixtures/keys/mismatch-public.asc")
	runPacmanRepositoryCommand(t, "pacman-key", "--lsign-key", archUnrelatedFingerprint)

	t.Run("mismatched material never changes native trust", func(t *testing.T) {
		provider := newRealPacmanSigningKeyProvider(t, models.PacmanSigningKey{
			Name: "mismatch-claim", Source: "https://fixtures.invalid/signing-public.asc",
			Fingerprint: archUnrelatedFingerprint,
		})
		result := provider.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "fingerprint mismatch") {
			t.Fatalf("mismatched key Apply() = %+v, want retained fingerprint failure", result)
		}
		if _, err := os.Stat("/var/lib/remotr/pacman-keys/mismatch-claim.fingerprint"); !os.IsNotExist(err) {
			t.Fatalf("mismatched trust marker persisted: %v", err)
		}
	})

	key := newRealPacmanSigningKeyProvider(t, models.PacmanSigningKey{
		Name: "remotr-fixture", Source: "https://fixtures.invalid/signing-public.asc", Fingerprint: archFixtureFingerprint,
	})
	assertPacmanRepositoryCheckStatus(t, key.Check(t.Context()), contract.Drifted)
	assertPacmanRepositoryApplyStatus(t, key.Apply(t.Context()), contract.Changed)
	assertPacmanRepositoryCheckStatus(t, key.Check(t.Context()), contract.Compliant)
	assertPacmanRepositoryApplyStatus(t, key.Apply(t.Context()), contract.NoChange)

	server := httptest.NewServer(http.FileServer(http.Dir("/fixtures/pacman/v1")))
	defer server.Close()
	repository := models.PacmanRepository{
		Name: "remotr-fixture", Servers: []string{server.URL}, Architecture: "x86_64",
		SignatureLevel: models.PacmanSignatureRequired, SigningKeys: []string{"remotr-fixture"},
	}

	t.Run("present activates a natively valid signed repository", func(t *testing.T) {
		provider := newRealPacmanRepositoryProvider(t, repository)
		assertPacmanRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertPacmanRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertPacmanRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertPacmanRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
		runPacmanRepositoryCommand(t, "pacman", "-Sy", "--noconfirm")
		output := runPacmanRepositoryCommand(t, "pacman", "-Si", "remotr-fixture")
		if !strings.Contains(output, "Version         : 1.0.0-1") {
			t.Fatalf("signed repository package metadata = %q", output)
		}
	})

	t.Run("disabled repository is absent from effective configuration", func(t *testing.T) {
		disabled := repository
		disabled.Lifecycle = models.LifecycleDisabled
		provider := newRealPacmanRepositoryProvider(t, disabled)
		assertPacmanRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertPacmanRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertPacmanRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		if output := runPacmanRepositoryCommand(t, "pacman-conf", "--config", "/etc/pacman.conf", "--repo-list"); strings.Contains(output, "remotr-fixture") {
			t.Fatalf("disabled repository remains effective: %q", output)
		}
	})

	assertPacmanRepositoryApplyStatus(t, newRealPacmanRepositoryProvider(t, repository).Apply(t.Context()), contract.Changed)
	t.Run("absence removes only owned configuration", func(t *testing.T) {
		absent := models.PacmanRepository{Name: repository.Name, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}}
		provider := newRealPacmanRepositoryProvider(t, absent)
		assertPacmanRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertPacmanRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertPacmanRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertPacmanRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
		if got, err := os.ReadFile("/etc/pacman.conf"); err != nil || string(got) != unrelatedConfig {
			t.Fatalf("unrelated pacman.conf bytes = %q, %v; want exact preservation", got, err)
		}
	})

	absentKey := newRealPacmanSigningKeyProvider(t, models.PacmanSigningKey{
		Name: "remotr-fixture", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	assertPacmanRepositoryApplyStatus(t, absentKey.Apply(t.Context()), contract.Changed)
	assertPacmanRepositoryCheckStatus(t, absentKey.Check(t.Context()), contract.Compliant)
	runPacmanRepositoryCommand(t, "pacman-key", "--nocolor", "--finger", archUnrelatedFingerprint)

	t.Run("composed trust repository and exact package workflow", func(t *testing.T) {
		_, _, _ = runPacmanRepositoryCommandResult("pacman", "-Rns", "--noconfirm", "remotr-fixture")
		keyProvider := pacmankeys.New(models.PacmanSigningKey{
			Name: "remotr-fixture", Source: "https://fixtures.invalid/signing-public.asc", Fingerprint: archFixtureFingerprint,
		}, nil)
		keyProvider.Fetch = func(context.Context, string) ([]byte, error) {
			return os.ReadFile("/fixtures/keys/signing-public.asc")
		}
		repositoryProvider := pacmanrepositories.New(repository, nil)
		allowUpgrade := true
		packageProvider := pacman.New(models.Package{
			Name: "remotr-fixture", Present: true, Version: "1.0.0-1", RefreshCache: true, AllowUpgrade: &allowUpgrade,
		}, nil)
		workflow, err := engine.NewForExecution([]engine.ExecutionResource{
			{Address: "base/remotr-fixture-key", Name: "remotr-fixture", Kind: engine.KindPacmanSigningKey, Provider: "pacman-key", Handler: keyProvider, LockDomains: []string{"package-manager:pacman"}},
			{Address: "base/remotr-fixture-repository", Name: "remotr-fixture", Kind: engine.KindPacmanRepository, Provider: "pacman-repository", DependsOn: []string{"base/remotr-fixture-key"}, Handler: repositoryProvider, LockDomains: []string{"package-manager:pacman"}},
			{Address: "base/remotr-fixture-package", Name: "remotr-fixture", Kind: engine.KindPackage, Provider: "pacman", DependsOn: []string{"base/remotr-fixture-repository"}, Handler: packageProvider, LockDomains: []string{"package-manager:pacman"}},
		}, executil.SanitizedOSRunner{})
		if err != nil {
			t.Fatal(err)
		}
		result := workflow.ApplyAll(t.Context(), engine.PolicyAuto)
		wantApplied := []string{"base/remotr-fixture-key", "base/remotr-fixture-repository", "base/remotr-fixture-package"}
		if result.Failed != nil || !slices.Equal(result.Applied, wantApplied) {
			t.Fatalf("composed Pacman ApplyAll() = %+v, want %v", result, wantApplied)
		}
		if report := workflow.CheckAll(t.Context()); !report.InCompliance {
			t.Fatalf("composed Pacman second CheckAll() = %+v", report)
		}
		output := runPacmanRepositoryCommand(t, "pacman", "-Q", "remotr-fixture")
		if strings.TrimSpace(output) != "remotr-fixture 1.0.0-1" {
			t.Fatalf("exact composed Pacman package version = %q", strings.TrimSpace(output))
		}
		if got, err := os.ReadFile("/etc/pacman.conf"); err != nil || !strings.HasPrefix(string(got), unrelatedConfig) {
			t.Fatalf("composed workflow did not preserve unrelated pacman.conf: %q, %v", got, err)
		}
		runPacmanRepositoryCommand(t, "pacman-key", "--nocolor", "--finger", archUnrelatedFingerprint)
	})
}

func assertPacmanRepositoryContainerIdentity(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ID=arch") || os.Getenv("REMOTR_PROVIDER_RELEASE") != "2026-07-06" || os.Getenv("REMOTR_PROVIDER_IMAGE_RELEASE") != "2026-07-06" {
		t.Fatalf("container identity = %s release=%q/%q, want pinned Arch 2026-07-06", data, os.Getenv("REMOTR_PROVIDER_RELEASE"), os.Getenv("REMOTR_PROVIDER_IMAGE_RELEASE"))
	}
	if output := runPacmanRepositoryCommand(t, "uname", "-m"); strings.TrimSpace(output) != "x86_64" {
		t.Fatalf("container architecture = %q, want x86_64", strings.TrimSpace(output))
	}
}

func assertPacmanRepositoryTooling(t *testing.T) {
	t.Helper()
	for _, name := range []string{"gpg", "pacman", "pacman-conf", "pacman-key"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("Pacman repository prerequisite %q is unavailable: %v", name, err)
		}
	}
}

func newRealPacmanSigningKeyProvider(t *testing.T, key models.PacmanSigningKey) contract.Provider {
	t.Helper()
	provider := pacmankeys.New(key, nil)
	provider.Fetch = func(context.Context, string) ([]byte, error) {
		return os.ReadFile("/fixtures/keys/signing-public.asc")
	}
	wrapped, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func newRealPacmanRepositoryProvider(t *testing.T, repository models.PacmanRepository) contract.Provider {
	t.Helper()
	wrapped, err := contract.New(pacmanrepositories.New(repository, nil))
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func cleanupPacmanRepositoryFixture(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		"/etc/pacman.d/remotr-repositories/remotr-fixture.conf",
		"/var/lib/remotr/pacman-keys/remotr-fixture.fingerprint",
		"/var/lib/remotr/pacman-keys/mismatch-claim.fingerprint",
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	for _, fingerprint := range []string{archFixtureFingerprint, archUnrelatedFingerprint} {
		_, _, _ = runPacmanRepositoryCommandResult("pacman-key", "--delete", fingerprint)
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
}

func runPacmanRepositoryCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, stderr, err := runPacmanRepositoryCommandResult(name, args...)
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, stderr)
	}
	return string(output)
}

func runPacmanRepositoryCommandResult(name string, args ...string) ([]byte, []byte, error) {
	command := exec.Command(name, args...)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return []byte(stdout.String()), []byte(stderr.String()), err
}

func assertPacmanRepositoryCheckStatus(t *testing.T, result contract.Observation, want contract.CheckStatus) {
	t.Helper()
	if result.Status != want {
		t.Fatalf("Check() = %+v, want %q", result, want)
	}
}

func assertPacmanRepositoryApplyStatus(t *testing.T, result contract.ApplyResult, want contract.ApplyStatus) {
	t.Helper()
	if result.Status != want || result.Err != nil {
		t.Fatalf("Apply() = %+v, want %q without error", result, want)
	}
}
