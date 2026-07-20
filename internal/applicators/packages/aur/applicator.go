// Package aur manages AUR packages through an isolated yay build boundary and
// an explicit privileged Pacman installation boundary.
package aur

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	serviceactions "github.com/DavidHoenisch/remotr/internal/applicators/services"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/types"
)

type Applicator struct {
	Distro        types.Distro
	Package       models.Package
	Exec          executil.Runner
	WorkspaceRoot string
}

func New(distro types.Distro, pkg models.Package, runner executil.Runner) *Applicator {
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	return &Applicator{Distro: distro, Package: pkg, Exec: runner}
}

func (a *Applicator) Name() string { return "yay:" + a.Package.Name }

func (a *Applicator) Description() string { return "AUR package " + a.Package.Name + " via yay" }

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	session, err := a.openBuildSession(ctx)
	if err != nil {
		return checkFailure(err)
	}
	defer session.cleanup()

	version, installed := a.installedVersion()
	actual := any(installed)
	compliant := installed == a.Package.Present
	if a.Package.Present && a.Package.Version != "" {
		actual = version
		compliant = installed && version == a.Package.Version
	}
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, Actual: actual}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, Actual: actual}
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (a *Applicator) Apply(ctx context.Context) error {
	_, err := a.apply(ctx)
	return err
}

func (a *Applicator) apply(ctx context.Context) ([]executor.RedactedSummary, error) {
	session, err := a.openBuildSession(ctx)
	if err != nil {
		return nil, err
	}
	defer session.cleanup()

	installedVersion, installed := a.installedVersion()
	if a.Package.Present && installed && (a.Package.Version == "" || installedVersion == a.Package.Version) {
		return nil, appErr.ErrStateAlreadyMet
	}
	if !a.Package.Present {
		if !installed {
			return nil, appErr.ErrStateAlreadyMet
		}
		_, stderr, err := a.Exec.Run("pacman", "-R", "--noconfirm", a.Package.Name)
		if err != nil {
			return nil, fmt.Errorf("remove AUR package %q through pacman: %s: %w", a.Package.Name, bounded(stderr), err)
		}
		return nil, nil
	}
	if installed && a.Package.Version != "" && installedVersion != a.Package.Version {
		out, stderr, err := a.Exec.Run("vercmp", installedVersion, a.Package.Version)
		if err != nil {
			return nil, fmt.Errorf("compare AUR package versions: %s: %w", session.diagnostic(stderr), err)
		}
		comparison, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			return nil, fmt.Errorf("parse AUR package version comparison: %w", err)
		}
		if comparison > 0 && (a.Package.AllowDowngrade == nil || !*a.Package.AllowDowngrade) {
			return nil, fmt.Errorf("AUR package %q downgrade from %s to %s is not permitted", a.Package.Name, installedVersion, a.Package.Version)
		}
		if comparison < 0 && a.Package.AllowUpgrade != nil && !*a.Package.AllowUpgrade {
			return nil, fmt.Errorf("AUR package %q upgrade from %s to %s is not permitted", a.Package.Name, installedVersion, a.Package.Version)
		}
	}

	if _, stderr, err := session.run(ctx, "yay", "-G", "--aur", "--noconfirm", a.Package.Name); err != nil {
		return nil, fmt.Errorf("resolve AUR package %q: %s: %w", a.Package.Name, session.diagnostic(stderr), err)
	}
	packageDir := filepath.Join(session.buildDir, a.Package.Name)
	if err := requireDirectory(packageDir); err != nil {
		return nil, fmt.Errorf("resolve AUR package %q: %w", a.Package.Name, err)
	}
	identity, err := readSourceIdentity(filepath.Join(packageDir, ".SRCINFO"), a.Package.Name)
	if err != nil {
		return nil, fmt.Errorf("resolve AUR package %q identity: %w", a.Package.Name, err)
	}
	diagnostics := []executor.RedactedSummary{
		executor.RedactedSummary(fmt.Sprintf("AUR source %s version %s", identity.Name, identity.Version)),
	}
	if a.Package.Version != "" && identity.Version != a.Package.Version {
		return diagnostics, fmt.Errorf("AUR package %q version %s is unavailable (source offers %s)", a.Package.Name, a.Package.Version, identity.Version)
	}
	if _, stderr, err := session.runIn(ctx, packageDir, "makepkg", "--nodeps", "--noconfirm", "--cleanbuild"); err != nil {
		return diagnostics, fmt.Errorf("build AUR package %q version %s: %s: %w", identity.Name, identity.Version, session.diagnostic(stderr), err)
	}
	artifact, digest, err := session.stageArtifact(identity)
	if err != nil {
		return diagnostics, fmt.Errorf("stage AUR package %q version %s: %w", identity.Name, identity.Version, err)
	}
	artifactIdentity, err := a.inspectArtifact(artifact)
	if err != nil {
		return diagnostics, err
	}
	diagnostics = append(diagnostics,
		executor.RedactedSummary(fmt.Sprintf("AUR artifact %s version %s sha256:%x", artifactIdentity.Name, artifactIdentity.Version, digest)),
	)
	if artifactIdentity != identity {
		return diagnostics, fmt.Errorf(
			"AUR package %q version %s artifact identifies %s version %s; refusing install",
			identity.Name, identity.Version, artifactIdentity.Name, artifactIdentity.Version,
		)
	}
	if err := ctx.Err(); err != nil {
		return diagnostics, err
	}
	if _, stderr, err := a.Exec.Run("pacman", "-U", "--noconfirm", artifact); err != nil {
		postVersion, present := a.installedVersion()
		if present {
			diagnostics = append(diagnostics, executor.RedactedSummary("AUR post-failure native state version "+postVersion))
		} else {
			diagnostics = append(diagnostics, "AUR post-failure native state absent")
		}
		return diagnostics, fmt.Errorf("install AUR package %q version %s through pacman: %s: %w", identity.Name, identity.Version, session.diagnostic(stderr), err)
	}
	return diagnostics, nil
}

