package aur_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/applicators/packages/aur"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-PRM-024: AUR resolution and build execution use the declared
// unprivileged identity in one bounded workspace; only the resulting package
// artifact crosses the privileged Pacman install boundary.
func TestApplicator_AURBuildUsesDeclaredUserAndBoundedWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.Chmod(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	uid, gid := testUnprivilegedIdentity()
	runner := &aurProcessRunner{uid: uid, gid: gid}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "1.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Changed || result.Err != nil {
		t.Fatalf("ApplyResult() = %+v, want changed", result)
	}
	if len(runner.userCalls) != 4 {
		t.Fatalf("unprivileged process calls = %+v, want four", runner.userCalls)
	}
	workspace := filepath.Dir(runner.userCalls[0].Home)
	if filepath.Dir(workspace) != workspaceRoot || !strings.HasPrefix(filepath.Base(workspace), "remotr-aur-") {
		t.Fatalf("AUR workspace = %q, want bounded child of %q", workspace, workspaceRoot)
	}
	buildDir := filepath.Join(workspace, "build")
	packageDir := filepath.Join(buildDir, "remotr-aur-fixture")
	wantUserCalls := []executil.UserProcess{
		{Name: "yay", Args: []string{"--version"}, Dir: buildDir, Home: buildDir, UID: uid, GID: gid},
		{Name: "makepkg", Args: []string{"--version"}, Dir: buildDir, Home: buildDir, UID: uid, GID: gid},
		{Name: "yay", Args: []string{"-G", "--aur", "--noconfirm", "remotr-aur-fixture"}, Dir: buildDir, Home: buildDir, UID: uid, GID: gid},
		{Name: "makepkg", Args: []string{"--nodeps", "--noconfirm", "--cleanbuild"}, Dir: packageDir, Home: buildDir, UID: uid, GID: gid},
	}
	if !slices.EqualFunc(runner.userCalls, wantUserCalls, equalUserProcess) {
		t.Fatalf("unprivileged process calls = %+v, want %+v", runner.userCalls, wantUserCalls)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("privileged process calls = %+v, want identity probes, state probe, artifact inspection, and install", runner.calls)
	}
	wantArtifact := filepath.Join(workspace, "artifacts", "remotr-aur-fixture-1.0.0-1-x86_64.pkg.tar.zst")
	wantCalls := []executil.MockCall{
		{Name: "id", Args: []string{"-u", "--", "aur-builder"}},
		{Name: "id", Args: []string{"-g", "--", "aur-builder"}},
		{Name: "pacman", Args: []string{"-Q", "remotr-aur-fixture"}},
		{Name: "pacman", Args: []string{"-Qp", wantArtifact}},
		{Name: "pacman", Args: []string{"-U", "--noconfirm", wantArtifact}},
	}
	if !slices.EqualFunc(runner.calls, wantCalls, equalAURCall) {
		t.Fatalf("privileged process calls = %+v, want %+v", runner.calls, wantCalls)
	}
	for _, call := range runner.calls {
		if call.Name == "sh" || call.Name == "bash" {
			t.Fatalf("AUR provider invoked a privileged shell: %+v", call)
		}
	}
	for _, process := range runner.userCalls {
		if process.Name == "sh" || process.Name == "bash" {
			t.Fatalf("AUR provider invoked an unprivileged shell: %+v", process)
		}
	}
	artifactDigest := sha256.Sum256(aurFixtureArtifact)
	wantDiagnostics := []executor.RedactedSummary{
		"AUR source remotr-aur-fixture version 1.0.0-1",
		executor.RedactedSummary(fmt.Sprintf("AUR artifact remotr-aur-fixture version 1.0.0-1 sha256:%x", artifactDigest)),
	}
	if !slices.Equal(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("ApplyResult diagnostics = %q, want %q", result.Diagnostics, wantDiagnostics)
	}
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AUR workspace remains after Apply: %v", err)
	}
}

func TestApplicator_defaultsToSanitizedProcessRunner(t *testing.T) {
	provider := aur.New(types.Arch, models.Package{Name: "fixture", Present: true, AURBuildUser: "builder"}, nil)
	if _, ok := provider.Exec.(executil.SanitizedOSRunner); !ok {
		t.Fatalf("default runner = %T, want SanitizedOSRunner", provider.Exec)
	}
}

