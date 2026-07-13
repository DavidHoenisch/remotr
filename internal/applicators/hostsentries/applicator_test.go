package hostsentries

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestApplicatorPreservesUnrelatedHostsContentAcrossLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	unrelated := "127.0.0.1 localhost\n192.0.2.20 unmanaged.example # keep this comment\n# local footer\n"
	if err := os.WriteFile(path, []byte(unrelated), 0o644); err != nil {
		t.Fatal(err)
	}

	resource := models.HostsEntryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "api", Address: "203.0.113.10", CanonicalHost: "api.example", Aliases: []string{"api.internal"},
	}
	a := New(resource)
	a.Path = path
	if check := a.Check(context.Background()); check.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v", check)
	}
	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantPresent := unrelated + "203.0.113.10 api.example api.internal # remotr:api\n"
	assertHostsContent(t, path, wantPresent)
	if check := a.Check(context.Background()); check.Status != executor.Compliant {
		t.Fatalf("second Check() = %+v", check)
	}

	resource.Aliases = []string{"api.internal", "api"}
	updated := New(resource)
	updated.Path = path
	if err := updated.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertHostsContent(t, path, unrelated+"203.0.113.10 api.example api.internal api # remotr:api\n")

	resource.Lifecycle = models.LifecycleAbsent
	resource.Address = ""
	resource.CanonicalHost = ""
	resource.Aliases = nil
	absent := New(resource)
	absent.Path = path
	if err := absent.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertHostsContent(t, path, unrelated)
}

func TestApplicatorRejectsHostsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("127.0.0.1 localhost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hosts")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	a := New(models.HostsEntryResource{Name: "api", Address: "203.0.113.10", CanonicalHost: "api.example"})
	a.Path = path
	err := a.Apply(context.Background())
	if err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestApplicatorPreflightBlocksActiveRemotrDestination(t *testing.T) {
	a := New(models.HostsEntryResource{Name: "api", Address: "203.0.113.10", CanonicalHost: "server.example"})
	a.SyncURL = "https://server.example:8443/api/v1/sync"
	if err := a.Preflight(context.Background()); err == nil || !strings.Contains(err.Error(), "active Remotr destination") {
		t.Fatalf("Preflight() error = %v", err)
	}
}

func assertHostsContent(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("hosts content:\n%s\nwant:\n%s", raw, want)
	}
}
