// Package aptkeys manages scoped APT signing-key keyrings.
package aptkeys

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

	"github.com/DavidHoenisch/remotr/internal/applicators/filetx"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
)

const maxKeyMaterialBytes = 1 << 20

// FetchFunc and FingerprintFunc are external boundaries injected by tests.
// They also make the provider independent from a particular HTTP or OpenPGP
// library while keeping production command arguments explicit.
type FetchFunc func(context.Context, string) ([]byte, error)
type FingerprintFunc func(context.Context, []byte) (string, error)
type DearmorFunc func(context.Context, []byte) ([]byte, error)

// Applicator manages one file below KeyringsDir. The directory is intentionally
// fixed to an APT-specific trust scope rather than the deprecated global ring.
type Applicator struct {
	Key            models.APTSigningKey
	KeyringsDir    string
	Runner         executil.Runner
	GPGHomeDir     string
	Fetch          FetchFunc
	Fingerprint    FingerprintFunc
	Dearmor        DearmorFunc
	httpClient     *http.Client
	previous       []byte
	previousExists bool
	rollbackArmed  bool
	rollback       *filetx.Handle
}

// ConfigureRollback binds the owned keyring to protected agent state.
func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := filetx.New(store, address, artifactDigest, false)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
}

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	path, err := a.keyringPath()
	if err != nil {
		return err
	}
	return a.rollback.Preflight(ctx, path)
}

// New creates an APT signing-key provider.
func New(key models.APTSigningKey, runner executil.Runner) *Applicator {
	if key.Lifecycle == "" {
		key.Lifecycle = models.LifecyclePresent
	}
	if runner == nil {
		runner = executil.SanitizedOSRunner{}
	}
	a := &Applicator{
		Key: key, KeyringsDir: "/etc/apt/keyrings", Runner: runner,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	a.Fetch = a.fetch
	a.Fingerprint = a.fingerprint
	a.Dearmor = a.dearmor
	return a
}

func (a *Applicator) Name() string { return "apt-signing-key:" + a.Key.Name }

func (a *Applicator) Description() string { return "APT signing key " + a.Key.Name }

func (a *Applicator) keyringPath() (string, error) {
	if !filepath.IsAbs(a.KeyringsDir) {
		return "", fmt.Errorf("APT keyrings directory must be absolute")
	}
	if err := a.Key.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(a.KeyringsDir, a.Key.Name+".gpg"), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	path, err := a.keyringPath()
	if err != nil {
		return nil, false
	}
	key, err := os.ReadFile(path) // #nosec G304 -- provider validates the owned keyring name.
	if a.Key.Lifecycle == models.LifecycleAbsent {
		return nil, os.IsNotExist(err)
	}
	if err != nil {
		return nil, false
	}
	fingerprint, err := a.Fingerprint(ctx, key)
	if err != nil {
		return nil, false
	}
	return strings.ToUpper(strings.ReplaceAll(fingerprint, " ", "")), strings.EqualFold(strings.ReplaceAll(fingerprint, " ", ""), a.Key.NormalizedFingerprint())
}

func (a *Applicator) Check(ctx context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("scoped APT signing key " + a.Key.Name)
	path, err := a.keyringPath()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	material, err := os.ReadFile(path) // #nosec G304 -- provider validates the owned keyring name.
	if a.Key.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
		}
		if err != nil {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "managed keyring exists"}
	}
	if os.IsNotExist(err) {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "keyring is absent"}
	}
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	fingerprint, err := a.Fingerprint(ctx, material)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	actual := strings.ToUpper(strings.ReplaceAll(fingerprint, " ", ""))
	if strings.EqualFold(actual, a.Key.NormalizedFingerprint()) {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(actual)}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: executor.RedactedSummary(actual)}
}