// OS-PRM-025: a build artifact whose native package identity differs from the
// resolved source is rejected before installation even when its filename looks
// correct.
func TestApplicator_AURArtifactIdentityMismatchFailsBeforeInstall(t *testing.T) {
	workspaceRoot := t.TempDir()
	uid, gid := testUnprivilegedIdentity()
	runner := &aurProcessRunner{uid: uid, gid: gid, artifactIdentity: "different-package 1.0.0-1\n"}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "1.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "artifact identifies different-package version 1.0.0-1") {
		t.Fatalf("ApplyResult() = %+v, want sanitized artifact-identity failure", result)
	}
	for _, call := range runner.calls {
		if call.Name == "pacman" && len(call.Args) > 0 && call.Args[0] == "-U" {
			t.Fatalf("mismatched artifact reached install: %+v", call)
		}
	}
	digest := sha256.Sum256(aurFixtureArtifact)
	wantDiagnostics := []executor.RedactedSummary{
		"AUR source remotr-aur-fixture version 1.0.0-1",
		executor.RedactedSummary(fmt.Sprintf("AUR artifact different-package version 1.0.0-1 sha256:%x", digest)),
	}
	if !slices.Equal(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("ApplyResult diagnostics = %q, want %q", result.Diagnostics, wantDiagnostics)
	}
	workspace := filepath.Dir(runner.userCalls[0].Home)
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AUR workspace remains after artifact mismatch: %v", err)
	}
}

// OS-PRM-025: failed third-party build output is bounded and has its transient
// path removed before it becomes a provider diagnostic; no artifact is handed
// to Pacman and the workspace is still cleaned.
func TestApplicator_AURBuildFailureIsSanitizedAndCleaned(t *testing.T) {
	workspaceRoot := t.TempDir()
	uid, gid := testUnprivilegedIdentity()
	runner := &aurProcessRunner{uid: uid, gid: gid, buildFailure: true}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "1.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("ApplyResult() = %+v, want failed build", result)
	}
	if got := result.Err.Error(); !strings.Contains(got, "build failed in [workspace]") || strings.Contains(got, workspaceRoot) || strings.ContainsAny(got, "\r\n\x00") || len(got) > 1400 {
		t.Fatalf("build failure = %q, want bounded single-line diagnostic with redacted workspace", got)
	}
	if !slices.Equal(result.Diagnostics, []executor.RedactedSummary{"AUR source remotr-aur-fixture version 1.0.0-1"}) {
		t.Fatalf("build failure diagnostics = %q, want resolved source identity", result.Diagnostics)
	}
	for _, call := range runner.calls {
		if call.Name == "pacman" && len(call.Args) > 0 && call.Args[0] != "-Q" {
			t.Fatalf("failed build reached native mutation: %+v", call)
		}
	}
	workspace := filepath.Dir(runner.userCalls[0].Home)
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AUR workspace remains after failed build: %v", err)
	}
}

// OS-PRM-025: cancellation after a successful build but before the privileged
// boundary prevents Pacman mutation and still cleans the transient workspace.
func TestApplicator_AURCancellationBeforeInstallPreventsMutation(t *testing.T) {
	workspaceRoot := t.TempDir()
	uid, gid := testUnprivilegedIdentity()
	ctx, cancel := context.WithCancel(t.Context())
	runner := &aurProcessRunner{uid: uid, gid: gid, cancelAfterInspect: cancel}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "1.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(ctx)
	if result.Status != executor.Failed || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("ApplyResult() = %+v, want canceled failure", result)
	}
	for _, call := range runner.calls {
		if call.Name == "pacman" && len(call.Args) > 0 && call.Args[0] == "-U" {
			t.Fatalf("canceled AUR operation reached install: %+v", call)
		}
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("canceled AUR diagnostics = %q, want resolved source and artifact evidence", result.Diagnostics)
	}
	workspace := filepath.Dir(runner.userCalls[0].Home)
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AUR workspace remains after cancellation: %v", err)
	}
}

