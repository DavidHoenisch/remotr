package downloads

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const backupSuffix = ".remotr.bak"

type Applicator struct {
	Download      models.DownloadResource
	Exec          executil.Runner
	ResolveSecret func(context.Context, string) (string, error)
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
	if a.Download.Lifecycle == models.LifecycleAbsent {
		_, err := os.Lstat(dest)
		return nil, os.IsNotExist(err)
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
	uid, gid, err := resolveOwnership(a.Download.Owner, a.Download.Group)
	if err != nil {
		return nil, false
	}
	if uid != nil || gid != nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (uid != nil && int(stat.Uid) != *uid) || (gid != nil && int(stat.Gid) != *gid) {
			return info.Sys(), false
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
	if a.Download.Lifecycle == models.LifecycleAbsent {
		info, err := os.Lstat(dest)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to remove non-regular destination %s", dest)
		}
		return os.Remove(dest)
	}
	data, err := a.fetch(ctx)
	if err != nil {
		return err
	}
	if err := verifySignature(data, a.Download.Signature, a.Download.TrustedSigner); err != nil {
		return fmt.Errorf("signature verification for %s: %w", dest, err)
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
	uid, gid, err := resolveOwnership(a.Download.Owner, a.Download.Group)
	if err != nil {
		return err
	}
	if err := atomicWrite(dest, data, mode, uid, gid); err != nil {
		return err
	}
	return nil
}

// ApplyResult activates downloaded content through the engine's shared queue.
func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if err == appErr.ErrStateAlreadyMet {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
	}
	result := executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	for _, notification := range a.Download.Notifications {
		result.Activation = append(result.Activation, executor.ActivationSignal{Kind: executor.ActivationKind(notification.Type), Target: notification.Target})
	}
	if len(a.Download.ReloadExec) > 0 {
		if signal, ok := legacyReloadSignal(a.Download.ReloadExec); ok {
			result.Activation = append(result.Activation, signal)
		}
	}
	if strings.TrimSpace(a.Download.NotifySystemd) != "" && len(a.Download.ReloadExec) == 0 {
		result.Activation = append(result.Activation, executor.ActivationSignal{Kind: executor.ActivationRestart, Target: strings.TrimSpace(a.Download.NotifySystemd)})
	}
	return result
}

func legacyReloadSignal(argv []string) (executor.ActivationSignal, bool) {
	if len(argv) == 2 && argv[0] == "systemctl" && argv[1] == "daemon-reload" {
		return executor.ActivationSignal{Kind: executor.ActivationDaemonReload}, true
	}
	if len(argv) == 3 && argv[0] == "systemctl" && argv[1] == "reload" {
		return executor.ActivationSignal{Kind: executor.ActivationReload, Target: argv[2]}, true
	}
	if len(argv) == 3 && argv[0] == "systemctl" && argv[1] == "restart" {
		return executor.ActivationSignal{Kind: executor.ActivationRestart, Target: argv[2]}, true
	}
	return executor.ActivationSignal{}, false
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
	if a.Download.AuthenticationRef == "" && a.Download.RedirectPolicy == "" && a.Download.Timeout == "" {
		out, _, err := a.Exec.Run("curl", "-fsSL", a.Download.URL)
		if err == nil {
			return out, nil
		}
	}
	timeout := 30 * time.Second
	if a.Download.Timeout != "" {
		parsed, err := time.ParseDuration(a.Download.Timeout)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("invalid download timeout %q", a.Download.Timeout)
		}
		timeout = parsed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Download.URL, nil)
	if err != nil {
		return nil, err
	}
	if a.Download.AuthenticationRef != "" {
		if a.ResolveSecret == nil {
			return nil, fmt.Errorf("authentication reference %q has no secret resolver", a.Download.AuthenticationRef)
		}
		secret, err := a.ResolveSecret(ctx, a.Download.AuthenticationRef)
		if err != nil {
			return nil, fmt.Errorf("resolve download authentication: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	client := &http.Client{Timeout: timeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		switch a.Download.RedirectPolicy {
		case "", "follow":
			return nil
		case "none":
			return http.ErrUseLastResponse
		case "same-origin":
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				return fmt.Errorf("cross-origin redirect blocked")
			}
			return nil
		default:
			return fmt.Errorf("unknown redirect policy %q", a.Download.RedirectPolicy)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", a.Download.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: HTTP %d", a.Download.URL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func atomicWrite(dest string, data []byte, mode os.FileMode, uid, gid *int) error {
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
	if uid != nil || gid != nil {
		u, g := -1, -1
		if uid != nil {
			u = *uid
		}
		if gid != nil {
			g = *gid
		}
		if err := tmp.Chown(u, g); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return err
	}
	cleanup = false
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func verifySignature(data []byte, signature, signer string) error {
	if signature == "" && signer == "" {
		return nil
	}
	if signature == "" || signer == "" {
		return fmt.Errorf("signature and trustedSigner must be provided together")
	}
	pub, err := base64.StdEncoding.DecodeString(signer)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("trustedSigner must be a base64 Ed25519 public key")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("signature must be base64 Ed25519")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func resolveOwnership(ownerName, groupName string) (*int, *int, error) {
	var uid, gid *int
	if ownerName != "" {
		u, err := user.Lookup(ownerName)
		if err != nil {
			return nil, nil, fmt.Errorf("owner %q: %w", ownerName, err)
		}
		value, err := strconv.Atoi(u.Uid)
		if err != nil {
			return nil, nil, err
		}
		uid = &value
	}
	if groupName != "" {
		g, err := user.LookupGroup(groupName)
		if err != nil {
			return nil, nil, fmt.Errorf("group %q: %w", groupName, err)
		}
		value, err := strconv.Atoi(g.Gid)
		if err != nil {
			return nil, nil, err
		}
		gid = &value
	}
	return uid, gid, nil
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
