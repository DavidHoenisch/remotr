//go:build vmsafety

package kernelmodules_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/kernelmodules"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

// OS-AEC-098: runs as root in the disposable Ubuntu kernel-safety VM. It
// proves real loaded, persistent, parameter, and blacklist state, declared,
// root, and network protection, next-boot reporting, native activation-failure
// recovery, unmanaged preservation, idempotent Apply, and second Checks.
func TestKernelModuleProviderContractVM(t *testing.T) {
	const (
		module               = "dummy"
		parameter            = "numdummies"
		parameterValue       = "0"
		unmanagedLoadPath    = "/etc/modules-load.d/98-remotr-vm-unmanaged.conf"
		unmanagedOptionsPath = "/etc/modprobe.d/98-remotr-vm-unmanaged.conf"
	)
	if os.Geteuid() != 0 {
		t.Fatal("kernel-module VM contract must run as root")
	}
	builtInLoaded := true
	builtInProvider := vmKernelModuleProvider(t, models.KernelModuleResource{
		Name: "vm-built-in-loop", Module: "loop", Loaded: &builtInLoaded,
	})
	if check := builtInProvider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("Ubuntu built-in loop Check = %+v, want compliant", check)
	}

	originalLoaded := vmKernelModuleLoaded(t, module)
	originalParameter := ""
	if originalLoaded {
		body, err := os.ReadFile(filepath.Join("/sys/module", module, "parameters", parameter))
		if err != nil {
			t.Fatal(err)
		}
		originalParameter = strings.TrimSpace(string(body))
		vmRunKernelCommand(t, "modprobe", "-r", module)
	}
	managedPaths := []string{
		"/etc/modules-load.d/99-remotr-vm-dummy-qualified.conf",
		"/etc/modprobe.d/99-remotr-vm-dummy-qualified.conf",
		"/etc/modules-load.d/99-remotr-vm-dummy-nextboot.conf",
		"/etc/modprobe.d/99-remotr-vm-dummy-blacklist.conf",
		"/etc/modules-load.d/99-remotr-vm-missing.conf",
	}
	for _, path := range append(append([]string(nil), managedPaths...), unmanagedLoadPath, unmanagedOptionsPath) {
		_ = os.Remove(path)
	}
	t.Cleanup(func() {
		for _, path := range append(append([]string(nil), managedPaths...), unmanagedLoadPath, unmanagedOptionsPath) {
			_ = os.Remove(path)
		}
		if vmKernelModuleLoadedNoFail(module) {
			_ = exec.Command("modprobe", "-r", module).Run()
		}
		if originalLoaded {
			args := []string{module}
			if originalParameter != "" {
				args = append(args, parameter+"="+originalParameter)
			}
			_ = exec.Command("modprobe", args...).Run()
		}
	})
	for path, content := range map[string][]byte{
		unmanagedLoadPath:    []byte("# preserve unmanaged modules-load state\n"),
		unmanagedOptionsPath: []byte("# preserve unmanaged modprobe state\n"),
	} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	loaded, persistent := true, true
	resource := models.KernelModuleResource{
		Name: "vm-dummy-qualified", Module: module, Loaded: &loaded, Persistent: &persistent,
		Parameters: map[string]string{parameter: parameterValue},
	}
	provider := vmKernelModuleProvider(t, resource)
	if check := provider.Check(context.Background()); check.Status != contract.Drifted {
		t.Fatalf("unloaded module Check = %+v, want drifted", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("module load Apply = %+v, want changed", result)
	}
	if !vmKernelModuleLoaded(t, module) {
		t.Fatal("dummy module is not loaded after Apply")
	}
	parameterBody, err := os.ReadFile(filepath.Join("/sys/module", module, "parameters", parameter))
	if err != nil || strings.TrimSpace(string(parameterBody)) != parameterValue {
		t.Fatalf("live dummy parameter = %q, err=%v, want %s", parameterBody, err, parameterValue)
	}
	vmAssertKernelFragment(t, managedPaths[0], module+"\n")
	vmAssertKernelFragment(t, managedPaths[1], "options "+module+" "+parameter+"="+parameterValue+"\n")
	vmAssertKernelSecondCheck(t, provider)

	persistenceOnly := true
	nextBootProvider := vmKernelModuleProvider(t, models.KernelModuleResource{
		Name: "vm-dummy-nextboot", Module: module, Persistent: &persistenceOnly,
	})
	result := nextBootProvider.Apply(context.Background())
	if result.Status != contract.Changed || result.Err != nil || len(result.Activation) != 1 || result.Activation[0].Kind != contract.ActivationNextBoot {
		t.Fatalf("persistence-only Apply = %+v, want next-boot activation", result)
	}
	vmAssertKernelFragment(t, managedPaths[2], module+"\n")
	vmAssertKernelSecondCheck(t, nextBootProvider)

	blacklisted := true
	blacklistResource := models.KernelModuleResource{Name: "vm-dummy-blacklist", Module: module, Blacklisted: &blacklisted}
	blacklistProvider := vmKernelModuleProvider(t, blacklistResource)
	if result := blacklistProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("blacklist Apply = %+v, want changed", result)
	}
	if vmKernelModuleLoaded(t, module) {
		t.Fatal("blacklisted dummy module remains loaded")
	}
	vmAssertKernelFragment(t, managedPaths[3], "blacklist "+module+"\n")
	vmAssertKernelSecondCheck(t, blacklistProvider)
	blacklisted = false
	blacklistResource.Blacklisted = &blacklisted
	unblacklistProvider := vmKernelModuleProvider(t, blacklistResource)
	if result := unblacklistProvider.Apply(context.Background()); result.Status != contract.Changed || result.Err != nil {
		t.Fatalf("blacklist removal Apply = %+v, want changed", result)
	}
	if _, err := os.Stat(managedPaths[3]); !os.IsNotExist(err) {
		t.Fatalf("blacklist fragment remains: %v", err)
	}
	vmAssertKernelSecondCheck(t, unblacklistProvider)
	vmRunKernelCommand(t, "modprobe", module, parameter+"="+parameterValue)

	remove := false
	protectedProvider := vmKernelModuleProvider(t, models.KernelModuleResource{
		Name: "vm-dummy-protected", Module: module, Loaded: &remove, ProtectedModules: []string{module},
	})
	if result := protectedProvider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("declared protected unload Apply = %+v, want failed", result)
	}
	if !vmKernelModuleLoaded(t, module) {
		t.Fatal("declared protected preflight mutated dummy module")
	}

	rootBoundary := t.TempDir()
	rootMountInfo := filepath.Join(rootBoundary, "mountinfo")
	if err := os.WriteFile(rootMountInfo, []byte("24 23 8:1 / / rw,relatime - dummy /dev/vda1 rw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootApplicator := kernelmodules.New(models.KernelModuleResource{Name: "vm-dummy-root", Module: module, Loaded: &remove}, nil)
	rootApplicator.ProcMountInfo = rootMountInfo
	rootApplicator.SysRoot = filepath.Join(rootBoundary, "sys")
	rootProvider, err := contract.New(rootApplicator)
	if err != nil {
		t.Fatal(err)
	}
	if result := rootProvider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("root-module unload Apply = %+v, want failed", result)
	}

	networkBoundary := t.TempDir()
	networkMountInfo := filepath.Join(networkBoundary, "mountinfo")
	if err := os.WriteFile(networkMountInfo, []byte("24 23 8:1 / / rw,relatime - ext4 /dev/vda1 rw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleTarget := filepath.Join(networkBoundary, "sys", "module", module)
	moduleLink := filepath.Join(networkBoundary, "sys", "class", "net", "remotr0", "device", "driver", "module")
	if err := os.MkdirAll(moduleTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(moduleLink), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moduleTarget, moduleLink); err != nil {
		t.Fatal(err)
	}
	networkApplicator := kernelmodules.New(models.KernelModuleResource{Name: "vm-dummy-network", Module: module, Loaded: &remove}, nil)
	networkApplicator.ProcMountInfo = networkMountInfo
	networkApplicator.SysRoot = filepath.Join(networkBoundary, "sys")
	networkProvider, err := contract.New(networkApplicator)
	if err != nil {
		t.Fatal(err)
	}
	if result := networkProvider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("network-module unload Apply = %+v, want failed", result)
	}
	if !vmKernelModuleLoaded(t, module) {
		t.Fatal("root/network preflight mutated dummy module")
	}

	missingLoaded, missingPersistent := true, true
	missingProvider := vmKernelModuleProvider(t, models.KernelModuleResource{
		Name: "vm-missing", Module: "remotr_qualification_missing", Loaded: &missingLoaded, Persistent: &missingPersistent,
	})
	if result := missingProvider.Apply(context.Background()); result.Status != contract.Failed || result.Err == nil {
		t.Fatalf("missing module activation Apply = %+v, want failed", result)
	}
	if _, err := os.Stat(managedPaths[4]); !os.IsNotExist(err) {
		t.Fatalf("failed activation left staged boot fragment: %v", err)
	}

	for path, want := range map[string][]byte{
		unmanagedLoadPath:    []byte("# preserve unmanaged modules-load state\n"),
		unmanagedOptionsPath: []byte("# preserve unmanaged modprobe state\n"),
	} {
		body, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(body, want) {
			t.Fatalf("unmanaged kernel fragment %s changed: %q, err=%v", path, body, err)
		}
	}
}

func vmKernelModuleProvider(t *testing.T, resource models.KernelModuleResource) contract.Provider {
	t.Helper()
	if err := resource.Validate(); err != nil {
		t.Fatal(err)
	}
	provider, err := contract.New(kernelmodules.New(resource, nil))
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func vmAssertKernelSecondCheck(t *testing.T, provider contract.Provider) {
	t.Helper()
	if check := provider.Check(context.Background()); check.Status != contract.Compliant {
		t.Fatalf("kernel-module second Check = %+v, want compliant", check)
	}
	if result := provider.Apply(context.Background()); result.Status != contract.NoChange || result.Err != nil {
		t.Fatalf("compliant kernel-module Apply = %+v, want no change", result)
	}
}

func vmAssertKernelFragment(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil || string(body) != want {
		t.Fatalf("kernel fragment %s = %q, err=%v, want %q", path, body, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("kernel fragment %s mode = %v, err=%v", path, info.Mode().Perm(), err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		t.Fatalf("kernel fragment %s ownership = %T %+v, want root:root", path, info.Sys(), stat)
	}
}

func vmKernelModuleLoaded(t *testing.T, module string) bool {
	t.Helper()
	body, err := os.ReadFile("/proc/modules")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(module, "-", "_")
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.ReplaceAll(fields[0], "-", "_") == want {
			return true
		}
	}
	return false
}

func vmKernelModuleLoadedNoFail(module string) bool {
	body, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	want := strings.ReplaceAll(module, "-", "_")
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.ReplaceAll(fields[0], "-", "_") == want {
			return true
		}
	}
	return false
}

func vmRunKernelCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
	return string(output)
}
