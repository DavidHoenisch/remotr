//go:build vmsafety

package sysctl_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/sysctl"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: runs as root in the disposable Ubuntu system-safety VM. It
// proves independent runtime and persistent scopes, provider-owned drop-ins,
// unsupported keys, native reload, next-boot signaling, preservation,
// idempotent Apply, and compliant second Checks.
func TestSysctlProviderContractVM(t *testing.T) {
	const (
		key           = "net.ipv4.ip_forward"
		unmanagedPath = "/etc/sysctl.d/98-remotr-vm-unmanaged.conf"
	)
	if os.Geteuid() != 0 {
		t.Fatal("sysctl VM contract must run as root")
	}
	runtimePath := "/proc/sys/net/ipv4/ip_forward"
	originalBody, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	original := strings.TrimSpace(string(originalBody))
	alternate := "1"
	if original == alternate {
		alternate = "0"
	}
	managedNames := []string{"vm-runtime", "vm-persistent", "vm-reload", "vm-next-boot"}
	managedPaths := make([]string, 0, len(managedNames))
	for _, name := range managedNames {
		managedPaths = append(managedPaths, filepath.Join("/etc/sysctl.d", "99-remotr-"+name+".conf"))
	}
	for _, path := range append(append([]string(nil), managedPaths...), unmanagedPath) {
		_ = os.Remove(path)
	}
	t.Cleanup(func() {
		_, _ = exec.Command("sysctl", "-w", key+"="+original).CombinedOutput()
		for _, path := range append(append([]string(nil), managedPaths...), unmanagedPath) {
			_ = os.Remove(path)
		}
	})
	if err := os.WriteFile(unmanagedPath, []byte("# preserve this unmanaged fragment\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runtime := models.SysctlResource{Name: "vm-runtime", Key: key, Value: alternate, Runtime: true}
	runtimeProvider := vmSysctlProvider(t, runtime)
	if check := runtimeProvider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("runtime drift Check = %+v, want drifted", check)
	}
	if result := runtimeProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("runtime Apply = %+v, want changed", result)
	}
	vmAssertSysctlRuntime(t, runtimePath, alternate)
	if _, err := os.Stat(managedPaths[0]); !os.IsNotExist(err) {
		t.Fatalf("runtime-only resource wrote a persistent fragment: %v", err)
	}
	vmAssertSysctlSecondCheck(t, runtimeProvider)

	persistent := models.SysctlResource{Name: "vm-persistent", Key: key, Value: original, Persistent: true}
	persistentProvider := vmSysctlProvider(t, persistent)
	if check := persistentProvider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("persistent drift Check = %+v, want drifted", check)
	}
	if result := persistentProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("persistent Apply = %+v, want changed", result)
	}
	vmAssertSysctlRuntime(t, runtimePath, alternate)
	vmAssertSysctlDropIn(t, managedPaths[1], key+" = "+original+"\n")
	if err := os.Chmod(managedPaths[1], 0o600); err != nil {
		t.Fatal(err)
	}
	if check := persistentProvider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("persistent metadata Check = %+v, want drifted", check)
	}
	if result := persistentProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("persistent metadata repair = %+v, want changed", result)
	}
	vmAssertSysctlDropIn(t, managedPaths[1], key+" = "+original+"\n")
	vmAssertSysctlSecondCheck(t, persistentProvider)

	unsupported := vmSysctlProvider(t, models.SysctlResource{
		Name: "vm-unsupported", Key: "kernel.remotr_qualification_missing", Value: "1", Runtime: true,
	})
	if check := unsupported.Check(context.Background()); check.Status != contract.Unsupported || check.ReasonCode != "sysctl_key_unsupported" {
		t.Fatalf("unsupported key Check = %+v, want unsupported", check)
	}

	reload := models.SysctlResource{
		Name: "vm-reload", Key: key, Value: original, Persistent: true, Activation: models.SysctlReload,
	}
	reloadProvider := vmSysctlProvider(t, reload)
	if result := reloadProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("native reload Apply = %+v, want changed", result)
	}
	vmAssertSysctlRuntime(t, runtimePath, original)
	vmAssertSysctlDropIn(t, managedPaths[2], key+" = "+original+"\n")
	vmAssertSysctlSecondCheck(t, reloadProvider)

	nextBoot := models.SysctlResource{
		Name: "vm-next-boot", Key: key, Value: alternate, Persistent: true, Activation: models.SysctlNextBoot,
	}
	nextBootProvider := vmSysctlProvider(t, nextBoot)
	result := nextBootProvider.Apply(context.Background())
	if result.Status != contract.Changed || result.Err != nil || len(result.Activation) != 1 || result.Activation[0].Kind != contract.ActivationNextBoot {
		t.Fatalf("next-boot Apply = %+v, want changed with next-boot activation", result)
	}
	vmAssertSysctlRuntime(t, runtimePath, original)
	vmAssertSysctlDropIn(t, managedPaths[3], key+" = "+alternate+"\n")
	vmAssertSysctlSecondCheck(t, nextBootProvider)

	unmanaged, err := os.ReadFile(unmanagedPath)
	if err != nil || !bytes.Equal(unmanaged, []byte("# preserve this unmanaged fragment\n")) {
		t.Fatalf("unmanaged sysctl fragment changed: %q, err=%v", unmanaged, err)
	}
}

func vmSysctlProvider(t *testing.T, resource models.SysctlResource) contract.Provider {
	t.Helper()
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(sysctl.New(resource, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmAssertSysctlSecondCheck(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("sysctl second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant sysctl Apply = %+v, want no change", result)
	}
}

func vmAssertSysctlRuntime(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(body)) != want {
		t.Fatalf("runtime sysctl = %q, err=%v, want %q", body, err, want)
	}
}

func vmAssertSysctlDropIn(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || string(body) != want {
		t.Fatalf("sysctl drop-in %s = %q, err=%v, want %q", path, body, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("sysctl drop-in %s mode = %v, err=%v", path, info.Mode().Perm(), err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("sysctl drop-in %s ownership = %T %+v, want root:root", path, info.Sys(), stat)
	}
}
