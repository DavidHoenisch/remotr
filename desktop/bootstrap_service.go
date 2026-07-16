package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

type BootstrapAttempt struct {
	Profile ConnectionProfile `json:"profile"`
	Token   []byte            `json:"-"`
}

type BootstrapFailureKind string

const (
	BootstrapRejected          BootstrapFailureKind = "rejected"
	BootstrapPersistenceFailed BootstrapFailureKind = "persistence_failed"
	BootstrapInvalidResponse   BootstrapFailureKind = "invalid_response"
	BootstrapConnectionFailed  BootstrapFailureKind = "connection_failed"
	BootstrapUnexpected        BootstrapFailureKind = "unexpected"
)

type BootstrapFailure struct {
	Kind     BootstrapFailureKind `json:"kind"`
	Message  string               `json:"message"`
	Guidance string               `json:"guidance"`
}

func (e *BootstrapFailure) Error() string {
	return e.Message
}

type CredentialPersistence func(dir, operatorID, certPEM, keyPEM, caPEM string) error

type BootstrapOption func(*BootstrapService)

type BootstrapService struct {
	persist    CredentialPersistence
	connection *ConnectionService
}

func NewBootstrapService(options ...BootstrapOption) *BootstrapService {
	service := &BootstrapService{
		persist:    persistCredentialSet,
		connection: NewConnectionService(),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithCredentialPersistence(persist CredentialPersistence) BootstrapOption {
	return func(service *BootstrapService) {
		if persist != nil {
			service.persist = persist
		}
	}
}

func (s *BootstrapService) Bootstrap(ctx context.Context, attempt *BootstrapAttempt) (ConnectionView, error) {
	if attempt == nil {
		return ConnectionView{}, bootstrapFailure(
			BootstrapUnexpected,
			"Bootstrap input is missing.",
			"Enter a bootstrap token and try again.",
		)
	}
	defer clearSecretBytes(attempt.Token)

	profile := normalizeProfile(attempt.Profile)
	if err := validateProfile(profile); err != nil {
		return ConnectionView{}, err
	}
	if len(attempt.Token) == 0 {
		return ConnectionView{}, bootstrapFailure(
			BootstrapRejected,
			"A bootstrap token is required.",
			"Enter the one-time token issued by the Remotr server.",
		)
	}
	if admin.DemoEnabled() {
		return ConnectionView{}, bootstrapFailure(
			BootstrapUnexpected,
			"Desktop bootstrap requires a live Remotr server.",
			"Unset CLI demo mode before bootstrapping Remotr Desktop.",
		)
	}
	fragmentsPresent, err := credentialFragmentsPresent(profile.StateDir)
	if err != nil || fragmentsPresent {
		return ConnectionView{}, bootstrapFailure(
			BootstrapPersistenceFailed,
			"The Operator state directory already contains credential state.",
			"Select an empty state directory or use the existing Operator profile.",
		)
	}

	trustConfig, err := tlsconfig.TrustOnlyTLSConfig(profile.CAPath)
	if err != nil {
		return ConnectionView{}, bootstrapFailure(
			BootstrapConnectionFailed,
			"The Remotr server trust configuration is invalid.",
			"Verify the profile's CA certificate reference before trying again.",
		)
	}
	client, err := admin.NewClient(strings.TrimRight(profile.ServerURL, "/"), profile.StateDir, trustConfig)
	if err != nil {
		return ConnectionView{}, classifyBootstrapExchangeError(err)
	}
	client.HTTPClient.Timeout = 15 * time.Second

	response, err := client.BootstrapContext(ctx, string(attempt.Token))
	if contextError := context.Cause(ctx); contextError != nil {
		return ConnectionView{}, contextError
	}
	if err != nil {
		return ConnectionView{}, classifyBootstrapExchangeError(err)
	}
	if strings.TrimSpace(response.OperatorID) == "" || response.CertPEM == "" || response.KeyPEM == "" || response.CAPEM == "" {
		return ConnectionView{}, bootstrapFailure(
			BootstrapInvalidResponse,
			"The Remotr server returned an incomplete bootstrap credential.",
			"Verify the server is healthy before requesting another one-time token.",
		)
	}

	if err := s.persist(profile.StateDir, response.OperatorID, response.CertPEM, response.KeyPEM, response.CAPEM); err != nil {
		cleanupCredentialFragments(profile.StateDir)
		return ConnectionView{}, bootstrapFailure(
			BootstrapPersistenceFailed,
			"The Operator credential could not be saved.",
			"Check the Operator state-directory permissions before trying again.",
		)
	}

	view, err := s.connection.Connect(ctx, profile)
	if err != nil {
		cleanupCredentialFragments(profile.StateDir)
		return ConnectionView{}, bootstrapFailure(
			BootstrapConnectionFailed,
			"The issued Operator credential could not be verified.",
			"Check the server trust and request a new bootstrap token.",
		)
	}
	if view.OperatorID != response.OperatorID {
		cleanupCredentialFragments(profile.StateDir)
		return ConnectionView{}, bootstrapFailure(
			BootstrapInvalidResponse,
			"The verified Operator identity did not match the issued credential.",
			"Verify the server is healthy before requesting another one-time token.",
		)
	}
	return view, nil
}

func persistCredentialSet(dir, operatorID, certPEM, keyPEM, caPEM string) error {
	dir = filepath.Clean(dir)
	parent := filepath.Dir(dir)
	base := filepath.Base(dir)
	if dir == parent || base == "." || base == string(filepath.Separator) {
		return errors.New("unsafe Operator state directory")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create Operator state parent: %w", err)
	}

	destinationExists := false
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return errors.New("Operator state path is not a directory")
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("inspect Operator state directory: %w", err)
		}
		if len(entries) != 0 {
			return errors.New("Operator state directory is not empty")
		}
		destinationExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Operator state directory: %w", err)
	}

	staging, err := os.MkdirTemp(parent, "."+base+"-*")
	if err != nil {
		return fmt.Errorf("create staged Operator state: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, 0o700); err != nil {
		return fmt.Errorf("protect staged Operator state: %w", err)
	}
	if err := opcreds.Save(staging, operatorID, certPEM, keyPEM, caPEM); err != nil {
		return err
	}

	if destinationExists {
		if err := os.Remove(dir); err != nil {
			return fmt.Errorf("remove empty Operator state directory: %w", err)
		}
	}
	if err := os.Rename(staging, dir); err != nil {
		return fmt.Errorf("install Operator state directory: %w", err)
	}

	parentHandle, err := os.Open(parent)
	if err != nil {
		return fmt.Errorf("open Operator state parent: %w", err)
	}
	defer parentHandle.Close()
	if err := parentHandle.Sync(); err != nil {
		return fmt.Errorf("sync Operator state parent: %w", err)
	}
	return nil
}

func cleanupCredentialFragments(dir string) {
	layout, err := opcreds.Layout(dir)
	if err != nil {
		return
	}
	for _, path := range []string{layout.Cert, layout.Key, layout.CA, layout.Meta} {
		_ = os.Remove(path)
	}
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
}

func credentialFragmentsPresent(dir string) (bool, error) {
	layout, err := opcreds.Layout(dir)
	if err != nil {
		return false, err
	}
	for _, path := range []string{layout.Cert, layout.Key, layout.CA, layout.Meta} {
		if _, err := os.Lstat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func classifyBootstrapExchangeError(err error) error {
	var payloadError *admin.BootstrapPayloadError
	if errors.As(err, &payloadError) {
		return bootstrapFailure(
			BootstrapInvalidResponse,
			"The Remotr server returned an invalid bootstrap credential.",
			"Verify the server is healthy before requesting another one-time token.",
		)
	}
	var responseError *admin.BootstrapResponseError
	if errors.As(err, &responseError) && responseError.StatusCode >= http.StatusBadRequest && responseError.StatusCode < http.StatusInternalServerError {
		return bootstrapFailure(
			BootstrapRejected,
			"The Remotr server rejected the bootstrap token.",
			"Confirm the one-time token is current and has not already been used.",
		)
	}
	return bootstrapFailure(
		BootstrapConnectionFailed,
		"The bootstrap request could not reach a trusted Remotr server.",
		"Check the server address, CA trust, and network connectivity before trying again.",
	)
}

func bootstrapFailure(kind BootstrapFailureKind, message, guidance string) *BootstrapFailure {
	return &BootstrapFailure{Kind: kind, Message: message, Guidance: guidance}
}

func clearSecretBytes(secret []byte) {
	for index := range secret {
		secret[index] = 0
	}
	runtime.KeepAlive(secret)
}
