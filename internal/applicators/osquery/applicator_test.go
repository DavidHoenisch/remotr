package osquery_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/osquery"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func currentUsername(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return u.Username
}

// OS-OSQ-003: managed private installation verifies SHA-256, isolates binary
// inside a 0700 directory outside user PATH, persists runtime descriptor, and
// removes only Remotr-owned state on lifecycle: absent.
func TestOsqueryApplicator_ManagedPrivateLifecycleAndChecksum(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "private-osquery")
	stateFile := filepath.Join(root, "state", "osquery-runtime.json")
	payload := []byte("#!/bin/sh\necho '[]'\n")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	// Bad checksum must fail without creating installDir binary.
	badRes := models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "priv-bad",
		Mode:         models.OsqueryModeManaged,
		Visibility:   models.OsqueryVisibilityPrivate,
		InstallDir:   installDir,
		Version:      "5.14.1",
		SourceURL:    "https://downloads.example.test/osqueryi",
		SHA256:       strings.Repeat("f", 64),
	}
	badApp := osquery.New(badRes)
	badApp.RuntimeStatePath = stateFile
	badApp.Fetch = func(context.Context, string) ([]byte, error) { return payload, nil }
	if err := badApp.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Apply(bad checksum) error = %v, want sha256 mismatch", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "bin", "osqueryi")); !os.IsNotExist(err) {
		t.Fatalf("binary should not exist after failed checksum: %v", err)
	}

	// Valid checksum installs privately with 0700 dir & 0700 binary.
	res := badRes
	res.Name = "priv-ok"
	res.SHA256 = hexSum
	app := osquery.New(res)
	app.RuntimeStatePath = stateFile
	app.Fetch = func(context.Context, string) ([]byte, error) { return payload, nil }

	if chk := app.Check(context.Background()); chk.Status != executor.Drifted {
		t.Fatalf("initial Check = %+v, want drifted", chk)
	}
	if ar := app.ApplyResult(context.Background()); ar.Status != executor.Changed || ar.Err != nil {
		t.Fatalf("ApplyResult = %+v, want changed", ar)
	}
	if chk := app.Check(context.Background()); chk.Status != executor.Compliant {
		t.Fatalf("second Check = %+v, want compliant", chk)
	}

	dirInfo, err := os.Stat(installDir)
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("installDir perm = %o, err=%v, want 0700", dirInfo.Mode().Perm(), err)
	}
	binPath := filepath.Join(installDir, "bin", "osqueryi")
	binInfo, err := os.Stat(binPath)
	if err != nil || binInfo.Mode().Perm() != 0o700 {
		t.Fatalf("binary perm = %o, err=%v, want 0700", binInfo.Mode().Perm(), err)
	}

	rt, err := osquery.LoadRuntimeState(stateFile)
	if err != nil {
		t.Fatalf("LoadRuntimeState() error = %v", err)
	}
	if !rt.Enabled || rt.BinaryPath != binPath || rt.UseDaemonShell || rt.Mode != models.OsqueryModeManaged || rt.Visibility != models.OsqueryVisibilityPrivate {
		t.Fatalf("runtime state = %+v", rt)
	}

	// Absent removes only Remotr-owned directory.
	absentRes := res
	absentRes.Lifecycle = models.LifecycleAbsent
	absentApp := osquery.New(absentRes)
	absentApp.RuntimeStatePath = stateFile
	if ar := absentApp.ApplyResult(context.Background()); ar.Status != executor.Changed || ar.Err != nil {
		t.Fatalf("absent ApplyResult = %+v, want changed", ar)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("installDir still exists after absent: %v", err)
	}
}

// OS-OSQ-004: global managed installation refuses to clobber an unmanaged file
// or symlink at symlinkPath.
func TestOsqueryApplicator_ManagedGlobalRefusesToClobberUnmanagedBinary(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "opt-osquery")
	symlinkPath := filepath.Join(root, "usr-local-bin", "osqueryi")
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const foreignContent = "#!/bin/sh\necho foreign\n"
	if err := os.WriteFile(symlinkPath, []byte(foreignContent), 0o755); err != nil {
		t.Fatal(err)
	}

	payload := []byte("#!/bin/sh\necho '[]'\n")
	sum := sha256.Sum256(payload)
	res := models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "glob",
		Mode:         models.OsqueryModeManaged,
		Visibility:   models.OsqueryVisibilityGlobal,
		InstallDir:   installDir,
		SymlinkPath:  symlinkPath,
		Version:      "5.14.1",
		SourceURL:    "https://downloads.example.test/osqueryi",
		SHA256:       hex.EncodeToString(sum[:]),
	}
	app := osquery.New(res)
	app.RuntimeStatePath = filepath.Join(root, "state", "osquery-runtime.json")
	app.Fetch = func(context.Context, string) ([]byte, error) { return payload, nil }

	if err := app.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("Apply() error = %v, want unmanaged anti-clobber error", err)
	}
	got, err := os.ReadFile(symlinkPath)
	if err != nil || string(got) != foreignContent {
		t.Fatalf("unmanaged file at symlinkPath was clobbered: %q, err=%v", string(got), err)
	}
}

