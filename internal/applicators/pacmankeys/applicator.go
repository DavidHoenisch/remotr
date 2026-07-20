// Package pacmankeys manages narrowly owned trust in Pacman's native keyring.
package pacmankeys

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const maxKeyMaterialBytes = 1 << 20

type FetchFunc func(context.Context, string) ([]byte, error)

type Applicator struct {
	Key        models.PacmanSigningKey
	StateDir   string
	Runner     executil.Runner
	GPGHomeDir string
	Fetch      FetchFunc
	httpClient *http.Client
}

func New(key models.PacmanSigningKey, runner executil.Runner) *Applicator {
	if key.Lifecycle == "" {
		key.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	provider := &Applicator{
		Key: key, StateDir: "/var/lib/remotr/pacman-keys", Runner: runner,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	provider.Fetch = provider.fetch
	return provider
}

func (a *Applicator) Name() string { return "pacman-signing-key:" + a.Key.Name }

func (a *Applicator) Description() string { return "Pacman signing key " + a.Key.Name }

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("owned Pacman signing key " + a.Key.Name)
	marker, err := a.markerPath()
	if err != nil {
		return checkFailure(desired, err)
	}
	if err := ctx.Err(); err != nil {
		return checkFailure(desired, err)
	}
	ownedFingerprint, err := os.ReadFile(marker) // #nosec G304 -- validated resource name below a fixed state root.
	if a.Key.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
		}
		if err != nil {
			return checkFailure(desired, err)
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "owned trust marker exists"}
	}
	if os.IsNotExist(err) {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "owned trust marker is absent"}
	}
	if err != nil {
		return checkFailure(desired, err)
	}
	if strings.TrimSpace(string(ownedFingerprint)) != a.Key.NormalizedFingerprint() {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "owned trust marker differs"}
	}
	present, err := a.nativePresent(ctx, a.Key.NormalizedFingerprint())
	if err != nil {
		return checkFailure(desired, err)
	}
	if !present {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "owned key is absent from Pacman trust"}
	}
	return executor.CheckResult{
		Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired,
		ObservedSummary: executor.RedactedSummary(a.Key.NormalizedFingerprint()),
	}
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.Actual, check.Status == executor.Compliant
}

func (a *Applicator) Apply(ctx context.Context) error {
	if err := a.Key.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	check := a.Check(ctx)
	if check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	if check.Status == executor.CheckFailed {
		return check.Err
	}
	marker, err := a.markerPath()
	if err != nil {
		return err
	}
	if a.Key.Lifecycle == models.LifecycleAbsent {
		ownedFingerprint, err := os.ReadFile(marker) // #nosec G304 -- validated resource name below a fixed state root.
		if err != nil {
			return err
		}
		fingerprint := strings.TrimSpace(string(ownedFingerprint))
		if !validFingerprint(fingerprint) {
			return errors.New("owned Pacman signing-key fingerprint marker is malformed")
		}
		fingerprint = strings.ToUpper(fingerprint)
		present, err := a.nativePresent(ctx, fingerprint)
		if err != nil {
			return err
		}
		if present {
			if _, _, err := a.Runner.Run("pacman-key", "--delete", fingerprint); err != nil {
				return fmt.Errorf("delete owned Pacman signing key %q failed", a.Key.Name)
			}
		}
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	material, err := a.Fetch(ctx, a.Key.Source)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("fetch Pacman signing key %q failed", a.Key.Name)
	}
	fingerprint, err := a.fingerprint(material)
	if err != nil {
		return fmt.Errorf("inspect Pacman signing key %q: %w", a.Key.Name, err)
	}
	if fingerprint != a.Key.NormalizedFingerprint() {
		return fmt.Errorf("Pacman signing key %q fingerprint mismatch: got %s", a.Key.Name, fingerprint)
	}
	present, err := a.nativePresent(ctx, a.Key.NormalizedFingerprint())
	if err != nil {
		return err
	}
	newlyAdded := false
	if !present {
		materialPath, cleanup, err := a.stageKeyMaterial(material)
		if err != nil {
			return fmt.Errorf("stage Pacman signing key %q: %w", a.Key.Name, err)
		}
		defer cleanup()
		if _, _, err := a.Runner.Run("pacman-key", "--add", materialPath); err != nil {
			return fmt.Errorf("import Pacman signing key %q failed", a.Key.Name)
		}
		newlyAdded = true
	}
	if _, _, err := a.Runner.Run("pacman-key", "--lsign-key", a.Key.NormalizedFingerprint()); err != nil {
		if newlyAdded {
			_, _, _ = a.Runner.Run("pacman-key", "--delete", a.Key.NormalizedFingerprint())
		}
		return fmt.Errorf("locally trust Pacman signing key %q failed", a.Key.Name)
	}
	if err := atomicWrite(marker, []byte(a.Key.NormalizedFingerprint()+"\n"), 0o644); err != nil {
		if newlyAdded {
			_, _, _ = a.Runner.Run("pacman-key", "--delete", a.Key.NormalizedFingerprint())
		}
		return fmt.Errorf("record Pacman signing-key ownership: %w", err)
	}
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackNone}
}