// OS-PRM-025: a failed Pacman transaction is followed by a native state probe
// so the provider records that no alternate package version entered the
// database before returning the failure.
func TestApplicator_AURInstallFailureRecordsConsistentNativeState(t *testing.T) {
	workspaceRoot := t.TempDir()
	uid, gid := testUnprivilegedIdentity()
	runner := &aurProcessRunner{uid: uid, gid: gid, installFailure: true}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "1.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "install AUR package") {
		t.Fatalf("ApplyResult() = %+v, want failed native install", result)
	}
	if len(runner.calls) < 2 || runner.calls[len(runner.calls)-2].Args[0] != "-U" || !slices.Equal(runner.calls[len(runner.calls)-1].Args, []string{"-Q", "remotr-aur-fixture"}) {
		t.Fatalf("install failure process tail = %+v, want pacman -U followed by pacman -Q", runner.calls)
	}
	if len(result.Diagnostics) != 3 || result.Diagnostics[2] != "AUR post-failure native state absent" {
		t.Fatalf("install failure diagnostics = %q, want source, artifact, and absent native state", result.Diagnostics)
	}
	workspace := filepath.Dir(runner.userCalls[0].Home)
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AUR workspace remains after install failure: %v", err)
	}
}

func TestApplicator_AURExactDowngradeIsBlockedBeforeBuild(t *testing.T) {
	workspaceRoot := t.TempDir()
	uid, gid := testUnprivilegedIdentity()
	runner := &aurProcessRunner{uid: uid, gid: gid, installedVersion: "2.0.0-1"}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "1.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil || !strings.Contains(result.Err.Error(), "downgrade from 2.0.0-1 to 1.0.0-1 is not permitted") {
		t.Fatalf("ApplyResult() = %+v, want blocked exact downgrade", result)
	}
	for _, call := range runner.userCalls {
		if call.Name == "yay" && !slices.Equal(call.Args, []string{"--version"}) {
			t.Fatalf("blocked downgrade reached source resolution: %+v", call)
		}
		if call.Name == "makepkg" && !slices.Equal(call.Args, []string{"--version"}) {
			t.Fatalf("blocked downgrade reached build: %+v", call)
		}
	}
	for _, call := range runner.calls {
		if call.Name == "pacman" && len(call.Args) > 0 && call.Args[0] == "-U" {
			t.Fatalf("blocked downgrade reached install: %+v", call)
		}
	}
}

// OS-PRM-025: a source that cannot provide the declared exact version is
// reported with sanitized source identity and never reaches a build or native
// package mutation.
func TestApplicator_ExactAURVersionUnavailableFailsWithoutInstallingAnotherVersion(t *testing.T) {
	workspaceRoot := t.TempDir()
	uid, gid := testUnprivilegedIdentity()
	runner := &aurProcessRunner{uid: uid, gid: gid, sourceVersion: "2.0.0-1"}
	provider := aur.New(types.Arch, models.Package{
		Name: "remotr-aur-fixture", Present: true, Version: "9.0.0-1", AURBuildUser: "aur-builder",
	}, runner)
	provider.WorkspaceRoot = workspaceRoot

	result := provider.ApplyResult(t.Context())
	if result.Status != executor.Failed || result.Err == nil {
		t.Fatalf("ApplyResult() = %+v, want failed unavailable version", result)
	}
	if got := result.Err.Error(); !strings.Contains(got, "version 9.0.0-1 is unavailable") || !strings.Contains(got, "source offers 2.0.0-1") || strings.Contains(got, workspaceRoot) {
		t.Fatalf("unavailable-version error = %q, want sanitized requested and resolved versions", got)
	}
	wantDiagnostics := []executor.RedactedSummary{"AUR source remotr-aur-fixture version 2.0.0-1"}
	if !slices.Equal(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("ApplyResult diagnostics = %q, want %q", result.Diagnostics, wantDiagnostics)
	}
	for _, call := range runner.calls {
		if call.Name == "pacman" && len(call.Args) > 0 && call.Args[0] != "-Q" {
			t.Fatalf("unavailable version reached native mutation: %+v", call)
		}
	}
	for _, call := range runner.userCalls {
		if call.Name == "makepkg" && !slices.Equal(call.Args, []string{"--version"}) {
			t.Fatalf("unavailable version reached build: %+v", call)
		}
	}
	workspace := filepath.Dir(runner.userCalls[0].Home)
	if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AUR workspace remains after unavailable version: %v", err)
	}
}

var aurFixtureArtifact = []byte("controlled AUR package artifact\n")

type aurProcessRunner struct {
	uid, gid           uint32
	sourceVersion      string
	artifactIdentity   string
	buildFailure       bool
	cancelAfterInspect context.CancelFunc
	installFailure     bool
	installedVersion   string
	calls              []executil.MockCall
	userCalls          []executil.UserProcess
}