// OS-OSQ-005 & OS-OSQ-006: existing mode safely resolves Elastic Agent-style
// anchored patterns (detecting osqueryd -S), rejects world/group-writable
// binaries, never modifies external binaries, and disabled mode turns off runtime.
func TestOsqueryApplicator_ExistingModeAndDisabledMode(t *testing.T) {
	root := t.TempDir()
	stateFile := filepath.Join(root, "state", "osquery-runtime.json")
	elasticBin := filepath.Join(root, "Elastic", "Agent", "data", "elastic-agent-8.15.0-abc123", "components", "osqueryd")
	if err := os.MkdirAll(filepath.Dir(elasticBin), 0o755); err != nil {
		t.Fatal(err)
	}
	const elasticContent = "#!/bin/sh\necho '[]'\n"
	if err := os.WriteFile(elasticBin, []byte(elasticContent), 0o755); err != nil {
		t.Fatal(err)
	}

	pattern := filepath.Join(root, "Elastic", "Agent", "data", "elastic-agent-*", "components", "osqueryd")
	res := models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "elastic-reuse",
		Mode:         models.OsqueryModeExisting,
		AllowedOwner: currentUsername(t),
		BinaryPaths:  []string{pattern},
	}
	app := osquery.New(res)
	app.RuntimeStatePath = stateFile

	if ar := app.ApplyResult(context.Background()); ar.Status != executor.Changed || ar.Err != nil {
		t.Fatalf("existing ApplyResult = %+v, want changed", ar)
	}
	if chk := app.Check(context.Background()); chk.Status != executor.Compliant {
		t.Fatalf("existing second Check = %+v, want compliant", chk)
	}
	rt, err := osquery.LoadRuntimeState(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if !rt.Enabled || rt.BinaryPath != elasticBin || !rt.UseDaemonShell {
		t.Fatalf("existing runtime state = %+v, want enabled=true path=%s useDaemonShell=true", rt, elasticBin)
	}

	// Disabled mode transitions enabled runtime -> disabled cleanly.
	disabledRes := models.OsqueryResource{
		ResourceMeta: models.ResourceMeta{Lifecycle: models.LifecyclePresent},
		Name:         "disabled",
		Mode:         models.OsqueryModeDisabled,
	}
	disabledApp := osquery.New(disabledRes)
	disabledApp.RuntimeStatePath = stateFile
	if ar := disabledApp.ApplyResult(context.Background()); ar.Status != executor.Changed || ar.Err != nil {
		t.Fatalf("disabled ApplyResult() = %+v, want changed", ar)
	}
	rt, err = osquery.LoadRuntimeState(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if rt.Enabled || rt.BinaryPath != "" || rt.Mode != models.OsqueryModeDisabled {
		t.Fatalf("disabled runtime = %+v, want disabled/empty", rt)
	}

	// Re-enable existing, then verify absent does NOT delete the external Elastic Agent binary.
	if ar := app.ApplyResult(context.Background()); ar.Status != executor.Changed || ar.Err != nil {
		t.Fatalf("re-enable existing ApplyResult = %+v, want changed", ar)
	}
	absentRes := res
	absentRes.Lifecycle = models.LifecycleAbsent
	absentApp := osquery.New(absentRes)
	absentApp.RuntimeStatePath = stateFile
	if err := absentApp.Apply(context.Background()); err != nil {
		t.Fatalf("existing absent Apply error = %v", err)
	}
	if got, err := os.ReadFile(elasticBin); err != nil || string(got) != elasticContent {
		t.Fatalf("external Elastic Agent binary was modified/removed: %q, err=%v", string(got), err)
	}

	// Group/world-writable binary MUST be rejected.
	if err := os.Chmod(elasticBin, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := app.Apply(context.Background()); err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("writable binary Apply() error = %v, want writable rejection", err)
	}
}
