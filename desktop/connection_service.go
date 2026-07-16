package main

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"github.com/DavidHoenisch/remotr/internal/tlsconfig"
)

type ConnectionFailureKind string

const (
	ConnectionCredentialsMissing ConnectionFailureKind = "credentials_missing"
	ConnectionUnknownCA          ConnectionFailureKind = "unknown_ca"
	ConnectionCredentialExpired  ConnectionFailureKind = "credential_expired"
	ConnectionCredentialInvalid  ConnectionFailureKind = "credential_invalid"
	ConnectionServerUnreachable  ConnectionFailureKind = "server_unreachable"
	ConnectionIdentityForbidden  ConnectionFailureKind = "identity_forbidden"
	ConnectionUnexpected         ConnectionFailureKind = "unexpected"
)

type ConnectionFailure struct {
	Kind               ConnectionFailureKind `json:"kind"`
	Message            string                `json:"message"`
	Guidance           string                `json:"guidance"`
	BootstrapAvailable bool                  `json:"bootstrapAvailable"`
}

func (e *ConnectionFailure) Error() string {
	return e.Message
}

type ConnectionView struct {
	ProfileName string   `json:"profileName"`
	ServerURL   string   `json:"serverUrl"`
	OperatorID  string   `json:"operatorId"`
	Roles       []string `json:"roles"`
}

type ConnectionService struct{}

type authenticatedConnection struct {
	view   ConnectionView
	client *admin.Client
}

func NewConnectionService() *ConnectionService {
	return &ConnectionService{}
}

func (s *ConnectionService) Connect(ctx context.Context, profile ConnectionProfile) (ConnectionView, error) {
	connected, err := s.connect(ctx, profile)
	if err != nil {
		return ConnectionView{}, err
	}
	return connected.view, nil
}

func (s *ConnectionService) connect(ctx context.Context, profile ConnectionProfile) (authenticatedConnection, error) {
	profile = normalizeProfile(profile)
	if err := validateProfile(profile); err != nil {
		return authenticatedConnection{}, err
	}
	if admin.DemoEnabled() {
		return authenticatedConnection{}, connectionFailure(
			ConnectionUnexpected,
			"Desktop connections require a live Remotr server.",
			"Unset CLI demo mode before connecting Remotr Desktop.",
			false,
		)
	}
	if !opcreds.Present(profile.StateDir) {
		return authenticatedConnection{}, connectionFailure(
			ConnectionCredentialsMissing,
			"Operator credentials are missing.",
			"Bootstrap the first Operator or select a profile with a complete Operator state directory.",
			true,
		)
	}

	layout, err := opcreds.Layout(profile.StateDir)
	if err != nil {
		return authenticatedConnection{}, connectionFailure(
			ConnectionCredentialInvalid,
			"Operator credentials could not be loaded.",
			"Select a profile with a valid protected Operator credential layout.",
			false,
		)
	}
	caPath := profile.CAPath
	if caPath == "" {
		caPath = layout.CA
	}
	tlsConfig, err := tlsconfig.ClientTLSConfig(layout.Cert, layout.Key, caPath)
	if err != nil {
		return authenticatedConnection{}, classifyConnectionError(err)
	}
	client, err := admin.NewClient(strings.TrimRight(profile.ServerURL, "/"), profile.StateDir, tlsConfig)
	if err != nil {
		return authenticatedConnection{}, classifyConnectionError(err)
	}
	client.HTTPClient.Timeout = 15 * time.Second

	identity, err := client.GetOperatorMeContext(ctx)
	if err != nil {
		if contextError := context.Cause(ctx); contextError != nil {
			return authenticatedConnection{}, contextError
		}
		return authenticatedConnection{}, classifyConnectionError(err)
	}
	if strings.TrimSpace(identity.OperatorID) == "" {
		return authenticatedConnection{}, connectionFailure(
			ConnectionUnexpected,
			"The Remotr server returned an invalid Operator identity.",
			"Verify the server is healthy and try again.",
			false,
		)
	}
	return authenticatedConnection{
		view: ConnectionView{
			ProfileName: profile.Name,
			ServerURL:   profile.ServerURL,
			OperatorID:  identity.OperatorID,
			Roles:       slices.Clone(identity.Roles),
		},
		client: client,
	}, nil
}

func (s *ConnectionService) ConnectSession(ctx context.Context, profile ConnectionProfile) (ConnectedSession, error) {
	connected, err := s.connect(ctx, profile)
	if err != nil {
		return ConnectedSession{}, err
	}
	return ConnectedSession{
		Identity: OperatorIdentity{
			OperatorID: connected.view.OperatorID,
			Roles:      slices.Clone(connected.view.Roles),
		},
		client: connected.client,
	}, nil
}

func classifyConnectionError(err error) error {
	var responseErr *admin.OperatorMeResponseError
	if errors.As(err, &responseErr) && responseErr.StatusCode == http.StatusForbidden {
		return connectionFailure(
			ConnectionIdentityForbidden,
			"This Operator is not authorized to connect.",
			"Ask a Remotr administrator to verify the Operator credential and its assigned roles.",
			false,
		)
	}

	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return connectionFailure(
			ConnectionUnknownCA,
			"The Remotr server certificate is not trusted.",
			"Verify the profile's CA certificate reference before reconnecting.",
			false,
		)
	}
	lowerError := strings.ToLower(err.Error())
	if strings.Contains(lowerError, "expired certificate") || strings.Contains(lowerError, "certificate has expired") || strings.Contains(lowerError, "certificate expired") {
		return connectionFailure(
			ConnectionCredentialExpired,
			"The Operator credential has expired.",
			"Replace the expired Operator credential before reconnecting.",
			false,
		)
	}
	if strings.Contains(lowerError, "load client key pair") || strings.Contains(lowerError, "private key") || strings.Contains(lowerError, "failed to find any pem data") {
		return connectionFailure(
			ConnectionCredentialInvalid,
			"Operator credentials could not be loaded.",
			"Restore a complete protected Operator credential set or bootstrap again.",
			false,
		)
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		return connectionFailure(
			ConnectionServerUnreachable,
			"The Remotr server could not be reached.",
			"Check the server address and network connectivity, then try again.",
			false,
		)
	}
	return connectionFailure(
		ConnectionUnexpected,
		"The Remotr connection failed.",
		"Review the profile references and try again.",
		false,
	)
}

func connectionFailure(kind ConnectionFailureKind, message, guidance string, bootstrapAvailable bool) *ConnectionFailure {
	return &ConnectionFailure{
		Kind:               kind,
		Message:            message,
		Guidance:           guidance,
		BootstrapAvailable: bootstrapAvailable,
	}
}