func (a *Applicator) inspectArtifact(path string) (sourceIdentity, error) {
	out, stderr, err := a.Exec.Run("pacman", "-Qp", path)
	if err != nil {
		return sourceIdentity{}, fmt.Errorf("inspect built AUR package artifact: %s: %w", bounded(stderr), err)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return sourceIdentity{}, errors.New("inspect built AUR package artifact: native package identity is malformed")
	}
	return sourceIdentity{Name: fields[0], Version: fields[1]}, nil
}

func (a *Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	diagnostics, err := a.apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{
			Status: executor.Failed, RebootRequired: executor.RebootNotRequired,
			RollbackClass: executor.RollbackNone, Diagnostics: diagnostics, Err: err,
		}
	}
	result := executor.ApplyResult{
		Status: executor.Changed, RebootRequired: executor.RebootNotRequired,
		RollbackClass: executor.RollbackNone, Diagnostics: diagnostics,
	}
	result.Activation = append(result.Activation, serviceactions.ActivationSignals(a.Package.Notifications)...)
	return result
}

type buildIdentity struct {
	uid uint32
	gid uint32
}

type buildSession struct {
	identity  buildIdentity
	runner    executil.UserRunner
	workspace string
	buildDir  string
	stageDir  string
}

func (a *Applicator) openBuildSession(ctx context.Context) (*buildSession, error) {
	if a.Distro != types.Arch {
		return nil, prerequisiteError("AUR packages require the qualifying Arch platform")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(a.Package.AURBuildUser) == "" {
		return nil, prerequisiteError("AUR build user is not declared")
	}
	userRunner, ok := a.Exec.(executil.UserRunner)
	if !ok {
		return nil, prerequisiteError("provider runner cannot enforce the AUR build identity")
	}
	identity, err := a.resolveBuildIdentity()
	if err != nil {
		return nil, err
	}
	session, err := newBuildSession(a.WorkspaceRoot, identity, userRunner)
	if err != nil {
		return nil, fmt.Errorf("create bounded AUR workspace: %w", err)
	}
	if _, stderr, err := session.run(ctx, "yay", "--version"); err != nil {
		session.cleanup()
		return nil, prerequisiteError("yay executable is unavailable: " + session.diagnostic(stderr))
	}
	if _, stderr, err := session.run(ctx, "makepkg", "--version"); err != nil {
		session.cleanup()
		return nil, prerequisiteError("makepkg executable is unavailable: " + session.diagnostic(stderr))
	}
	return session, nil
}

func (a *Applicator) resolveBuildIdentity() (buildIdentity, error) {
	uid, err := a.readIdentityPart("-u")
	if err != nil {
		return buildIdentity{}, err
	}
	if uid == 0 {
		return buildIdentity{}, prerequisiteError("declared AUR build user is privileged")
	}
	gid, err := a.readIdentityPart("-g")
	if err != nil {
		return buildIdentity{}, err
	}
	return buildIdentity{uid: uid, gid: gid}, nil
}

func (a *Applicator) readIdentityPart(flag string) (uint32, error) {
	out, _, err := a.Exec.Run("id", flag, "--", a.Package.AURBuildUser)
	if err != nil {
		return 0, prerequisiteError("declared AUR build user is unavailable")
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse AUR build user identity: %w", err)
	}
	return uint32(value), nil
}

func newBuildSession(root string, identity buildIdentity, runner executil.UserRunner) (*buildSession, error) {
	workspace, err := os.MkdirTemp(root, "remotr-aur-")
	if err != nil {
		return nil, err
	}
	session := &buildSession{
		identity: identity, runner: runner, workspace: workspace,
		buildDir: filepath.Join(workspace, "build"), stageDir: filepath.Join(workspace, "artifacts"),
	}
	failed := true
	defer func() {
		if failed {
			session.cleanup()
		}
	}()
	if err := os.Chmod(workspace, 0o711); err != nil {
		return nil, err
	}
	if err := os.Mkdir(session.buildDir, 0o700); err != nil {
		return nil, err
	}
	if int(identity.uid) != os.Getuid() || int(identity.gid) != os.Getgid() {
		if err := os.Chown(session.buildDir, int(identity.uid), int(identity.gid)); err != nil {
			return nil, err
		}
	}
	if err := os.Mkdir(session.stageDir, 0o700); err != nil {
		return nil, err
	}
	failed = false
	return session, nil
}

func (s *buildSession) cleanup() { _ = os.RemoveAll(s.workspace) }

func (s *buildSession) diagnostic(value []byte) string {
	text := strings.ReplaceAll(string(value), s.workspace, "[workspace]")
	text = strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return r <= ' ' || r == '\u007f'
	}), " ")
	const max = 1024
	if len(text) > max {
		text = text[:max]
	}
	return text
}

