// Package certificates manages X.509 certificate/private-key pairs.
package certificates

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	serviceactions "github.com/DavidHoenisch/remotr/internal/applicators/services"
	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/secrets"
)

// ResolveFunc obtains bounded material through the configured secret
// provider. Implementations must not place returned bytes in argv or the
// environment.
type ResolveFunc func(context.Context, string) ([]byte, error)
type ResolvePurposeFunc func(context.Context, string, string) ([]byte, error)

type snapshot struct {
	certificate, privateKey             []byte
	certificateExists, privateKeyExists bool
	certificateMode, privateKeyMode     os.FileMode
	certificateUID, certificateGID      int
	privateKeyUID, privateKeyGID        int
}

type Applicator struct {
	Resource           models.CertificateResource
	Resolve            ResolveFunc
	ResolveWithPurpose ResolvePurposeFunc
	Now                func() time.Time
	previous           snapshot
	armed              bool
}

func New(resource models.CertificateResource) *Applicator {
	if resource.Lifecycle == "" {
		resource.Lifecycle = models.LifecyclePresent
	}
	return &Applicator{Resource: resource, Now: time.Now}
}

func (a *Applicator) Name() string { return "certificate:" + a.Resource.Name }

func (a *Applicator) Description() string { return "certificate " + a.Resource.Name }

func (a *Applicator) State(ctx context.Context) (any, bool) {
	check := a.Check(ctx)
	return check.ObservedSummary, check.Status == executor.Compliant
}

func (a *Applicator) Check(_ context.Context) executor.CheckResult {
	desired := executor.RedactedSummary("certificate pair " + a.Resource.Name)
	certificatePEM, certificateErr := os.ReadFile(a.Resource.CertificatePath) // #nosec G304 -- validated absolute managed path.
	privateKeyPEM, keyErr := os.ReadFile(a.Resource.PrivateKeyPath)           // #nosec G304 -- validated absolute managed path.
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if os.IsNotExist(certificateErr) && os.IsNotExist(keyErr) {
			return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired}
		}
		if (certificateErr != nil && !os.IsNotExist(certificateErr)) || (keyErr != nil && !os.IsNotExist(keyErr)) {
			return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: errors.Join(certificateErr, keyErr)}
		}
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "certificate pair remains present"}
	}
	if certificateErr != nil || keyErr != nil {
		if os.IsNotExist(certificateErr) || os.IsNotExist(keyErr) {
			return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: "certificate or private key is absent"}
		}
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: errors.Join(certificateErr, keyErr)}
	}
	defer clear(privateKeyPEM)
	certificate, fingerprint, err := inspectPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, Err: fmt.Errorf("active certificate pair is invalid")}
	}
	observed := safeSummary(certificate, fingerprint)
	compliant, err := a.certificateMeetsPolicy(certificate, fingerprint)
	if err != nil {
		return executor.CheckResult{Status: executor.CheckFailed, ReasonCode: executor.ReasonProbeFailed, DesiredSummary: desired, ObservedSummary: observed, Err: err}
	}
	if compliant {
		compliant = a.metadataMet(a.Resource.CertificatePath, a.certificateMode()) && a.metadataMet(a.Resource.PrivateKeyPath, a.privateKeyMode())
	}
	if !compliant {
		return executor.CheckResult{Status: executor.Drifted, ReasonCode: executor.ReasonStateDrift, DesiredSummary: desired, ObservedSummary: observed}
	}
	return executor.CheckResult{Status: executor.Compliant, ReasonCode: executor.ReasonCompliant, DesiredSummary: desired, ObservedSummary: observed}
}

