package cron_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cronprovider "github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/cron"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

// OS-ESM-003, OS-ESM-004, OS-ESM-006, OS-ESM-010: the cron provider owns
// one stable fragment, preserves argv boundaries in a protected launcher, and
// leaves unrelated cron fragments untouched.
func TestApplicatorConvergesOwnedCronFragmentAndProtectedLauncher(t *testing.T) {
	root := t.TempDir()
	cronDir := filepath.Join(root, "cron.d")
	stateDir := filepath.Join(root, "state")
	if err := os.MkdirAll(cronDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(cronDir, "unrelated")
	if err := os.WriteFile(unrelated, []byte("0 1 * * * root /usr/bin/true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resource := models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "nightly-backup", Backend: models.ScheduleBackendCron,
		Schedule: "0 3 * * *", User: "backup",
		Argv:             []string{"/usr/local/bin/backup", "daily archive", "quote'boundary"},
		WorkingDirectory: "/var/lib/backup", Timeout: "30m", Overlap: models.ScheduleOverlapForbid,
		Environment: []models.ScheduleEnvironment{{Name: "BACKUP_BUCKET", Value: "daily archive"}, {Name: "BACKUP_TOKEN", SecretRef: "remotr:schedules/backup-token@active"}},
	}
	provider := cronprovider.New(resource)
	provider.CronDir, provider.StateDir, provider.RunDir = cronDir, stateDir, filepath.Join(root, "run")
	provider.BackendAvailable = func() error { return nil }
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ResolveSecret = func(context.Context, string) (string, error) { return "schedule-secret-canary", nil }

	if result := provider.Check(context.Background()); result.Status != executor.Drifted {
		t.Fatalf("Check() = %+v, want drifted", result)
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if result := provider.Check(context.Background()); result.Status != executor.Compliant {
		t.Fatalf("Check() after Apply = %+v, want compliant", result)
	}
	if err := provider.Apply(context.Background()); !errors.Is(err, appErr.ErrStateAlreadyMet) {
		t.Fatalf("second Apply() = %v, want state already met", err)
	}

	fragment, err := os.ReadFile(filepath.Join(cronDir, "remotr-nightly-backup"))
	if err != nil {
		t.Fatal(err)
	}
	wantFragment := "# Managed by Remotr: endpointSchedule/nightly-backup\n0 3 * * * backup " + filepath.Join(stateDir, "nightly-backup.sh") + "\n"
	if string(fragment) != wantFragment {
		t.Fatalf("cron fragment = %q, want %q", fragment, wantFragment)
	}
	launcher, err := os.ReadFile(filepath.Join(stateDir, "nightly-backup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	wantCommand := "exec '/usr/bin/flock' '--nonblock' '" + filepath.Join(root, "run", "nightly-backup.lock") + "' '/usr/bin/timeout' '--signal=TERM' '30m' '--' '/usr/local/bin/backup' 'daily archive' 'quote'\"'\"'boundary'"
	if !strings.Contains(string(launcher), "cd -- '/var/lib/backup'") || !strings.Contains(string(launcher), wantCommand) {
		t.Fatalf("launcher = %q, want cwd and exact argv command %q", launcher, wantCommand)
	}
	if strings.Contains(string(fragment), "schedule-secret-canary") || strings.Contains(string(launcher), "schedule-secret-canary") {
		t.Fatalf("public schedule files leaked secret: fragment=%q launcher=%q", fragment, launcher)
	}
	environment, err := os.ReadFile(filepath.Join(stateDir, "nightly-backup.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(environment), "BACKUP_TOKEN='schedule-secret-canary'") {
		t.Fatalf("protected environment = %q, want resolved secret", environment)
	}
	info, err := os.Stat(filepath.Join(stateDir, "nightly-backup.env"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("environment mode = %v, %v; want 0600", info, err)
	}
	if got, err := os.ReadFile(unrelated); err != nil || string(got) != "0 1 * * * root /usr/bin/true\n" {
		t.Fatalf("unrelated cron changed: %q, %v", got, err)
	}
}

// OS-ESM-010: exercise the generated launcher against the real flock process.
// Pipe synchronization keeps the overlap proof deterministic without sleeps.
func TestApplicatorNonOverlapLauncherRejectsConcurrentOccurrence(t *testing.T) {
	if _, err := exec.LookPath("flock"); err != nil {
		// test-exception: EXC-012
		t.Skip("flock is required for the real cron launcher check")
	}
	root := t.TempDir()
	provider := cronprovider.New(models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "non-overlap", Backend: models.ScheduleBackendCron,
		Schedule: "0 3 * * *", User: "root", Argv: []string{"/bin/sh", "-c", "printf ready >&3; read release <&4"}, Overlap: models.ScheduleOverlapForbid,
	})
	provider.CronDir, provider.StateDir, provider.RunDir = filepath.Join(root, "cron.d"), filepath.Join(root, "state"), filepath.Join(root, "run")
	provider.BackendAvailable = func() error { return nil }
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ResolveSecret = func(context.Context, string) (string, error) { return "", nil }
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(provider.RunDir, 0o755); err != nil {
		t.Fatal(err)
	}

	readyR, readyW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	releaseR, releaseW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	first := exec.Command(filepath.Join(provider.StateDir, "non-overlap.sh")) // #nosec G204 -- provider-owned temp launcher
	first.ExtraFiles = []*os.File{readyW, releaseR}
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	readyW.Close()
	releaseR.Close()
	ready := make([]byte, len("ready"))
	if _, err := readyR.Read(ready); err != nil || string(ready) != "ready" {
		t.Fatalf("first occurrence readiness = %q, %v", ready, err)
	}
	second := exec.Command(filepath.Join(provider.StateDir, "non-overlap.sh")) // #nosec G204 -- provider-owned temp launcher
	if err := second.Run(); err == nil {
		t.Fatal("second occurrence unexpectedly acquired the non-overlap lock")
	}
	if _, err := releaseW.Write([]byte("release\n")); err != nil {
		t.Fatal(err)
	}
	releaseW.Close()
	if err := first.Wait(); err != nil {
		t.Fatalf("first occurrence = %v", err)
	}
}

// OS-ESM-001: the generated launcher is native endpoint state and executes
// even when the configured Remotr server is unreachable.
func TestApplicatorLauncherRunsWithoutServerConnectivity(t *testing.T) {
	root := t.TempDir()
	resultPath := filepath.Join(root, "ran")
	provider := cronprovider.New(models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent}, Name: "offline", Backend: models.ScheduleBackendCron,
		Schedule: "0 3 * * *", User: "root", Argv: []string{"/bin/sh", "-c", "printf local-run > " + resultPath}, Overlap: models.ScheduleOverlapAllow,
	})
	provider.CronDir, provider.StateDir, provider.RunDir = filepath.Join(root, "cron.d"), filepath.Join(root, "state"), filepath.Join(root, "run")
	provider.BackendAvailable = func() error { return nil }
	provider.LookupUser = func(string) (int, int, error) { return os.Getuid(), os.Getgid(), nil }
	provider.ResolveSecret = func(context.Context, string) (string, error) { return "", nil }
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(filepath.Join(provider.StateDir, "offline.sh")) // #nosec G204 -- provider-owned temp launcher
	command.Env = append(os.Environ(), "REMOTR_SERVER_URL=http://127.0.0.1:1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("offline launcher = %v: %s", err, output)
	}
	if got, err := os.ReadFile(resultPath); err != nil || string(got) != "local-run" {
		t.Fatalf("offline result = %q, %v", got, err)
	}
}

// OS-ESM-003: absent removes only the named provider artifacts.
func TestApplicatorAbsentRemovesOnlyOwnedArtifacts(t *testing.T) {
	root := t.TempDir()
	provider := cronprovider.New(models.EndpointScheduleResource{ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent}, Name: "nightly", Backend: models.ScheduleBackendCron})
	provider.CronDir, provider.StateDir = filepath.Join(root, "cron.d"), filepath.Join(root, "state")
	provider.BackendAvailable = func() error { return nil }
	for _, path := range []string{filepath.Join(provider.CronDir, "remotr-nightly"), filepath.Join(provider.StateDir, "nightly.sh"), filepath.Join(provider.StateDir, "nightly.env")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("managed"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(provider.CronDir, "remotr-other")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := provider.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(other); err != nil || string(got) != "keep" {
		t.Fatalf("unrelated provider artifact changed: %q, %v", got, err)
	}
}
