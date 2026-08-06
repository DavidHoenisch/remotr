//go:build providercontainer

package aptrepositories_test

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
	"github.com/DavidHoenisch/remotr/internal/applicators/aptkeys"
	"github.com/DavidHoenisch/remotr/internal/applicators/aptrepositories"
	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

const aptFixtureFingerprint = "8DDFCCB89FC8A63796554F956177FE96142F67AB"

// OS-PRM-011 through OS-PRM-015: qualify the APT trust and repository
// providers against the real GPG/APT configuration boundary and controlled
// signed source in each explicitly selected Debian/Ubuntu image.
func TestProviderContainerAPTRepositoryAndKeyContract(t *testing.T) {
	assertAPTRepositoryContainerIdentity(t)
	assertAPTRepositoryTooling(t)
	unrelated := installUnrelatedAPTMarkers(t)
	defer removeOwnedAPTFixtureFiles(t)

	t.Run("complete fingerprint mismatch never activates key", func(t *testing.T) {
		provider := newRealAPTKeyProvider(t, models.APTSigningKey{
			Name: "mismatch", Source: "https://fixtures.invalid/signing-public.asc",
			Fingerprint: "0DDFCCB89FC8A63796554F956177FE96142F67AB",
		})
		result := provider.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "fingerprint mismatch") {
			t.Fatalf("mismatched key Apply() = %+v, want retained failure", result)
		}
		if _, err := os.Stat("/etc/apt/keyrings/mismatch.gpg"); !os.IsNotExist(err) {
			t.Fatalf("mismatched keyring was persisted: %v", err)
		}
	})

	key := newRealAPTKeyProvider(t, models.APTSigningKey{
		Name: "remotr-fixture", Source: "https://fixtures.invalid/signing-public.asc", Fingerprint: aptFixtureFingerprint,
	})
	assertAPTRepositoryCheckStatus(t, key.Check(t.Context()), contract.Drifted)
	assertAPTRepositoryApplyStatus(t, key.Apply(t.Context()), contract.Changed)
	assertAPTRepositoryCheckStatus(t, key.Check(t.Context()), contract.Compliant)
	assertAPTRepositoryApplyStatus(t, key.Apply(t.Context()), contract.NoChange)

	server := httptest.NewServer(http.FileServer(http.Dir("/fixtures/apt")))
	defer server.Close()
	repository := models.APTRepository{
		Name: "remotr-fixture", URL: server.URL, Suites: []string{"stable"}, Components: []string{"main"},
		Architectures: []string{"amd64"}, SigningKey: "remotr-fixture", Priority: 700,
		CredentialRef: "file:/run/remotr/provider-fixture-auth",
	}

	t.Run("present validates and converges canonical owned fragments", func(t *testing.T) {
		provider := newRealAPTRepositoryProvider(t, repository)
		assertAPTRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertAPTRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertAPTRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertAPTRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
		assertAPTAuthMode(t, "/etc/apt/auth.conf.d/remotr-remotr-fixture.conf", 0o600)
		runAPTFixtureUpdate(t)
	})

	t.Run("disabled converges without an active source", func(t *testing.T) {
		disabled := repository
		disabled.Lifecycle = models.LifecycleDisabled
		provider := newRealAPTRepositoryProvider(t, disabled)
		assertAPTRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertAPTRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertAPTRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		content, err := os.ReadFile("/etc/apt/sources.list.d/remotr-remotr-fixture.list")
		if err != nil || strings.Contains(string(content), "\ndeb ") || !strings.HasPrefix(string(content), "# disabled by Remotr\n") {
			t.Fatalf("disabled APT source = %q, %v", content, err)
		}
		runAPTFixtureUpdate(t)
	})

	t.Run("absence removes only owned fragments", func(t *testing.T) {
		absent := models.APTRepository{Name: repository.Name, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}}
		provider := newRealAPTRepositoryProvider(t, absent)
		assertAPTRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertAPTRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertAPTRepositoryCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertAPTRepositoryApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
	})

	absentKey := newRealAPTKeyProvider(t, models.APTSigningKey{
		Name: "remotr-fixture", ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	assertAPTRepositoryApplyStatus(t, absentKey.Apply(t.Context()), contract.Changed)
	assertAPTRepositoryCheckStatus(t, absentKey.Check(t.Context()), contract.Compliant)
	assertUnrelatedAPTMarkers(t, unrelated)

	t.Run("composed trust repository and exact package workflow", func(t *testing.T) {
		keyProvider := aptkeys.New(models.APTSigningKey{
			Name: "remotr-fixture", Source: "https://fixtures.invalid/signing-public.asc", Fingerprint: aptFixtureFingerprint,
		}, nil)
		keyProvider.Fetch = func(context.Context, string) ([]byte, error) {
			return os.ReadFile("/fixtures/keys/signing-public.asc")
		}
		repositoryProvider := aptrepositories.New(repository, nil)
		repositoryProvider.ResolveCredential = func(context.Context, string) (string, error) {
			return "machine 127.0.0.1 login remotr password controlled-fixture", nil
		}
		packageProvider := apt.New(models.Package{Name: "remotr-fixture", Present: true, Version: "1.0.0-1", RefreshCache: true}, nil)
		workflow, err := engine.NewForExecution([]engine.ExecutionResource{
			{Address: "base/remotr-fixture-key", Name: "remotr-fixture", Kind: engine.KindAPTSigningKey, Provider: "apt-key", Handler: keyProvider, LockDomains: []string{"package-manager:apt"}},
			{Address: "base/remotr-fixture-repository", Name: "remotr-fixture", Kind: engine.KindAPTRepository, Provider: "apt-repository", DependsOn: []string{"base/remotr-fixture-key"}, Handler: repositoryProvider, LockDomains: []string{"package-manager:apt"}},
			{Address: "base/remotr-fixture-package", Name: "remotr-fixture", Kind: engine.KindPackage, Provider: "apt", DependsOn: []string{"base/remotr-fixture-repository"}, Handler: packageProvider, LockDomains: []string{"package-manager:apt"}},
		}, executil.SanitizedOSRunner{})
		if err != nil {
			t.Fatal(err)
		}
		result := workflow.ApplyAll(t.Context(), engine.PolicyAuto)
		wantApplied := []string{"base/remotr-fixture-key", "base/remotr-fixture-repository", "base/remotr-fixture-package"}
		if result.Failed != nil {
			direct := packageProvider.ApplyResult(t.Context())
			t.Fatalf("composed APT ApplyAll() failed: %+v; direct package retry=%+v; result=%+v", *result.Failed, direct, result)
		}
		if !slices.Equal(result.Applied, wantApplied) {
			t.Fatalf("composed APT ApplyAll() = %+v, want %v", result, wantApplied)
		}
		if report := workflow.CheckAll(t.Context()); !report.InCompliance {
			t.Fatalf("composed APT second CheckAll() = %+v", report)
		}
		output, err := exec.Command("dpkg-query", "-W", "-f=${Version}", "remotr-fixture").CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != "1.0.0-1" {
			t.Fatalf("exact composed APT package version = %q, %v", strings.TrimSpace(string(output)), err)
		}
		assertUnrelatedAPTMarkers(t, unrelated)
	})
}

func assertAPTRepositoryContainerIdentity(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	distro, release := os.Getenv("REMOTR_PROVIDER_DISTRO"), os.Getenv("REMOTR_PROVIDER_RELEASE")
	if distro == "debian" {
		if release != "12" || !strings.Contains(string(data), "ID=debian") || !strings.Contains(string(data), `VERSION_ID="12"`) {
			t.Fatalf("container identity = %s, want Debian 12", data)
		}
	} else if distro == "ubuntu" {
		if release == "" || !strings.Contains(string(data), "ID=ubuntu") || !strings.Contains(string(data), `VERSION_ID="`+release+`"`) {
			t.Fatalf("container identity = %s, want Ubuntu %s", data, release)
		}
	} else if distro == "pop" {
		if release == "" || !strings.Contains(string(data), "ID=pop") || !strings.Contains(string(data), `VERSION_ID="`+release+`"`) {
			t.Fatalf("container identity = %s, want Pop!_OS %s", data, release)
		}
	} else {
		t.Fatalf("unsupported APT repository container row %q %q", distro, release)
	}
	out, err := exec.Command("uname", "-m").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "x86_64" {
		t.Fatalf("container architecture = %q, err %v; want x86_64", strings.TrimSpace(string(out)), err)
	}
}

func assertAPTRepositoryTooling(t *testing.T) {
	t.Helper()
	for _, name := range []string{"apt-get", "gpg"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("APT repository prerequisite %q is unavailable: %v", name, err)
		}
	}
}

