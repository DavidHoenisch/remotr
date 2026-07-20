//go:build providercontainer

package apt_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/apt"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

const (
	fixturePackage = "remotr-fixture"
	fixtureV1      = "1.0.0-1"
	fixtureV2      = "2.0.0-1"
)

// OS-PRM-001, OS-PRM-003 through OS-PRM-006, OS-PRM-010, and OS-PRM-017:
// qualify the APT provider contract against the actual package database and
// signed two-version repository in one exact pinned distribution image.
func TestProviderContainerAPTContract(t *testing.T) {
	assertContainerIdentity(t)
	configureFixtureRepository(t)
	ensurePackageAbsent(t)

	t.Run("drifted apply second check and compliant no change", func(t *testing.T) {
		provider := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV1})
		assertCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
		assertApplyStatus(t, provider.Apply(t.Context()), contract.Changed)
		assertInstalledVersion(t, fixtureV1)
		assertCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
		assertApplyStatus(t, provider.Apply(t.Context()), contract.NoChange)
	})

	t.Run("exact upgrade and downgrade policy", func(t *testing.T) {
		upgrade := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV2})
		assertCheckStatus(t, upgrade.Check(t.Context()), contract.Drifted)
		assertApplyStatus(t, upgrade.Apply(t.Context()), contract.Changed)
		assertInstalledVersion(t, fixtureV2)
		assertCheckStatus(t, upgrade.Check(t.Context()), contract.Compliant)

		blocked := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV1})
		result := blocked.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "downgrade") {
			t.Fatalf("blocked downgrade Apply() = %+v, want retained downgrade failure", result)
		}
		assertInstalledVersion(t, fixtureV2)

		allow := true
		permitted := newRealAPTProvider(t, models.Package{
			Name: fixturePackage, Present: true, Version: fixtureV1, AllowDowngrade: &allow,
		})
		assertApplyStatus(t, permitted.Apply(t.Context()), contract.Changed)
		assertInstalledVersion(t, fixtureV1)
		assertCheckStatus(t, permitted.Check(t.Context()), contract.Compliant)
	})

	t.Run("hold and unhold converge without reinstall", func(t *testing.T) {
		hold := true
		held := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV1, Hold: &hold})
		assertCheckStatus(t, held.Check(t.Context()), contract.Drifted)
		assertApplyStatus(t, held.Apply(t.Context()), contract.Changed)
		assertCheckStatus(t, held.Check(t.Context()), contract.Compliant)
		assertApplyStatus(t, held.Apply(t.Context()), contract.NoChange)
		assertInstalledVersion(t, fixtureV1)

		hold = false
		unheld := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV1, Hold: &hold})
		assertCheckStatus(t, unheld.Check(t.Context()), contract.Drifted)
		assertApplyStatus(t, unheld.Apply(t.Context()), contract.Changed)
		assertCheckStatus(t, unheld.Check(t.Context()), contract.Compliant)
		assertInstalledVersion(t, fixtureV1)
	})

	t.Run("unavailable version fails without changing package database", func(t *testing.T) {
		provider := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: "9.9.9-1"})
		result := provider.Apply(t.Context())
		if result.Status != contract.Failed || result.Err == nil {
			t.Fatalf("unavailable version Apply() = %+v, want retained failure", result)
		}
		assertInstalledVersion(t, fixtureV1)
		assertCheckStatus(t, provider.Check(t.Context()), contract.Drifted)
	})

	t.Run("native lock contention fails without mutation", func(t *testing.T) {
		ensurePackageAbsent(t)
		release := holdDPKGLock(t)
		provider := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV1})
		result := provider.Apply(t.Context())
		release()
		if result.Status != contract.Failed || result.Err == nil {
			t.Fatalf("contended Apply() = %+v, want retained lock failure", result)
		}
		assertPackageAbsent(t)
	})

	t.Run("reboot marker is observable without implicit reboot", func(t *testing.T) {
		if err := os.WriteFile("/var/run/reboot-required", []byte("fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove("/var/run/reboot-required") })
		provider := newRealAPTProvider(t, models.Package{Name: fixturePackage, Present: true, Version: fixtureV1})
		result := provider.Apply(t.Context())
		if result.Status != contract.Changed || result.RebootRequired != contract.RebootRequired ||
			!slices.Contains(result.Activation, executor.ActivationSignal{Kind: executor.ActivationRebootRequired}) {
			t.Fatalf("reboot-marker Apply() = %+v, want changed with observable reboot activation", result)
		}
		assertInstalledVersion(t, fixtureV1)
	})

	t.Run("remove and purge converge", func(t *testing.T) {
		removed := newRealAPTProvider(t, models.Package{
			Name: fixturePackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
		})
		assertCheckStatus(t, removed.Check(t.Context()), contract.Drifted)
		assertApplyStatus(t, removed.Apply(t.Context()), contract.Changed)
		assertCheckStatus(t, removed.Check(t.Context()), contract.Compliant)
		assertApplyStatus(t, removed.Apply(t.Context()), contract.NoChange)

		installFixtureVersion(t, fixtureV1)
		purged := newRealAPTProvider(t, models.Package{
			Name: fixturePackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePurged},
		})
		assertCheckStatus(t, purged.Check(t.Context()), contract.Drifted)
		assertApplyStatus(t, purged.Apply(t.Context()), contract.Changed)
		assertCheckStatus(t, purged.Check(t.Context()), contract.Compliant)
		assertApplyStatus(t, purged.Apply(t.Context()), contract.NoChange)
	})

	assertPackageAbsent(t)
}