func (*Applicator) Revert(context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) markerPath() (string, error) {
	if !filepath.IsAbs(a.StateDir) {
		return "", errors.New("Pacman signing-key state directory must be absolute")
	}
	if err := a.Key.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(a.StateDir, a.Key.Name+".fingerprint"), nil
}

func (a *Applicator) nativePresent(ctx context.Context, fingerprint string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	stdout, stderr, err := a.Runner.Run("pacman-key", "--nocolor", "--finger", fingerprint)
	if err != nil {
		diagnostic := strings.ToLower(string(append(append([]byte(nil), stdout...), stderr...)))
		if strings.Contains(diagnostic, "unknown key") || strings.Contains(diagnostic, "key not found") || strings.Contains(diagnostic, "no public key") {
			return false, nil
		}
		return false, errors.New("Pacman native trust probe failed")
	}
	normalized := strings.ToUpper(strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'F') || (r >= 'a' && r <= 'f') {
			return r
		}
		return -1
	}, string(stdout)))
	return strings.Contains(normalized, fingerprint), nil
}

func (a *Applicator) stageKeyMaterial(material []byte) (string, func(), error) {
	if err := os.MkdirAll(a.StateDir, 0o700); err != nil {
		return "", func() {}, err
	}
	temporary, err := os.CreateTemp(a.StateDir, ".pacman-key-")
	if err != nil {
		return "", func() {}, err
	}
	path := temporary.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		cleanup()
		return "", func() {}, err
	}
	if _, err := temporary.Write(material); err != nil {
		temporary.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".pacman-key-state-")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func checkFailure(desired executor.RedactedSummary, err error) executor.CheckResult {
	return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
}

func (a *Applicator) fingerprint(material []byte) (string, error) {
	input, ok := a.Runner.(executil.InputRunner)
	if !ok {
		return "", errors.New("Pacman signing-key runner does not support protected input")
	}
	home, cleanup, err := a.gpgHome()
	if err != nil {
		return "", err
	}
	defer cleanup()
	output, _, err := input.RunInput("gpg", material, "--homedir", home, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import")
	if err != nil {
		return "", errors.New("gpg inspection failed")
	}
	var fingerprints []string
	wantPrimaryFingerprint := false
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "pub":
			wantPrimaryFingerprint = true
		case "sub":
			wantPrimaryFingerprint = false
		case "fpr":
			if !wantPrimaryFingerprint {
				continue
			}
			if len(fields) <= 9 || !validFingerprint(fields[9]) {
				return "", errors.New("primary OpenPGP fingerprint is malformed")
			}
			fingerprints = append(fingerprints, strings.ToUpper(fields[9]))
			wantPrimaryFingerprint = false
		}
	}
	if len(fingerprints) != 1 {
		return "", fmt.Errorf("expected exactly one primary fingerprint, got %d", len(fingerprints))
	}
	return fingerprints[0], nil
}

func (a *Applicator) gpgHome() (string, func(), error) {
	if a.GPGHomeDir != "" {
		if !filepath.IsAbs(a.GPGHomeDir) || filepath.Clean(a.GPGHomeDir) != a.GPGHomeDir {
			return "", nil, errors.New("Pacman signing-key GPG home must be a clean absolute path")
		}
		return a.GPGHomeDir, func() {}, nil
	}
	home, err := os.MkdirTemp("", "remotr-pacman-key-gpg-")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary Pacman signing-key GPG home: %w", err)
	}
	return home, func() { _ = os.RemoveAll(home) }, nil
}

func (a *Applicator) fetch(ctx context.Context, source string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	material, err := io.ReadAll(io.LimitReader(response.Body, maxKeyMaterialBytes+1))
	if err != nil {
		return nil, err
	}
	if len(material) > maxKeyMaterialBytes {
		return nil, fmt.Errorf("key material exceeds %d byte limit", maxKeyMaterialBytes)
	}
	return material, nil
}

func validFingerprint(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