func (a *Applicator) Apply(ctx context.Context) error {
	if !filepath.IsAbs(a.Resource.CertificatePath) || !filepath.IsAbs(a.Resource.PrivateKeyPath) {
		return fmt.Errorf("certificate and private-key paths must be absolute")
	}
	if check := a.Check(ctx); check.Status == executor.Compliant {
		return appErr.ErrStateAlreadyMet
	}
	previous, err := captureSnapshot(a.Resource.CertificatePath, a.Resource.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("capture protected certificate rollback state: %w", err)
	}
	if a.Resource.Lifecycle == models.LifecycleAbsent {
		if err := removeRegular(a.Resource.CertificatePath); err != nil {
			return err
		}
		if err := removeRegular(a.Resource.PrivateKeyPath); err != nil {
			_ = restoreSnapshot(a.Resource.CertificatePath, a.Resource.PrivateKeyPath, previous)
			return err
		}
		a.previous, a.armed = previous, true
		return nil
	}
	if a.Resolve == nil && a.ResolveWithPurpose == nil {
		return fmt.Errorf("certificate %q has no material resolver", a.Resource.Name)
	}
	certificatePEM, err := a.resolve(ctx, a.Resource.CertificateRef, "certificate-public")
	if err != nil {
		return fmt.Errorf("resolve certificate %q: %w", a.Resource.Name, secrets.RedactedResolutionError(err))
	}
	defer clear(certificatePEM)
	for _, reference := range a.Resource.ChainRefs {
		chain, err := a.resolve(ctx, reference, "certificate-chain")
		if err != nil {
			return fmt.Errorf("resolve certificate chain for %q: %w", a.Resource.Name, secrets.RedactedResolutionError(err))
		}
		certificatePEM = append(certificatePEM, '\n')
		certificatePEM = append(certificatePEM, chain...)
		clear(chain)
	}
	privateKeyPEM, err := a.resolve(ctx, a.Resource.PrivateKeyRef, "certificate-private-key")
	if err != nil {
		return fmt.Errorf("resolve private key for certificate %q: %w", a.Resource.Name, secrets.RedactedResolutionError(err))
	}
	defer clear(privateKeyPEM)
	certificate, fingerprint, err := inspectPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return fmt.Errorf("certificate %q and private key do not match: %w", a.Resource.Name, err)
	}
	if compliant, err := a.certificateMeetsPolicy(certificate, fingerprint); err != nil {
		return err
	} else if !compliant {
		return fmt.Errorf("resolved certificate %q does not satisfy subject, SAN, fingerprint, or renewal policy", a.Resource.Name)
	}
	uid, gid, err := desiredOwnership(a.Resource.Owner, a.Resource.Group)
	if err != nil {
		return err
	}
	certificateStage, err := stage(a.Resource.CertificatePath, certificatePEM, a.certificateMode(), uid, gid)
	if err != nil {
		return err
	}
	defer os.Remove(certificateStage)
	keyStage, err := stage(a.Resource.PrivateKeyPath, privateKeyPEM, a.privateKeyMode(), uid, gid)
	if err != nil {
		return err
	}
	defer os.Remove(keyStage)
	if err := os.Rename(certificateStage, a.Resource.CertificatePath); err != nil {
		return fmt.Errorf("activate certificate %q: %w", a.Resource.Name, err)
	}
	if err := os.Rename(keyStage, a.Resource.PrivateKeyPath); err != nil {
		_ = restoreSnapshot(a.Resource.CertificatePath, a.Resource.PrivateKeyPath, previous)
		return fmt.Errorf("activate private key for certificate %q: %w", a.Resource.Name, err)
	}
	a.previous, a.armed = previous, true
	return nil
}

func (a *Applicator) resolve(ctx context.Context, reference, purpose string) ([]byte, error) {
	if a.ResolveWithPurpose != nil {
		return a.ResolveWithPurpose(ctx, reference, purpose)
	}
	return a.Resolve(ctx, reference)
}

func (a *Applicator) ApplyResult(ctx context.Context) executor.ApplyResult {
	err := a.Apply(ctx)
	switch {
	case errors.Is(err, appErr.ErrStateAlreadyMet):
		return executor.ApplyResult{Status: executor.NoChange, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackTransactional}
	case err != nil:
		return executor.ApplyResult{Status: executor.Failed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackTransactional, Err: err}
	default:
		return executor.ApplyResult{Status: executor.Changed, RebootRequired: executor.RebootNotRequired, RollbackClass: executor.RollbackTransactional, Activation: serviceactions.ActivationSignals(a.Resource.Notifications)}
	}
}

func (a *Applicator) Revert(context.Context) error {
	if !a.armed {
		return appErr.ErrNoOp
	}
	if err := restoreSnapshot(a.Resource.CertificatePath, a.Resource.PrivateKeyPath, a.previous); err != nil {
		return err
	}
	clear(a.previous.privateKey)
	a.previous = snapshot{}
	a.armed = false
	return nil
}

func inspectPair(certificatePEM, privateKeyPEM []byte) (*x509.Certificate, string, error) {
	pair, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return nil, "", err
	}
	if len(pair.Certificate) == 0 {
		return nil, "", errors.New("certificate chain is empty")
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(certificate.Raw)
	return certificate, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (a *Applicator) certificateMeetsPolicy(certificate *x509.Certificate, fingerprint string) (bool, error) {
	if a.Resource.Subject != "" && certificate.Subject.String() != a.Resource.Subject {
		return false, nil
	}
	if len(a.Resource.SANs) > 0 {
		want, got := append([]string(nil), a.Resource.SANs...), append([]string(nil), certificate.DNSNames...)
		sort.Strings(want)
		sort.Strings(got)
		if !slices.Equal(want, got) {
			return false, nil
		}
	}
	if a.Resource.Fingerprint != "" && !strings.EqualFold(normalizeFingerprint(a.Resource.Fingerprint), normalizeFingerprint(fingerprint)) {
		return false, nil
	}
	if a.Now().Before(certificate.NotBefore) || !a.Now().Before(certificate.NotAfter) {
		return false, nil
	}
	if a.Resource.RenewBefore != "" {
		threshold, err := time.ParseDuration(a.Resource.RenewBefore)
		if err != nil || threshold <= 0 {
			return false, fmt.Errorf("certificate renewBefore must be a positive duration")
		}
		if !a.Now().Add(threshold).Before(certificate.NotAfter) {
			return false, nil
		}
	}
	return true, nil
}

func safeSummary(certificate *x509.Certificate, fingerprint string) executor.RedactedSummary {
	return executor.RedactedSummary(fmt.Sprintf("fingerprint=%s expires=%s subject=%s", fingerprint, certificate.NotAfter.UTC().Format(time.RFC3339), certificate.Subject.String()))
}

func normalizeFingerprint(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "sha256:")
	return strings.ReplaceAll(value, ":", "")
}