func (r *aurProcessRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: slices.Clone(args)})
	switch {
	case name == "id" && slices.Equal(args, []string{"-u", "--", "aur-builder"}):
		return []byte(strconv.FormatUint(uint64(r.uid), 10) + "\n"), nil, nil
	case name == "id" && slices.Equal(args, []string{"-g", "--", "aur-builder"}):
		return []byte(strconv.FormatUint(uint64(r.gid), 10) + "\n"), nil, nil
	case name == "pacman" && slices.Equal(args, []string{"-Q", "remotr-aur-fixture"}):
		if r.installedVersion != "" {
			return []byte("remotr-aur-fixture " + r.installedVersion + "\n"), nil, nil
		}
		return nil, nil, errors.New("package not installed")
	case name == "vercmp" && len(args) == 2:
		switch {
		case args[0] == args[1]:
			return []byte("0\n"), nil, nil
		case args[0] == "2.0.0-1" && args[1] == "1.0.0-1":
			return []byte("1\n"), nil, nil
		default:
			return []byte("-1\n"), nil, nil
		}
	case name == "pacman" && len(args) == 2 && args[0] == "-Qp":
		identity := r.artifactIdentity
		if identity == "" {
			identity = "remotr-aur-fixture 1.0.0-1\n"
		}
		if r.cancelAfterInspect != nil {
			r.cancelAfterInspect()
		}
		return []byte(identity), nil, nil
	case name == "pacman" && len(args) == 3 && args[0] == "-U" && args[1] == "--noconfirm":
		if r.installFailure {
			return nil, []byte("transaction failed for " + args[2] + "\n"), errors.New("exit status 1")
		}
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected privileged process: %s %v", name, args)
	}
}

func (r *aurProcessRunner) RunAsUser(_ context.Context, process executil.UserProcess) ([]byte, []byte, error) {
	process.Args = slices.Clone(process.Args)
	r.userCalls = append(r.userCalls, process)
	switch {
	case process.Name == "yay" && slices.Equal(process.Args, []string{"--version"}):
		return []byte("yay v12\n"), nil, nil
	case process.Name == "makepkg" && slices.Equal(process.Args, []string{"--version"}):
		return []byte("makepkg 7\n"), nil, nil
	case process.Name == "yay" && slices.Equal(process.Args, []string{"-G", "--aur", "--noconfirm", "remotr-aur-fixture"}):
		packageDir := filepath.Join(process.Dir, "remotr-aur-fixture")
		if err := os.Mkdir(packageDir, 0o755); err != nil {
			return nil, nil, err
		}
		version := r.sourceVersion
		if version == "" {
			version = "1.0.0-1"
		}
		pkgver, pkgrel, ok := strings.Cut(version, "-")
		if !ok {
			return nil, nil, fmt.Errorf("invalid test source version %q", version)
		}
		srcinfo := fmt.Sprintf("pkgbase = remotr-aur-fixture\n\tpkgver = %s\n\tpkgrel = %s\n\tpkgname = remotr-aur-fixture\n", pkgver, pkgrel)
		return nil, nil, os.WriteFile(filepath.Join(packageDir, ".SRCINFO"), []byte(srcinfo), 0o644)
	case process.Name == "makepkg" && slices.Equal(process.Args, []string{"--nodeps", "--noconfirm", "--cleanbuild"}):
		if r.buildFailure {
			stderr := "build failed in " + process.Dir + "\n" + strings.Repeat("x", 2048) + "\x00"
			return nil, []byte(stderr), errors.New("exit status 2")
		}
		artifact := filepath.Join(process.Dir, "remotr-aur-fixture-1.0.0-1-x86_64.pkg.tar.zst")
		return nil, nil, os.WriteFile(artifact, aurFixtureArtifact, 0o644)
	default:
		return nil, nil, fmt.Errorf("unexpected unprivileged process: %s %v", process.Name, process.Args)
	}
}

func equalUserProcess(left, right executil.UserProcess) bool {
	return left.Name == right.Name && slices.Equal(left.Args, right.Args) && left.Dir == right.Dir &&
		left.Home == right.Home && left.UID == right.UID && left.GID == right.GID
}

func testUnprivilegedIdentity() (uint32, uint32) {
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 {
		return 1001, 1001
	}
	return uint32(uid), uint32(gid)
}
