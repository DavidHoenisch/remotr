// Package trustanchors manages named system CA trust anchors.
package trustanchors

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	serviceactions "github.com/DavidHoenisch/remotr/internal/applicators/services"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
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
	previousExist bool
	armed         bool
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
	if previousExists {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("managed trust anchor path must be a regular file")
		}
		previousMode = info.Mode().Perm()
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := os.Remove(path); err != nil {
			return err
		}
		a.previous, a.previousExist, a.previousMode, a.armed = previous, true, previousMode, true
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
	if err := atomicWrite(path, material, 0o644); err != nil {
		return err
	}
	a.previous, a.previousExist, a.previousMode, a.armed = previous, previousExists, previousMode, true
	return nil
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Err: err}
	default:
		activation := []executor.ActivationSignal{{Kind: executor.ActivationTrustStoreRefresh, Target: a.RefreshTarget}}
		activation = append(activation, serviceactions.ActivationSignals(a.Resource.Notifications)...)
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackBestEffort, Activation: activation}
	}
}

func (a *Applicator) Revert(_ context.Context) error {
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
	} else if err := atomicWrite(path, a.previous, a.previousMode); err != nil {
		return err
	}
	a.previous = nil
	a.armed = false
	return nil
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

func atomicWrite(path string, material []byte, mode os.FileMode) error {
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
