// Package osquery manages Remotr's explicit osquery runtime policy (disabled,
// existing binary reuse, or isolated private/global managed installations).
package osquery

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	DefaultRuntimeStatePath = "/var/lib/remotr/osquery-runtime.json"
	managedMarkerName       = ".remotr-osquery-managed"
	maxDownloadBytes        = 512 << 20
	maxExtractedBytes       = 512 << 20
)

// RuntimeState is the local descriptor persisted by the osquery applicator and
// consumed by agent/queryexec and capability advertisement.
type RuntimeState struct {
	Enabled        bool                     `json:"enabled"`
	Mode           models.OsqueryMode       `json:"mode"`
	Visibility     models.OsqueryVisibility `json:"visibility,omitempty"`
	BinaryPath     string                   `json:"binary_path,omitempty"`
	UseDaemonShell bool                     `json:"use_daemon_shell,omitempty"`
	Version        string                   `json:"version,omitempty"`
	InstallDir     string                   `json:"install_dir,omitempty"`
	SHA256         string                   `json:"sha256,omitempty"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

// LoadRuntimeState reads the persisted osquery runtime descriptor. Missing file
// returns a disabled RuntimeState without error.
func LoadRuntimeState(path string) (RuntimeState, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultRuntimeStatePath
	}
	raw, err := os.ReadFile(path) // #nosec G304 -- agent runtime state path.
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeState{Enabled: false, Mode: models.OsqueryModeDisabled}, nil
		}
		return RuntimeState{}, err
	}
	var st RuntimeState
	if err := json.Unmarshal(raw, &st); err != nil {
		return RuntimeState{}, fmt.Errorf("parse osquery runtime state: %w", err)
	}
	return st, nil
}

// Applicator converges a models.OsqueryResource.
type Applicator struct {
	Resource         models.OsqueryResource
	RuntimeStatePath string
	Fetch            func(ctx context.Context, rawURL string) ([]byte, error)
	Now              func() time.Time
}

// New creates an osquery runtime applicator.
func New(resource models.OsqueryResource) *Applicator {
	return &Applicator{
		Resource:         resource,
		RuntimeStatePath: DefaultRuntimeStatePath,
		Now:              time.Now,
	}
}

func (a *Applicator) Name() string        { return "osquery:" + a.Resource.Name }
func (a *Applicator) Description() string { return "osquery runtime " + a.Resource.Name }

func (a *Applicator) effectiveLifecycle() models.Lifecycle {
	if a.Resource.Lifecycle == "" {
		return models.LifecyclePresent
	}
	return a.Resource.Lifecycle
}

func (a *Applicator) statePath() string {
	if strings.TrimSpace(a.RuntimeStatePath) == "" {
		return DefaultRuntimeStatePath
	}
	return a.RuntimeStatePath
}

func (a *Applicator) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	st, err := LoadRuntimeState(a.statePath())
	if err != nil {
		return nil, false
	}
	if a.effectiveLifecycle() == models.LifecycleAbsent {
		if st.Enabled {
			return nil, false
		}
		if a.Resource.Mode == models.OsqueryModeManaged {
			installDir := a.Resource.EffectiveInstallDir()
			if _, statErr := os.Stat(filepath.Join(installDir, managedMarkerName)); statErr == nil {
				return nil, false
			}
		}
		return nil, true
	}

	switch a.Resource.Mode {
	case models.OsqueryModeDisabled:
		return nil, !st.Enabled && st.Mode == models.OsqueryModeDisabled && st.BinaryPath == ""

	case models.OsqueryModeExisting:
		resolved, useShell, err := a.resolveExistingBinary()
		if err != nil {
			return nil, false
		}
		return nil, st.Enabled && st.Mode == models.OsqueryModeExisting &&
			st.BinaryPath == resolved && st.UseDaemonShell == useShell

	case models.OsqueryModeManaged:
		installDir := a.Resource.EffectiveInstallDir()
		binPath := filepath.Join(installDir, "bin", "osqueryi")
		wantDirPerm := os.FileMode(0o700)
		wantBinPerm := os.FileMode(0o700)
		if a.Resource.EffectiveVisibility() == models.OsqueryVisibilityGlobal {
			wantDirPerm = 0o755
			wantBinPerm = 0o755
		}
		dirInfo, err := os.Stat(installDir)
		if err != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm() != wantDirPerm {
			return nil, false
		}
		if _, err := os.Stat(filepath.Join(installDir, managedMarkerName)); err != nil {
			return nil, false
		}
		binInfo, err := os.Stat(binPath)
		if err != nil || !binInfo.Mode().IsRegular() || binInfo.Mode().Perm() != wantBinPerm {
			return nil, false
		}
		if a.Resource.EffectiveVisibility() == models.OsqueryVisibilityGlobal {
			symPath := a.Resource.EffectiveSymlinkPath()
			target, err := os.Readlink(symPath)
			if err != nil || target != binPath {
				return nil, false
			}
		}
		return nil, st.Enabled &&
			st.Mode == models.OsqueryModeManaged &&
			st.Visibility == a.Resource.EffectiveVisibility() &&
			st.BinaryPath == binPath &&
			st.Version == a.Resource.Version &&
			strings.EqualFold(st.SHA256, a.Resource.SHA256)
	}
	return nil, false
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	_, compliant := a.State(ctx)
	if compliant {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: "osquery runtime policy"}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: "osquery runtime policy"}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if _, compliant := a.State(ctx); compliant {
		return appErr.ErrStateAlreadyMet
	}
	if a.effectiveLifecycle() == models.LifecycleAbsent {
		return a.applyAbsent()
	}
	switch a.Resource.Mode {
	case models.OsqueryModeDisabled:
		return a.writeRuntimeState(RuntimeState{
			Enabled:   false,
			Mode:      models.OsqueryModeDisabled,
			UpdatedAt: a.now(),
		})
	case models.OsqueryModeExisting:
		resolved, useShell, err := a.resolveExistingBinary()
		if err != nil {
			return err
		}
		return a.writeRuntimeState(RuntimeState{
			Enabled:        true,
			Mode:           models.OsqueryModeExisting,
			BinaryPath:     resolved,
			UseDaemonShell: useShell,
			UpdatedAt:      a.now(),
		})
	case models.OsqueryModeManaged:
		return a.applyManaged(ctx)
	default:
		return fmt.Errorf("unsupported osquery mode %q", a.Resource.Mode)
	}
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if err == nil {
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
}

func (a *Applicator) Revert(_ context.Context) error {
	return appErr.ErrNoOp
}

func (a *Applicator) applyAbsent() error {
	if a.Resource.Mode == models.OsqueryModeManaged {
		installDir := a.Resource.EffectiveInstallDir()
		if a.Resource.EffectiveVisibility() == models.OsqueryVisibilityGlobal {
			symPath := a.Resource.EffectiveSymlinkPath()
			if target, err := os.Readlink(symPath); err == nil && strings.HasPrefix(target, installDir+string(filepath.Separator)) {
				if rmErr := os.Remove(symPath); rmErr != nil && !os.IsNotExist(rmErr) {
					return rmErr
				}
			}
		}
		markerPath := filepath.Join(installDir, managedMarkerName)
		if _, err := os.Stat(markerPath); err == nil {
			if err := os.RemoveAll(installDir); err != nil {
				return err
			}
		}
	}
	// Note: mode: existing NEVER modifies or removes external binaries.
	return a.writeRuntimeState(RuntimeState{
		Enabled:   false,
		Mode:      models.OsqueryModeDisabled,
		UpdatedAt: a.now(),
	})
}

func (a *Applicator) applyManaged(ctx context.Context) error {
	installDir := a.Resource.EffectiveInstallDir()
	binDir := filepath.Join(installDir, "bin")
	binPath := filepath.Join(binDir, "osqueryi")
	vis := a.Resource.EffectiveVisibility()

	// Anti-clobber check BEFORE fetching or modifying disk for global visibility.
	if vis == models.OsqueryVisibilityGlobal {
		symPath := a.Resource.EffectiveSymlinkPath()
		if info, err := os.Lstat(symPath); err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("refusing to clobber unmanaged file at %q", symPath)
			}
			target, readErr := os.Readlink(symPath)
			if readErr != nil || !strings.HasPrefix(target, installDir+string(filepath.Separator)) {
				return fmt.Errorf("refusing to clobber unmanaged symlink at %q", symPath)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	var downloadFile *os.File
	var cleanupDownload bool
	defer func() {
		if downloadFile != nil {
			name := downloadFile.Name()
			_ = downloadFile.Close()
			if cleanupDownload {
				_ = os.Remove(name)
			}
		}
	}()

	var actualHex string
	if a.Fetch != nil {
		payload, err := a.Fetch(ctx, a.Resource.SourceURL)
		if err != nil {
			return fmt.Errorf("fetch osquery artifact: %w", err)
		}
		actualSum := sha256.Sum256(payload)
		actualHex = hex.EncodeToString(actualSum[:])
		if !strings.EqualFold(actualHex, a.Resource.SHA256) {
			return fmt.Errorf("osquery artifact sha256 mismatch: got %s, want %s", actualHex, a.Resource.SHA256)
		}
		tmpDL, err := os.CreateTemp("", "remotr-osq-dl-*")
		if err != nil {
			return err
		}
		downloadFile, cleanupDownload = tmpDL, true
		if _, err := tmpDL.Write(payload); err != nil {
			return err
		}
	} else {
		tmpDL, sumHex, err := streamDownloadToTemp(ctx, a.Resource.SourceURL)
		if err != nil {
			return fmt.Errorf("fetch osquery artifact: %w", err)
		}
		downloadFile, cleanupDownload = tmpDL, true
		actualHex = sumHex
		if !strings.EqualFold(actualHex, a.Resource.SHA256) {
			return fmt.Errorf("osquery artifact sha256 mismatch: got %s, want %s", actualHex, a.Resource.SHA256)
		}
	}

	dirPerm := os.FileMode(0o700)
	binPerm := os.FileMode(0o700)
	if vis == models.OsqueryVisibilityGlobal {
		dirPerm = 0o755
		binPerm = 0o755
	}

	if err := os.MkdirAll(binDir, dirPerm); err != nil {
		return err
	}
	if err := os.Chmod(installDir, dirPerm); err != nil {
		return err
	}
	if err := os.Chmod(binDir, dirPerm); err != nil {
		return err
	}

	// Stream extraction directly to an atomic temp file inside binDir.
	tmpFile, err := os.CreateTemp(binDir, ".osqueryi-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := downloadFile.Seek(0, io.SeekStart); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := extractOsqueryBinaryTo(downloadFile, tmpFile); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(binPerm); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		return err
	}
	cleanupTmp = false

	marker := fmt.Sprintf("managed-by=remotr\nversion=%s\nsha256=%s\n", a.Resource.Version, strings.ToLower(a.Resource.SHA256))
	if err := os.WriteFile(filepath.Join(installDir, managedMarkerName), []byte(marker), 0o600); err != nil {
		return err
	}

	if vis == models.OsqueryVisibilityGlobal {
		symPath := a.Resource.EffectiveSymlinkPath()
		if err := os.MkdirAll(filepath.Dir(symPath), 0o755); err != nil {
			return err
		}
		_ = os.Remove(symPath)
		if err := os.Symlink(binPath, symPath); err != nil {
			return err
		}
	}

	return a.writeRuntimeState(RuntimeState{
		Enabled:        true,
		Mode:           models.OsqueryModeManaged,
		Visibility:     vis,
		BinaryPath:     binPath,
		UseDaemonShell: false,
		Version:        a.Resource.Version,
		InstallDir:     installDir,
		SHA256:         strings.ToLower(a.Resource.SHA256),
		UpdatedAt:      a.now(),
	})
}

func (a *Applicator) resolveExistingBinary() (string, bool, error) {
	allowedOwner := a.Resource.EffectiveAllowedOwner()
	ownerAcc, err := user.Lookup(allowedOwner)
	if err != nil {
		return "", false, fmt.Errorf("allowedOwner %q lookup failed: %w", allowedOwner, err)
	}
	wantUID, err := strconv.ParseUint(ownerAcc.Uid, 10, 32)
	if err != nil {
		return "", false, fmt.Errorf("parse allowedOwner uid: %w", err)
	}

	var lastErr error
	for _, candidate := range a.Resource.BinaryPaths {
		var matches []string
		if strings.Contains(candidate, "*") {
			globbed, globErr := filepath.Glob(candidate)
			if globErr != nil {
				lastErr = globErr
				continue
			}
			// Reverse lexical sort prefers newer versioned directories (e.g. Elastic Agent upgrades).
			sort.Sort(sort.Reverse(sort.StringSlice(globbed)))
			matches = globbed
		} else {
			matches = []string{candidate}
		}

		for _, m := range matches {
			resolved, err := filepath.EvalSymlinks(m)
			if err != nil {
				lastErr = err
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil {
				lastErr = err
				continue
			}
			if !info.Mode().IsRegular() {
				lastErr = fmt.Errorf("existing osquery candidate %q is not a regular file", resolved)
				continue
			}
			if info.Mode().Perm()&0o111 == 0 {
				lastErr = fmt.Errorf("existing osquery candidate %q is not executable", resolved)
				continue
			}
			if info.Mode().Perm()&0o022 != 0 {
				lastErr = fmt.Errorf("existing osquery candidate %q is group/world writable (mode %04o)", resolved, info.Mode().Perm())
				continue
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uint32(wantUID) {
				lastErr = fmt.Errorf("existing osquery candidate %q is not owned by %q", resolved, allowedOwner)
				continue
			}
			useShell := filepath.Base(resolved) == "osqueryd"
			return resolved, useShell, nil
		}
	}
	if lastErr != nil {
		return "", false, lastErr
	}
	return "", false, errors.New("no valid existing osquery binary found")
}

func (a *Applicator) writeRuntimeState(st RuntimeState) error {
	path := a.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".osquery-runtime-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func extractOsqueryBinaryTo(src io.ReadSeeker, dst io.Writer) error {
	var magic [2]byte
	n, err := io.ReadFull(src, magic[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	if n == 0 {
		return errors.New("osquery artifact payload is empty")
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if n >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("open osquery gzip archive: %w", err)
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("read osquery tar archive: %w", err)
			}
			if hdr.Typeflag != tar.TypeReg {
				continue
			}
			base := filepath.Base(hdr.Name)
			if base == "osqueryi" || base == "osqueryd" {
				written, err := io.Copy(dst, io.LimitReader(tr, maxExtractedBytes+1))
				if err != nil {
					return fmt.Errorf("extract osquery binary: %w", err)
				}
				if written > maxExtractedBytes {
					return fmt.Errorf("extracted osquery binary exceeds %d byte cap", maxExtractedBytes)
				}
				return nil
			}
		}
		return errors.New("osquery archive did not contain an osqueryi or osqueryd binary")
	}
	written, err := io.Copy(dst, io.LimitReader(src, maxExtractedBytes+1))
	if err != nil {
		return err
	}
	if written == 0 {
		return errors.New("osquery artifact payload is empty")
	}
	if written > maxExtractedBytes {
		return fmt.Errorf("osquery binary exceeds %d byte cap", maxExtractedBytes)
	}
	return nil
}

func streamDownloadToTemp(ctx context.Context, rawURL string) (*os.File, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "remotr-osq-dl-*")
	if err != nil {
		return nil, "", err
	}
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		name := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(name)
		return nil, "", err
	}
	if written > maxDownloadBytes {
		name := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(name)
		return nil, "", fmt.Errorf("download exceeds %d byte cap", maxDownloadBytes)
	}
	return tmp, hex.EncodeToString(hasher.Sum(nil)), nil
}