func newRealAPTKeyProvider(t *testing.T, key models.APTSigningKey) contract.Provider {
	t.Helper()
	provider := aptkeys.New(key, nil)
	provider.Fetch = func(context.Context, string) ([]byte, error) {
		return os.ReadFile("/fixtures/keys/signing-public.asc")
	}
	wrapped, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func newRealAPTRepositoryProvider(t *testing.T, repository models.APTRepository) contract.Provider {
	t.Helper()
	provider := aptrepositories.New(repository, nil)
	provider.ResolveCredential = func(context.Context, string) (string, error) {
		return "machine 127.0.0.1 login remotr password controlled-fixture", nil
	}
	wrapped, err := contract.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func runAPTFixtureUpdate(t *testing.T) {
	t.Helper()
	args := []string{
		"-o", "Dir::Etc::sourcelist=/etc/apt/sources.list.d/remotr-remotr-fixture.list",
		"-o", "Dir::Etc::sourceparts=-", "-o", "APT::Get::List-Cleanup=0", "update",
	}
	if out, err := exec.Command("apt-get", args...).CombinedOutput(); err != nil {
		t.Fatalf("validate controlled signed APT repository: %v: %s", err, out)
	}
}

func installUnrelatedAPTMarkers(t *testing.T) map[string]string {
	t.Helper()
	markers := map[string]string{
		"/etc/apt/sources.list.d/provider-matrix-unrelated.keep": "unrelated source\n",
		"/etc/apt/preferences.d/provider-matrix-unrelated":       "Package: unrelated-package\nPin: version 1.0\nPin-Priority: 100\n",
		"/etc/apt/auth.conf.d/provider-matrix-unrelated.conf":    "machine unrelated.invalid login keep password keep\n",
		"/etc/apt/keyrings/provider-matrix-unrelated.gpg":        "unrelated keyring\n",
	}
	for path, content := range markers {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.Contains(path, "auth.conf.d") {
			mode = 0o600
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	return markers
}

func assertUnrelatedAPTMarkers(t *testing.T, markers map[string]string) {
	t.Helper()
	for path, want := range markers {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("unrelated APT configuration %s = %q, %v; want preserved", path, got, err)
		}
	}
}

func removeOwnedAPTFixtureFiles(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		"/etc/apt/keyrings/mismatch.gpg", "/etc/apt/keyrings/remotr-fixture.gpg",
		"/etc/apt/sources.list.d/remotr-remotr-fixture.list",
		"/etc/apt/preferences.d/remotr-remotr-fixture.pref",
		"/etc/apt/auth.conf.d/remotr-remotr-fixture.conf",
	} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func assertAPTAuthMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("APT auth fragment mode = %v, want %v", info.Mode().Perm(), want)
	}
}

func assertAPTRepositoryCheckStatus(t *testing.T, result contract.Observation, want contract.CheckStatus) {
	t.Helper()
	if result.Status != want {
		t.Fatalf("Check() = %+v, want %q", result, want)
	}
}

func assertAPTRepositoryApplyStatus(t *testing.T, result contract.ApplyResult, want contract.ApplyStatus) {
	t.Helper()
	if result.Status != want || result.Err != nil {
		t.Fatalf("Apply() = %+v, want %q without error", result, want)
	}
}
