//go:build vmsafety

package osquery_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/queryexec"
	"github.com/DavidHoenisch/remotr/internal/applicators/osquery"
	"github.com/DavidHoenisch/remotr/internal/models"
	contract "github.com/DavidHoenisch/remotr/internal/providercontract"
)

const (
	upstreamOsqueryVersion = "5.14.1"
	upstreamOsqueryURL     = "https://pkg.osquery.io/linux/osquery-5.14.1_1.linux_x86_64.tar.gz"
	upstreamOsquerySHA256  = "3624bd8d5601d852cbf08abc11f77ba2eb2476e94f24ae781d95409d4e8ddad8"
)

// OS-OSQ-003, OS-OSQ-004, OS-OSQ-005, OS-OSQ-006, OS-OSQ-007:
// Runs as root inside a disposable Linux VM (Ubuntu 24.04/26.04, Debian 12,
// Pop!_OS 24.04, Arch Linux) against the real upstream osquery release tarball. Proves:
//  1. Private managed install isolates osqueryi under root:root 0700 so an
//     unprivileged local user gets Permission denied and cannot find it in PATH.
//  2. queryexec executes real live SQL queries against the private osqueryi binary
//     returning real Linux os_version rows.
//  3. Global managed install refuses to clobber a pre-existing unmanaged file at
//     /usr/local/bin/osqueryi.
//  4. Elastic Agent-style mode: existing reuses /opt/Elastic/Agent/.../osqueryd
//     via -S --json --ephemeral --disable_database --disable_events, rejects a
//     0777 writable binary, and never deletes the external binary on absent.
//  5. Managed absent removes only Remotr's private install directory.
func TestOsqueryProviderAndQueryExecVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Fatal("osquery VM contract must run as root")
	}

	const unprivUser = "remotr-vm-osq-user"
	const privateDir = "/usr/lib/remotr/osquery"
	const runtimeStatePath = "/var/lib/remotr/osquery-runtime.json"
	const globalSymlink = "/usr/local/bin/osqueryi"
	const elasticRoot = "/opt/Elastic"
	const elasticBin = "/opt/Elastic/Agent/data/elastic-agent-8.16.0-vm/components/osqueryd"

	cleanup := func() {
		_ = os.RemoveAll(privateDir)
		_ = os.Remove(runtimeStatePath)
		_ = os.Remove(globalSymlink)
		_ = os.RemoveAll(elasticRoot)
		if _, err := user.Lookup(unprivUser); err == nil {
			_ = exec.Command("userdel", "--remove", "--force", "--", unprivUser).Run()
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	if out, err := exec.Command("useradd", "--create-home", "--shell", "/bin/sh", "--", unprivUser).CombinedOutput(); err != nil {
		t.Fatalf("create %s: %v: %s", unprivUser, err, out)
	}

	// 1. Managed private install from real upstream osquery tarball.
	privRes := models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-private-osquery",
		Mode:         models.OsqueryModeManaged,
		Visibility:   models.OsqueryVisibilityPrivate,
		InstallDir:   privateDir,
		Version:      upstreamOsqueryVersion,
		SourceURL:    upstreamOsqueryURL,
		SHA256:       upstreamOsquerySHA256,
	}
	privApp := osquery.New(privRes)
	privApp.RuntimeStatePath = runtimeStatePath
	prov, err := contract.New(privApp)
	if err != nil {
		t.Fatal(err)
	}
	if chk := prov.Check(context.Background()); chk.Status != contract.Drifted {
		t.Fatalf("initial private Check = %+v, want drifted", chk)
	}
	if ar := prov.Apply(context.Background()); ar.Status != contract.Changed || ar.Err != nil {
		t.Fatalf("private Apply = %+v, want changed", ar)
	}
	if chk := prov.Check(context.Background()); chk.Status != contract.Compliant {
		t.Fatalf("second private Check = %+v, want compliant", chk)
	}
	if ar := prov.Apply(context.Background()); ar.Status != contract.NoChange || ar.Err != nil {
		t.Fatalf("idempotent private Apply = %+v, want no-change", ar)
	}

	// Verify root:root 0700 permissions on private directory and binary.
	privBin := filepath.Join(privateDir, "bin", "osqueryi")
	for _, p := range []string{privateDir, privBin} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s perm = %04o, want 0700", p, info.Mode().Perm())
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || st.Gid != 0 {
			t.Fatalf("%s owner = %+v, want root:root", p, st)
		}
	}

	// Verify real unprivileged Linux user CANNOT access or execute private osqueryi.
	if out, err := exec.Command("su", "-s", "/bin/sh", "-c", privBin+" --version", unprivUser).CombinedOutput(); err == nil {
		t.Fatalf("unprivileged user unexpectedly executed private osqueryi: %s", out)
	}
	if out, err := exec.Command("su", "-s", "/bin/sh", "-c", "command -v osqueryi", unprivUser).CombinedOutput(); err == nil {
		t.Fatalf("private osqueryi leaked onto user PATH: %s", out)
	}

	// 2. Execute REAL live SQL query via queryexec against private osqueryi.
	execEngine := &queryexec.Executor{RuntimeStatePath: runtimeStatePath}
	res := execEngine.Execute(context.Background(), queryexec.Request{
		RunID:   "vm-live-1",
		Name:    "os-version",
		SQL:     "SELECT name, version, platform FROM os_version",
		Timeout: 15 * time.Second,
		MaxRows: 10,
	})
	if res.Status != queryexec.StatusSuccess || len(res.Rows) != 1 {
		t.Fatalf("live osqueryi execution = %+v, want success with 1 row", res)
	}
	t.Logf("live os_version row = %+v", res.Rows[0])
	if strings.TrimSpace(res.Rows[0]["name"]) == "" || strings.TrimSpace(res.Rows[0]["platform"]) == "" {
		t.Fatalf("unexpected os_version row = %+v", res.Rows[0])
	}

	// 3. Global managed install refuses to clobber unmanaged /usr/local/bin/osqueryi.
	const foreignMarker = "#!/bin/sh\necho unmanaged-foreign-tool\n"
	if err := os.WriteFile(globalSymlink, []byte(foreignMarker), 0o755); err != nil {
		t.Fatal(err)
	}
	globApp := osquery.New(models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-global-clobber",
		Mode:         models.OsqueryModeManaged,
		Visibility:   models.OsqueryVisibilityGlobal,
		SymlinkPath:  globalSymlink,
		Version:      upstreamOsqueryVersion,
		SourceURL:    upstreamOsqueryURL,
		SHA256:       upstreamOsquerySHA256,
	})
	globApp.RuntimeStatePath = runtimeStatePath
	if err := globApp.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("global Apply() error = %v, want anti-clobber unmanaged error", err)
	}
	if got, err := os.ReadFile(globalSymlink); err != nil || string(got) != foreignMarker {
		t.Fatalf("unmanaged global binary was modified: %q, err=%v", string(got), err)
	}

	// 4. Elastic Agent-style mode: existing reusing real osqueryd binary with -S.
	if err := os.MkdirAll(filepath.Dir(elasticBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFileExact(privBin, elasticBin, 0o755); err != nil {
		t.Fatalf("stage elastic osqueryd: %v", err)
	}
	existingRes := models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "vm-elastic-existing",
		Mode:         models.OsqueryModeExisting,
		AllowedOwner: "root",
		BinaryPaths:  []string{"/opt/Elastic/Agent/data/elastic-agent-*/components/osqueryd"},
	}
	existingApp := osquery.New(existingRes)
	existingApp.RuntimeStatePath = runtimeStatePath
	if err := existingApp.Apply(context.Background()); err != nil {
		t.Fatalf("existing Apply() error = %v", err)
	}
	resElastic := execEngine.Execute(context.Background(), queryexec.Request{
		RunID:   "vm-live-elastic-1",
		Name:    "system-info",
		SQL:     "SELECT hostname, cpu_logical_cores FROM system_info",
		Timeout: 15 * time.Second,
		MaxRows: 10,
	})
	if resElastic.Status != queryexec.StatusSuccess || len(resElastic.Rows) != 1 || resElastic.Rows[0]["hostname"] == "" {
		t.Fatalf("live elastic osqueryd -S execution = %+v, want success with hostname", resElastic)
	}

	// Absent on existing mode MUST preserve Elastic Agent's binary, and disable queryexec.
	absentExisting := existingRes
	absentExisting.Lifecycle = models.LifecycleAbsent
	absentExistingApp := osquery.New(absentExisting)
	absentExistingApp.RuntimeStatePath = runtimeStatePath
	if err := absentExistingApp.Apply(context.Background()); err != nil {
		t.Fatalf("existing absent Apply() error = %v", err)
	}
	if _, err := os.Stat(elasticBin); err != nil {
		t.Fatalf("external Elastic Agent osqueryd was removed: %v", err)
	}
	unsupportedRes := execEngine.Execute(context.Background(), queryexec.Request{
		RunID: "vm-live-disabled",
		Name:  "os-version",
		SQL:   "SELECT version FROM os_version",
	})
	if unsupportedRes.Status != queryexec.StatusUnsupported {
		t.Fatalf("disabled runtime Execute = %+v, want unsupported", unsupportedRes)
	}

	// World-writable external binary MUST be rejected.
	if err := os.Chmod(elasticBin, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := existingApp.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("world-writable existing Apply() error = %v, want writable rejection", err)
	}

	// 5. Managed absent removes only Remotr's private install directory.
	absentPriv := privRes
	absentPriv.Lifecycle = models.LifecycleAbsent
	absentPrivApp := osquery.New(absentPriv)
	absentPrivApp.RuntimeStatePath = runtimeStatePath
	if err := absentPrivApp.Apply(context.Background()); err != nil {
		t.Fatalf("managed absent Apply() error = %v", err)
	}
	if _, err := os.Stat(privateDir); !os.IsNotExist(err) {
		t.Fatalf("privateDir still exists after managed absent: %v", err)
	}
}

func copyFileExact(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
