//go:build vmsafety

package hostsentries_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/hostsentries"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
	"github.com/DavidHoenisch/remotr/internal/resourceregistry"
)

// OS-AEC-098: exercise ResourceKindHostsEntry through the registry against
// Ubuntu's real /etc/hosts and resolver. The fixture proves stable marked
// lifecycle, configured and effective observation, unrelated-content and
// metadata preservation, active-control-host planning, removal, second Check,
// and no-change second Apply behavior.
func TestHostsEntryProviderVM(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-023
		t.Skip("hosts-entry VM test runs as root in the isolated Vagrant guest")
	}
	ctx := context.Background()
	path := "/etc/hosts"
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	t.Cleanup(func() {
		if err := os.WriteFile(path, original, info.Mode().Perm()); err != nil {
			t.Errorf("restore /etc/hosts: %v", err)
			return
		}
		if err := os.Chown(path, int(stat.Uid), int(stat.Gid)); err != nil {
			t.Errorf("restore /etc/hosts ownership: %v", err)
		}
	})

	resource := models.HostsEntryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "ubuntu-vm-hosts", Address: "192.0.2.44", CanonicalHost: "remotr-hosts-entry.invalid",
		Aliases: []string{"remotr-hosts-alias.invalid"},
	}
	present := vmHostsEntryProvider(t, resource)
	if check := present.Check(ctx); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if result := present.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("present Apply = %+v, want changed", result)
	}
	vmAssertHostsEntryLine(t, path, "192.0.2.44 remotr-hosts-entry.invalid remotr-hosts-alias.invalid # remotr:ubuntu-vm-hosts")
	if check := present.Check(ctx); !vmHostsEntryCheckIsEffective(check, resource.Address) {
		t.Fatalf("configured/effective Check = %+v, want compliant effective address", check)
	}
	vmAssertHostsEntrySecondPass(t, present)

	beforePreflight, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	guarded := hostsentries.New(resource)
	guarded.SyncURL = "https://remotr-hosts-entry.invalid:8443/v1/sync"
	if err := guarded.Preflight(ctx); err == nil || !strings.Contains(err.Error(), "active Remotr destination") {
		t.Fatalf("active-control-host Preflight error = %v", err)
	}
	afterPreflight, err := os.ReadFile(path)
	if err != nil || string(afterPreflight) != string(beforePreflight) {
		t.Fatalf("preflight mutated /etc/hosts: %v", err)
	}

	resource.Address = "192.0.2.45"
	resource.Aliases = []string{"remotr-hosts-alias.invalid", "remotr-hosts-updated.invalid"}
	updated := vmHostsEntryProvider(t, resource)
	if result := updated.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("updated Apply = %+v, want changed", result)
	}
	vmAssertHostsEntryLine(t, path, "192.0.2.45 remotr-hosts-entry.invalid remotr-hosts-alias.invalid remotr-hosts-updated.invalid # remotr:ubuntu-vm-hosts")
	vmAssertHostsEntrySecondPass(t, updated)

	absentResource := models.HostsEntryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: resource.Name,
	}
	absent := vmHostsEntryProvider(t, absentResource)
	if result := absent.Apply(ctx); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("absent Apply = %+v, want changed", result)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(original) {
		t.Fatalf("/etc/hosts after removal differs from original: %v", err)
	}
	if current, err := os.Stat(path); err != nil || current.Mode().Perm() != info.Mode().Perm() || current.Sys().(*syscall.Stat_t).Uid != stat.Uid || current.Sys().(*syscall.Stat_t).Gid != stat.Gid {
		t.Fatalf("/etc/hosts metadata after lifecycle = %+v, %v", current, err)
	}
	vmAssertHostsEntrySecondPass(t, absent)
}

func vmHostsEntryProvider(t *testing.T, resource models.HostsEntryResource) contract.Provider {
	t.Helper()
	registry, err := resourceregistry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	resources, err := registry.Resources(&models.Configuration{HostsEntries: []models.HostsEntryResource{resource}})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range resources {
		if candidate.Kind() != models.ResourceKindHostsEntry {
			continue
		}
		if err := candidate.Validate(); err != nil {
			t.Fatal(err)
		}
		handler, err := candidate.NewProvider(resourceregistry.FactoryContext{SyncURL: "https://remotr-control.invalid:8443/v1/sync"})
		if err != nil {
			t.Fatal(err)
		}
		provider, err := contract.New(handler)
		if err != nil {
			t.Fatal(err)
		}
		return provider
	}
	t.Fatal("hostsEntry resource is absent from the registry")
	return nil
}

func vmAssertHostsEntrySecondPass(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != executor.NoChange || result.Err != nil {
		t.Fatalf("second Apply = %+v, want no change", result)
	}
}

func vmAssertHostsEntryLine(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- fixed VM fixture path
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.Contains(line, "# remotr:ubuntu-vm-hosts") {
			count++
			if line != want {
				t.Fatalf("managed hosts line = %q, want %q", line, want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("managed hosts line count = %d, want 1", count)
	}
}

func vmHostsEntryCheckIsEffective(check contract.Observation, wantAddress string) bool {
	if check.Status != contract.Compliant {
		return false
	}
	actual, ok := check.Actual.(map[string]any)
	if !ok {
		return false
	}
	effective, ok := actual["effective"].([]string)
	if !ok {
		return false
	}
	for _, address := range effective {
		if address == wantAddress {
			return true
		}
	}
	return false
}