// TestProviderContainerDPKGLockHelper is a subprocess helper. It acquires the
// same fcntl lock used by dpkg, signals readiness on fd 3, and releases the
// lock only after its stdin is closed by the parent test.
func TestProviderContainerDPKGLockHelper(t *testing.T) {
	if os.Getenv("REMOTR_DPKG_LOCK_HELPER") != "1" {
		// test-exception: EXC-029
		t.Skip("subprocess helper")
	}
	lock, err := os.OpenFile("/var/lib/dpkg/lock-frontend", os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.FcntlFlock(lock.Fd(), syscall.F_SETLK, &syscall.Flock_t{Type: syscall.F_WRLCK}); err != nil {
		t.Fatal(err)
	}
	ready := os.NewFile(3, "ready")
	if _, err := ready.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	_ = ready.Close()
	_, _ = os.Stdin.Read(make([]byte, 1))
}

func assertContainerIdentity(t *testing.T) {
	t.Helper()
	wantID, wantRelease := os.Getenv("REMOTR_PROVIDER_DISTRO"), os.Getenv("REMOTR_PROVIDER_RELEASE")
	if wantID == "" || wantRelease == "" {
		t.Fatal("container selector requires REMOTR_PROVIDER_DISTRO and REMOTR_PROVIDER_RELEASE")
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]string)
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = strings.Trim(value, "\"")
		}
	}
	if values["ID"] != wantID || values["VERSION_ID"] != wantRelease {
		t.Fatalf("container identity = %s %s, want %s %s", values["ID"], values["VERSION_ID"], wantID, wantRelease)
	}
	if out, err := exec.Command("dpkg", "--print-architecture").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "amd64" {
		t.Fatalf("container architecture = %q, err %v; want amd64", strings.TrimSpace(string(out)), err)
	}
}

func configureFixtureRepository(t *testing.T) {
	t.Helper()
	entries, err := filepath.Glob("/etc/apt/sources.list.d/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if err := os.Remove(entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove("/etc/apt/sources.list"); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	const source = "deb [signed-by=/fixtures/keys/signing-public.asc] file:/fixtures/apt stable main\n"
	if err := os.WriteFile("/etc/apt/sources.list.d/remotr-fixture.list", []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := apt.New(models.Package{Name: fixturePackage, Present: true}, nil)
	if err := provider.RefreshCache(t.Context()); err != nil {
		t.Fatalf("refresh signed fixture repository: %v", err)
	}
}

func newRealAPTProvider(t *testing.T, pkg models.Package) contract.Provider {
	t.Helper()
	provider, err := contract.New(apt.New(pkg, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func installFixtureVersion(t *testing.T, version string) {
	t.Helper()
	allow := true
	provider := newRealAPTProvider(t, models.Package{
		Name: fixturePackage, Present: true, Version: version, AllowUpgrade: &allow, AllowDowngrade: &allow,
	})
	result := provider.Apply(t.Context())
	if result.Status != contract.Changed && result.Status != contract.NoChange {
		t.Fatalf("install fixture %s: %+v", version, result)
	}
}

func ensurePackageAbsent(t *testing.T) {
	t.Helper()
	provider := newRealAPTProvider(t, models.Package{
		Name: fixturePackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePurged},
	})
	result := provider.Apply(t.Context())
	if result.Status != contract.Changed && result.Status != contract.NoChange {
		t.Fatalf("ensure fixture absent: %+v", result)
	}
	assertPackageAbsent(t)
}

func assertPackageAbsent(t *testing.T) {
	t.Helper()
	provider := newRealAPTProvider(t, models.Package{
		Name: fixturePackage, ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
	})
	assertCheckStatus(t, provider.Check(t.Context()), contract.Compliant)
}

func assertInstalledVersion(t *testing.T, want string) {
	t.Helper()
	out, err := exec.Command("dpkg-query", "-W", "-f=${Status}\\t${Version}", fixturePackage).CombinedOutput()
	if err != nil {
		t.Fatalf("query installed version: %v: %s", err, out)
	}
	got := strings.TrimSpace(string(out))
	status, version, ok := strings.Cut(got, "\t")
	fields := strings.Fields(status)
	installed := len(fields) == 3 && (fields[0] == "install" || fields[0] == "hold") &&
		fields[1] == "ok" && fields[2] == "installed"
	if !ok || !installed || version != want {
		t.Fatalf("installed package state = %q, want installed or held version %q", got, want)
	}
	executable, err := exec.Command("remotr-fixture").CombinedOutput()
	if err != nil || strings.TrimSpace(string(executable)) != want {
		t.Fatalf("fixture executable = %q, err %v; want %q", strings.TrimSpace(string(executable)), err, want)
	}
}

func assertCheckStatus(t *testing.T, result contract.Observation, want contract.CheckStatus) {
	t.Helper()
	if result.Status != want {
		t.Fatalf("Check() = %+v, want %q", result, want)
	}
}

func assertApplyStatus(t *testing.T, result contract.ApplyResult, want contract.ApplyStatus) {
	t.Helper()
	if result.Status != want || result.Err != nil {
		t.Fatalf("Apply() = %+v, want %q without error", result, want)
	}
}

func holdDPKGLock(t *testing.T) func() {
	t.Helper()
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestProviderContainerDPKGLockHelper$")
	cmd.Env = append(os.Environ(), "REMOTR_DPKG_LOCK_HELPER=1")
	cmd.ExtraFiles = []*os.File{readyWriter}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output := new(strings.Builder)
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = readyWriter.Close()
	var signal [1]byte
	if _, err := readyReader.Read(signal[:]); err != nil {
		t.Fatalf("wait for dpkg lock helper: %v: %s", err, output.String())
	}
	_ = readyReader.Close()
	return func() {
		_ = stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatalf("dpkg lock helper: %v: %s", err, output.String())
		}
	}
}