func (a *Applicator) Apply(ctx context.Context) error {
	path, err := a.keyringPath()
	if err != nil {
		return err
	}
	current, err := os.ReadFile(path) // #nosec G304 -- provider validates the owned keyring name.
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if a.Key.Lifecycle == models.LifecycleAbsent {
		if !exists {
			return appErr.ErrStateAlreadyMet
		}
		if a.rollback != nil {
			if err := a.rollback.Arm(ctx, path); err != nil {
				return err
			}
		} else {
			a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), current...), true, true
		}
		return os.Remove(path) // #nosec G703 -- provider constructs an owned keyring path.
	}
	material, err := a.Fetch(ctx, a.Key.Source)
	if err != nil {
		return fmt.Errorf("fetch APT signing key %q: %w", a.Key.Name, err)
	}
	fingerprint, err := a.Fingerprint(ctx, material)
	if err != nil {
		return fmt.Errorf("inspect APT signing key %q: %w", a.Key.Name, err)
	}
	if !strings.EqualFold(strings.ReplaceAll(fingerprint, " ", ""), a.Key.NormalizedFingerprint()) {
		return fmt.Errorf("APT signing key %q fingerprint mismatch: got %s", a.Key.Name, strings.ToUpper(strings.ReplaceAll(fingerprint, " ", "")))
	}
	keyring, err := a.Dearmor(ctx, material)
	if err != nil {
		return fmt.Errorf("dearmor APT signing key %q: %w", a.Key.Name, err)
	}
	keyringFingerprint, err := a.Fingerprint(ctx, keyring)
	if err != nil {
		return fmt.Errorf("inspect dearmored APT signing key %q: %w", a.Key.Name, err)
	}
	if !strings.EqualFold(strings.ReplaceAll(keyringFingerprint, " ", ""), a.Key.NormalizedFingerprint()) {
		return fmt.Errorf(
			"APT signing key %q dearmored fingerprint mismatch: got %s",
			a.Key.Name, strings.ToUpper(strings.ReplaceAll(keyringFingerprint, " ", "")),
		)
	}
	if exists && string(current) == string(keyring) {
		return appErr.ErrStateAlreadyMet
	}
	if a.rollback != nil {
		if err := a.rollback.Arm(ctx, path); err != nil {
			return err
		}
	} else {
		a.previous, a.previousExists, a.rollbackArmed = append([]byte(nil), current...), exists, true
	}
	if err := atomicWrite(path, keyring, 0o644); err != nil {
		return fmt.Errorf("install APT signing key %q: %w", a.Key.Name, err)
	}
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	if errors.Is(err, appErr.ErrStateAlreadyMet) {
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	}
	if err != nil {
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
	}
	return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
}

func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback != nil {
		err := a.rollback.Rollback(ctx)
		if errors.Is(err, os.ErrNotExist) {
			return appErr.ErrNoOp
		}
		return err
	}
	if !a.rollbackArmed {
		return appErr.ErrNoOp
	}
	path, err := a.keyringPath()
	if err != nil {
		return err
	}
	if !a.previousExists {
		return os.Remove(path) // #nosec G703 -- provider constructs an owned keyring path.
	}
	return atomicWrite(path, a.previous, 0o644)
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

func (a *Applicator) fingerprint(_ context.Context, material []byte) (string, error) {
	input, ok := a.Runner.(executil.InputRunner)
	if !ok {
		return "", errors.New("APT signing-key runner does not support protected input")
	}
	home, cleanup, err := a.gpgHome()
	if err != nil {
		return "", err
	}
	defer cleanup()
	output, _, err := input.RunInput("gpg", material, "--homedir", home, "--batch", "--with-colons", "--import-options", "show-only", "--dry-run", "--import")
	if err != nil {
		return "", errors.New("GPG signing-key inspection failed")
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
			if len(fields) <= 9 || !validOpenPGPFingerprint(fields[9]) {
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

func validOpenPGPFingerprint(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (a *Applicator) dearmor(_ context.Context, material []byte) ([]byte, error) {
	input, ok := a.Runner.(executil.InputRunner)
	if !ok {
		return nil, errors.New("APT signing-key runner does not support protected input")
	}
	home, cleanup, err := a.gpgHome()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	output, _, err := input.RunInput("gpg", material, "--homedir", home, "--batch", "--dearmor", "--output", "-")
	if err != nil {
		return nil, errors.New("GPG signing-key dearmor failed")
	}
	if len(output) == 0 {
		return nil, errors.New("gpg dearmor produced no keyring data")
	}
	return output, nil
}

func (a *Applicator) gpgHome() (string, func(), error) {
	if a.GPGHomeDir != "" {
		if !filepath.IsAbs(a.GPGHomeDir) || filepath.Clean(a.GPGHomeDir) != a.GPGHomeDir {
			return "", nil, errors.New("APT signing-key GPG home must be a clean absolute path")
		}
		return a.GPGHomeDir, func() {}, nil
	}
	home, err := os.MkdirTemp("", "remotr-apt-key-gpg-")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary APT signing-key GPG home: %w", err)
	}
	return home, func() { _ = os.RemoveAll(home) }, nil
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".remotr-apt-key-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
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
	return os.Rename(temporaryName, path)
}