func (a *Applicator) certificateMode() os.FileMode {
	if len(a.Resource.CertificateMode) > 0 {
		return os.FileMode(a.Resource.CertificateMode[0] & 0o777)
	}
	return 0o644
}

func (a *Applicator) privateKeyMode() os.FileMode {
	if len(a.Resource.PrivateKeyMode) > 0 {
		return os.FileMode(a.Resource.PrivateKeyMode[0] & 0o777)
	}
	return 0o600
}

func (a *Applicator) metadataMet(path string, mode os.FileMode) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != mode {
		return false
	}
	uid, gid, err := desiredOwnership(a.Resource.Owner, a.Resource.Group)
	if err != nil || (uid < 0 && gid < 0) {
		return err == nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && (uid < 0 || int(stat.Uid) == uid) && (gid < 0 || int(stat.Gid) == gid)
}

func desiredOwnership(owner, group string) (int, int, error) {
	uid, gid := -1, -1
	if owner != "" {
		entry, err := user.Lookup(owner)
		if err != nil {
			return -1, -1, fmt.Errorf("certificate owner %q: %w", owner, err)
		}
		uid, err = strconv.Atoi(entry.Uid)
		if err != nil {
			return -1, -1, err
		}
	}
	if group != "" {
		entry, err := user.LookupGroup(group)
		if err != nil {
			return -1, -1, fmt.Errorf("certificate group %q: %w", group, err)
		}
		gid, err = strconv.Atoi(entry.Gid)
		if err != nil {
			return -1, -1, err
		}
	}
	return uid, gid, nil
}

func stage(path string, material []byte, mode os.FileMode, uid, gid int) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("certificate path %q must be absolute", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".remotr-certificate-")
	if err != nil {
		return "", err
	}
	name := file.Name()
	fail := func(err error) (string, error) {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Chmod(mode); err != nil {
		return fail(err)
	}
	if uid >= 0 || gid >= 0 {
		if err := file.Chown(uid, gid); err != nil {
			return fail(err)
		}
	}
	if _, err := file.Write(material); err != nil {
		return fail(err)
	}
	if err := file.Sync(); err != nil {
		return fail(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func captureSnapshot(certificatePath, keyPath string) (snapshot, error) {
	var out snapshot
	var err error
	if out.certificate, out.certificateExists, out.certificateMode, out.certificateUID, out.certificateGID, err = readSnapshotFile(certificatePath); err != nil {
		return snapshot{}, err
	}
	if out.privateKey, out.privateKeyExists, out.privateKeyMode, out.privateKeyUID, out.privateKeyGID, err = readSnapshotFile(keyPath); err != nil {
		clear(out.certificate)
		return snapshot{}, err
	}
	return out, nil
}

func readSnapshotFile(path string) ([]byte, bool, os.FileMode, int, int, error) {
	material, err := os.ReadFile(path) // #nosec G304 -- managed path validated by caller.
	if os.IsNotExist(err) {
		return nil, false, 0, -1, -1, nil
	}
	if err != nil {
		return nil, false, 0, -1, -1, err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		clear(material)
		return nil, false, 0, -1, -1, fmt.Errorf("managed certificate path must be a regular file")
	}
	uid, gid := -1, -1
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid, gid = int(stat.Uid), int(stat.Gid)
	}
	return material, true, info.Mode().Perm(), uid, gid, nil
}

func restoreSnapshot(certificatePath, keyPath string, previous snapshot) error {
	if err := restoreFile(certificatePath, previous.certificate, previous.certificateExists, previous.certificateMode, previous.certificateUID, previous.certificateGID); err != nil {
		return err
	}
	return restoreFile(keyPath, previous.privateKey, previous.privateKeyExists, previous.privateKeyMode, previous.privateKeyUID, previous.privateKeyGID)
}

func restoreFile(path string, material []byte, exists bool, mode os.FileMode, uid, gid int) error {
	if !exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	staged, err := stage(path, material, mode, uid, gid)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	return os.Rename(staged, path)
}

func removeRegular(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-regular certificate path %q", path)
	}
	return os.Remove(path)
}

// DecodeCertificatePEM returns the leaf certificate for provider contract
// tests without exposing private-key material.
func DecodeCertificatePEM(material []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(material)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate PEM is invalid")
	}
	return x509.ParseCertificate(block.Bytes)
}
