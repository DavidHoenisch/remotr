// Package trustanchors manages named system CA trust anchors.
package trustanchors

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	serviceactions "github.com/DavidHoenisch/remotr/internal/applicators/services"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/internal/types"
)

type ResolveFunc func(context.Context, string) ([]byte, error)

type Applicator struct {
	Resource      models.TrustAnchorResource
	AnchorsDir    string
	RefreshTarget string
	Resolve       ResolveFunc
	previous      []byte
	previousMode  os.FileMode
	previousUID   int
	previousGID   int
	previousExist bool
	armed         bool
	rollback      *rollbackstore.Handle
}

type protectedSnapshot struct {
	Version int    `json:"version"`
	Path    string `json:"path"`
	Content []byte `json:"content,omitempty"`
	Exists  bool   `json:"exists"`
	Mode    uint32 `json:"mode,omitempty"`
	UID     int    `json:"uid,omitempty"`
	GID     int    `json:"gid,omitempty"`
}

func New(resource models.TrustAnchorResource, distro types.Distro) (*Applicator, error) {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	applicator := &Applicator{Resource: resource}
	switch distro {
	case types.Debian, types.Ubuntu:
		applicator.AnchorsDir = "/usr/local/share/ca-certificates"
		applicator.RefreshTarget = "debian"
	case types.Arch:
		applicator.AnchorsDir = "/etc/ca-certificates/trust-source/anchors"
		applicator.RefreshTarget = "arch"
	default:
		return nil, fmt.Errorf("trust anchor provider is unsupported for distro %q", distro)
	}
	return applicator, nil
}

// ConfigureRollback binds the named anchor to the agent transaction store.
func (a *Applicator) ConfigureRollback(store *rollbackstore.Store, address, artifactDigest string) error {
	handle, err := rollbackstore.NewHandle(store, address, artifactDigest, false)
	if err != nil {
		return err
	}
	a.rollback = handle
	return nil
}

func (a *Applicator) Name() string { return "trust-anchor:" + a.Resource.Name }

func (a *Applicator) Description() string { return "CA trust anchor " + a.Resource.Name }

func (a *Applicator) path() (string, error) {
	if err := a.Resource.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(a.AnchorsDir) {
		return "", fmt.Errorf("trust anchor directory must be absolute")
	}
	return filepath.Join(a.AnchorsDir, "remotr-"+a.Resource.Name+".crt"), nil
}

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(_ context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("named CA trust anchor " + a.Resource.Name)
	path, err := a.path()
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	material, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(err) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
		}
		if err != nil {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named anchor exists"}
	}
	if os.IsNotExist(err) {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "named anchor is absent"}
	}
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: err}
	}
	fingerprint, err := certificateFingerprint(material)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: errors.New("managed trust anchor is invalid")}
	}
	observed := executor.RedactedSummary("fingerprint=" + fingerprint)
	if sameFingerprint(fingerprint, a.Resource.Fingerprint) {
		return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: observed}
	}
	return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: observed}
}

func (a *Applicator) Apply(ctx context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	previous, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	previousExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	previousMode := os.FileMode(0o644)
	previousUID, previousGID := -1, -1
	if previousExists {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("managed trust anchor path must be a regular file")
		}
		previousMode = info.Mode().Perm()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			previousUID, previousGID = int(stat.Uid), int(stat.Gid)
		}
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := a.armRollback(ctx, path, previous, previousExists, previousMode, previousUID, previousGID); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		return nil
	}
	if a.Resolve == nil {
		return fmt.Errorf("trust anchor %q has no material resolver", a.Resource.Name)
	}
	material, err := a.Resolve(ctx, a.Resource.AnchorRef)
	if err != nil {
		return fmt.Errorf("resolve trust anchor %q: %w", a.Resource.Name, err)
	}
	defer clear(material)
	fingerprint, err := certificateFingerprint(material)
	if err != nil {
		return fmt.Errorf("trust anchor %q is not a valid certificate", a.Resource.Name)
	}
	if !sameFingerprint(fingerprint, a.Resource.Fingerprint) {
		return fmt.Errorf("trust anchor %q fingerprint mismatch: got %s", a.Resource.Name, fingerprint)
	}
	if err := a.armRollback(ctx, path, previous, previousExists, previousMode, previousUID, previousGID); err != nil {
		return err
	}
	if err := atomicWrite(path, material, 0o644, -1, -1); err != nil {
		return err
	}
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	rollbackClass := executor.RollbackNone
	if a.rollback != nil {
		rollbackClass = executor.RollbackTransactional
	}
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Err: err}
	default:
		activation := []executor.ActivationSignal{{Kind: executor.ActivationTrustStoreRefresh, Target: a.RefreshTarget}}
		activation = append(activation, serviceactions.ActivationSignals(a.Resource.Notifications)...)
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: rollbackClass, Activation: activation}
	}
}

