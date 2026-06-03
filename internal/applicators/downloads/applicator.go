package downloads

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const backupSuffix = ".remotr.bak"

type Applicator struct {
	Download models.DownloadResource
	Exec     executil.Runner
}

func New(d models.DownloadResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Download: d, Exec: exec}
}

func (a *Applicator) Name() string { return "download:" + a.Download.Name }

func (a *Applicator) Description() string {
	return "download " + a.Download.URL + " -> " + a.Download.Dest
}

func (a *Applicator) dest() (string, error) {
	return validateAbsPath(a.Download.Dest)
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	dest, err := a.dest()
	if err != nil {
		return nil, false
	}
	info, err := os.Stat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		return nil, false
	}
	if info.IsDir() {
		return nil, false
	}
	if len(a.Download.Mode) > 0 {
		want := os.FileMode(a.Download.Mode[0] & 0o777)
		if info.Mode().Perm() != want.Perm() {
			return info.Mode(), false
		}
	}
	if a.Download.Checksum != "" {
		sum, err := fileSHA256(dest)
		if err != nil {
			return nil, false
		}
		want, err := parseChecksum(a.Download.Checksum)
		if err != nil {
			return nil, false
		}
		if sum != want {
			return sum, false
		}
	}
	return dest, true
}

func (a *Applicator) Apply(ctx context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	dest, err := a.dest()
	if err != nil {
		return err
	}
	data, err := a.fetch(ctx)
	if err != nil {
		return err
	}
	if a.Download.Checksum != "" {
		want, err := parseChecksum(a.Download.Checksum)
		if err != nil {
			return err
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			return fmt.Errorf("checksum mismatch for %s", dest)
		}
	}
	bak := dest + backupSuffix
	if _, err := os.Stat(dest); err == nil {
		existing, err := os.ReadFile(dest) // #nosec G304 -- absolute path validated
		if err != nil {
			return err
		}
		if err := os.WriteFile(bak, existing, 0o600); err != nil { // #nosec G703
			return fmt.Errorf("backup %s: %w", dest, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if len(a.Download.Mode) > 0 {
		mode = os.FileMode(a.Download.Mode[0] & 0o777)
	}
	if err := atomicWrite(dest, data, mode); err != nil {
		return err
	}
	return a.notifySystemd()
}

func (a *Applicator) Revert(_ context.Context) error {
	dest, err := a.dest()
	if err != nil {
		return err
	}
	bak := dest + backupSuffix
	data, err := os.ReadFile(bak) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return os.Remove(dest)
		}
		return err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil { // #nosec G306 G703
		return err
	}
	return os.Remove(bak)
}

func (a *Applicator) fetch(ctx context.Context) ([]byte, error) {
	out, _, err := a.Exec.Run("curl", "-fsSL", a.Download.URL)
	if err == nil {
		return out, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Download.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", a.Download.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: HTTP %d", a.Download.URL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (a *Applicator) notifySystemd() error {
	unit := strings.TrimSpace(a.Download.NotifySystemd)
	if unit == "" {
		return nil
	}
	_, _, err := a.Exec.Run("systemctl", "try-restart", unit)
	if err == nil {
		return nil
	}
	if _, _, err := a.Exec.Run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, _, err = a.Exec.Run("systemctl", "restart", unit)
	return err
}

func atomicWrite(dest string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".remotr-download-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- caller validates absolute path
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func parseChecksum(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "sha256:") {
		return "", fmt.Errorf("checksum must be sha256:<hex>")
	}
	hexPart := strings.TrimPrefix(s, "sha256:")
	if len(hexPart) != 64 {
		return "", fmt.Errorf("checksum hex must be 64 characters")
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", fmt.Errorf("invalid checksum hex: %w", err)
	}
	return strings.ToLower(hexPart), nil
}

func validateAbsPath(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return "", fmt.Errorf("dest path is required")
	}
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("dest path must be absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid dest path")
	}
	return clean, nil
}
