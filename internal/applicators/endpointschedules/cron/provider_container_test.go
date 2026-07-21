//go:build providerintegration

package cron_test

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	cronprovider "github.com/DavidHoenisch/remotr/internal/applicators/endpointschedules/cron"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-ESM-001, OS-ESM-003, OS-ESM-004, OS-ESM-006, OS-ESM-010 /
// OS-AEC-097: prove the cron contract on the pinned Ubuntu image with the real
// backend, user identity, generated launcher, timeout/flock tools, lifecycle,
// protected environment, offline execution, and second Check.
func TestCronProviderUbuntuContainer(t *testing.T) {
	if os.Geteuid() != 0 {
		// test-exception: EXC-037
		t.Skip("cron container contract requires root to execute as the declared user")
	}
	if _, err := exec.LookPath("cron"); err != nil {
		t.Fatalf("native Ubuntu cron backend is unavailable: %v", err)
	}
	entry, err := user.Lookup("nobody")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := strconv.Atoi(entry.Uid)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := strconv.Atoi(entry.Gid)
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join("/tmp", "remotr-cron-provider-container")
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	workDir := filepath.Join(root, "work")
	for _, path := range []string{root, workDir} {
		if err := os.MkdirAll(path, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	secretPath := filepath.Join(root, "secret")
	const secretCanary = "ubuntu-cron-secret-canary"
	if err := os.WriteFile(secretPath, []byte(secretCanary+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(workDir, "argv-result")
	resource := models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "ubuntu-cron", Backend: models.ScheduleBackendCron,
		Schedule: "17 4 * * *", User: "nobody",
		Argv:             []string{"/bin/sh", "-c", "printf '%s|%s' \"$PUBLIC_VALUE\" \"$SECRET_VALUE\" > " + resultPath},
		WorkingDirectory: workDir, Timeout: "30s", Overlap: models.ScheduleOverlapForbid,
		Environment: []models.ScheduleEnvironment{
			{Name: "PUBLIC_VALUE", Value: "argv boundary"},
			{Name: "SECRET_VALUE", SecretRef: "file:" + secretPath},
		},
	}
	provider := newUbuntuCronProvider(resource, root)
	adapted := adaptCron(t, provider)
	if check := adapted.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", check)
	}
	if result := adapted.Apply(context.Background()); result.Status != contract.Changed {
		t.Fatalf("Apply = %+v, want changed", result)
	}
	if check := adapted.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("second Check = %+v, want compliant", check)
	}
	if result := adapted.Apply(context.Background()); result.Status != contract.NoChange {
		t.Fatalf("second Apply = %+v, want no-change", result)
	}

	fragment, err := os.ReadFile(filepath.Join(provider.CronDir, "remotr-ubuntu-cron"))
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := os.ReadFile(filepath.Join(provider.StateDir, "ubuntu-cron.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fragment), secretCanary) || strings.Contains(string(launcher), secretCanary) {
		t.Fatalf("public cron state leaked secret: fragment=%q launcher=%q", fragment, launcher)
	}
	command := exec.Command(filepath.Join(provider.StateDir, "ubuntu-cron.sh")) // #nosec G204 -- provider-owned fixed test path
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}}
	command.Env = append(os.Environ(), "REMOTR_SERVER_URL=http://127.0.0.1:1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("non-root offline launcher = %v: %s", err, output)
	}
	if contents, err := os.ReadFile(resultPath); err != nil || string(contents) != "argv boundary|"+secretCanary {
		t.Fatalf("argv launcher result = %q, %v", contents, err)
	}

	disabled := resource
	disabled.Lifecycle = models.LifecycleDisabled
	disabledProvider := newUbuntuCronProvider(disabled, root)
	if result := adaptCron(t, disabledProvider).Apply(context.Background()); result.Status != contract.Changed {
		t.Fatalf("disable Apply = %+v, want changed", result)
	}
	if _, err := os.Stat(filepath.Join(provider.CronDir, "remotr-ubuntu-cron")); !os.IsNotExist(err) {
		t.Fatalf("disabled cron fragment still active: %v", err)
	}
	if _, err := os.Stat(filepath.Join(provider.StateDir, "ubuntu-cron.sh")); err != nil {
		t.Fatalf("disabled cron support state was removed: %v", err)
	}

	absent := models.EndpointScheduleResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecycleAbsent},
		Name:         "ubuntu-cron", Backend: models.ScheduleBackendCron,
	}
	absentProvider := newUbuntuCronProvider(absent, root)
	absentAdapted := adaptCron(t, absentProvider)
	if result := absentAdapted.Apply(context.Background()); result.Status != contract.Changed {
		t.Fatalf("absence Apply = %+v, want changed", result)
	}
	if check := absentAdapted.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("absence Check = %+v, want compliant", check)
	}
	for _, path := range []string{
		filepath.Join(provider.StateDir, "ubuntu-cron.sh"),
		filepath.Join(provider.StateDir, "ubuntu-cron.env"),
		filepath.Join(provider.RunDir, "ubuntu-cron.lock"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("owned cron state %s survived absence: %v", path, err)
		}
	}
}

func newUbuntuCronProvider(resource models.EndpointScheduleResource, root string) *cronprovider.Applicator {
	provider := cronprovider.New(resource)
	provider.CronDir = filepath.Join(root, "cron.d")
	provider.StateDir = filepath.Join(root, "state")
	provider.RunDir = filepath.Join(root, "run")
	return provider
}