func (a *Applicator) Revert(ctx context.Context) error {
	if a.rollback != nil {
		err := a.rollback.Rollback(ctx, func(payload []byte) error {
			var snapshot protectedSnapshot
			if err := json.Unmarshal(payload, &snapshot); err != nil {
				return err
			}
			path, err := a.path()
			if err != nil {
				return err
			}
			return restoreProtectedSnapshot(path, snapshot)
		})
		if errors.Is(err, os.ErrNotExist) {
			return appErr.ErrNoOp
		}
		return err
	}
	if !a.armed {
		return appErr.ErrNoOp
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	if !a.previousExist {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := atomicWrite(path, a.previous, a.previousMode, a.previousUID, a.previousGID); err != nil {
		return err
	}
	a.previous = nil
	a.armed = false
	return nil
}

func (a *Applicator) PreflightRollback(ctx context.Context) error {
	if a.rollback == nil {
		return errors.New("protected trust-anchor rollback is not configured")
	}
	path, err := a.path()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path) // #nosec G304 -- validated named provider path.
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	mode := os.FileMode(0o644)
	uid, gid := -1, -1
	if exists {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("trust anchor rollback target must be regular")
		}
		mode = info.Mode().Perm()
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(stat.Uid), int(stat.Gid)
		}
	}
	payload, err := json.Marshal(protectedSnapshot{
		Version: 1, Path: path, Content: content, Exists: exists, Mode: uint32(mode), UID: uid, GID: gid,
	})
	if err != nil {
		return err
	}
	defer clear(payload)
	return a.rollback.Preflight(ctx, int64(len(payload)))
}

func (a *Applicator) armRollback(ctx context.Context, path string, content []byte, exists bool, mode os.FileMode, uid, gid int) error {
	if a.rollback == nil {
		a.previous, a.previousExist, a.previousMode = append([]byte(nil), content...), exists, mode
		a.previousUID, a.previousGID, a.armed = uid, gid, true
		return nil
	}
	payload, err := json.Marshal(protectedSnapshot{
		Version: 1, Path: path, Content: content, Exists: exists, Mode: uint32(mode), UID: uid, GID: gid,
	})
	if err != nil {
		return err
	}
	defer clear(payload)
	if err := a.rollback.Arm(ctx, payload); err != nil {
		return fmt.Errorf("arm protected trust-anchor rollback: %w", err)
	}
	return nil
}

func restoreProtectedSnapshot(path string, snapshot protectedSnapshot) error {
	if snapshot.Version != 1 || snapshot.Path != path {
		return errors.New("protected trust-anchor rollback identity is invalid")
	}
	if !snapshot.Exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return atomicWrite(path, snapshot.Content, os.FileMode(snapshot.Mode).Perm(), snapshot.UID, snapshot.GID)
}

func certificateFingerprint(material []byte) (string, error) {
	block, _ := pem.Decode(material)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("invalid certificate PEM")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(certificate.Raw)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func sameFingerprint(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.TrimPrefix(value, "sha256:")
		return strings.ReplaceAll(value, ":", "")
	}
	return normalize(left) == normalize(right)
}

func atomicWrite(path string, material []byte, mode os.FileMode, uid, gid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-trust-anchor-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if uid >= 0 || gid >= 0 {
		if err := file.Chown(uid, gid); err != nil {
			file.Close()
			return err
		}
	}
	if _, err := file.Write(material); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