func (s *buildSession) run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	return s.runIn(ctx, s.buildDir, name, args...)
}

func (s *buildSession) runIn(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
	return s.runner.RunAsUser(ctx, executil.UserProcess{
		Name: name, Args: args, Dir: dir, Home: s.buildDir, UID: s.identity.uid, GID: s.identity.gid,
	})
}

type sourceIdentity struct {
	Name    string
	Version string
}

func readSourceIdentity(path, target string) (sourceIdentity, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return sourceIdentity{}, err
	}
	var pkgver, pkgrel, epoch string
	foundTarget := false
	for _, line := range strings.Split(string(content), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "pkgname":
			foundTarget = foundTarget || value == target
		case "pkgver":
			pkgver = value
		case "pkgrel":
			pkgrel = value
		case "epoch":
			epoch = value
		}
	}
	if !foundTarget || pkgver == "" || pkgrel == "" {
		return sourceIdentity{}, errors.New("source metadata does not identify the requested package and version")
	}
	version := pkgver + "-" + pkgrel
	if epoch != "" && epoch != "0" {
		version = epoch + ":" + version
	}
	return sourceIdentity{Name: target, Version: version}, nil
}

func (s *buildSession) stageArtifact(identity sourceIdentity) (string, [sha256.Size]byte, error) {
	pattern := filepath.Join(s.buildDir, identity.Name, identity.Name+"-"+identity.Version+"-*.pkg.tar.*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", [sha256.Size]byte{}, err
	}
	artifacts := matches[:0]
	for _, match := range matches {
		if strings.HasSuffix(match, ".sig") {
			continue
		}
		info, err := os.Lstat(match)
		if err != nil {
			return "", [sha256.Size]byte{}, err
		}
		if info.Mode().IsRegular() {
			artifacts = append(artifacts, match)
		}
	}
	if len(artifacts) != 1 {
		return "", [sha256.Size]byte{}, fmt.Errorf("build produced %d matching package artifacts", len(artifacts))
	}
	source, err := os.Open(artifacts[0])
	if err != nil {
		return "", [sha256.Size]byte{}, err
	}
	defer source.Close()
	destinationPath := filepath.Join(s.stageDir, filepath.Base(artifacts[0]))
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", [sha256.Size]byte{}, err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(destination, hash), source)
	closeErr := destination.Close()
	if copyErr != nil {
		return "", [sha256.Size]byte{}, copyErr
	}
	if closeErr != nil {
		return "", [sha256.Size]byte{}, closeErr
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return destinationPath, digest, nil
}

func (a *Applicator) installedVersion() (string, bool) {
	out, _, err := a.Exec.Run("pacman", "-Q", a.Package.Name)
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", true
	}
	return fields[len(fields)-1], true
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("resolved package path is not a directory")
	}
	return nil
}

type prerequisiteError string

func (e prerequisiteError) Error() string { return string(e) }

func checkFailure(err error) executor.CheckResult {
	var prerequisite prerequisiteError
	if errors.As(err, &prerequisite) {
		return unsupported(executor.RedactedSummary(prerequisite))
	}
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, Err: err}
}

func unsupported(summary executor.RedactedSummary) executor.CheckResult {
	return executor.CheckResult{
		Status: executor.Unsupported, ReasonCode: executor.ReasonProviderUnavailable,
		ObservedSummary: summary,
	}
}

func bounded(value []byte) string {
	const max = 1024
	value = []byte(strings.TrimSpace(string(value)))
	if len(value) > max {
		value = value[:max]
	}
	return string(value)
}
