package main

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

var (
	ErrObsoleteProfileSwitch = errors.New("obsolete profile switch")
	ErrSessionNotConnected   = errors.New("desktop session is not connected")
)

type SessionStatus string

const (
	SessionDisconnected     SessionStatus = "disconnected"
	SessionConnecting       SessionStatus = "connecting"
	SessionConnected        SessionStatus = "connected"
	SessionConnectionFailed SessionStatus = "connection_failed"
)

type OperatorIdentity struct {
	OperatorID string
	Roles      []string
}

type WorkspaceSnapshot struct {
	EndpointIDs []string
}

type ConnectedSession struct {
	Identity  OperatorIdentity
	Workspace WorkspaceSnapshot
	client    *admin.Client
}

type SessionLocalState struct {
	SelectedEndpointID string
	Overlay            string
	TransientResult    string
}

type SessionState struct {
	ProfileName     string
	Status          SessionStatus
	Identity        *OperatorIdentity
	Workspace       WorkspaceSnapshot
	Local           SessionLocalState
	ConnectionError string
}

type ProfileConnector func(context.Context, ConnectionProfile) (ConnectedSession, error)

type AuthenticatedAction func(context.Context, *admin.Client) error

type SessionManager struct {
	mu         sync.RWMutex
	connect    ProfileConnector
	generation uint64
	cancel     context.CancelCauseFunc
	client     *admin.Client
	sessionCtx context.Context
	state      SessionState
}

func NewSessionManager(connect ProfileConnector) *SessionManager {
	return &SessionManager{
		connect: connect,
		state: SessionState{
			Status: SessionDisconnected,
		},
	}
}

func (m *SessionManager) SwitchProfile(ctx context.Context, profile ConnectionProfile) error {
	profile = normalizeProfile(profile)
	if err := validateProfile(profile); err != nil {
		return err
	}
	if m.connect == nil {
		return errors.New("profile connection boundary is not configured")
	}

	connectionContext, cancel := context.WithCancelCause(ctx)
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel(ErrObsoleteProfileSwitch)
	}
	m.generation++
	generation := m.generation
	m.cancel = cancel
	m.client = nil
	m.sessionCtx = nil
	m.state = SessionState{
		ProfileName: profile.Name,
		Status:      SessionConnecting,
	}
	m.mu.Unlock()

	connected, connectErr := m.connect(connectionContext, profile)

	m.mu.Lock()
	defer m.mu.Unlock()
	if generation != m.generation {
		cancel(ErrObsoleteProfileSwitch)
		return ErrObsoleteProfileSwitch
	}
	cause := context.Cause(connectionContext)
	if connectErr != nil {
		m.cancel = nil
		cancel(nil)
		m.state.Status = SessionConnectionFailed
		m.state.ConnectionError = connectErr.Error()
		return connectErr
	}
	if cause != nil {
		m.cancel = nil
		cancel(nil)
		m.state.Status = SessionConnectionFailed
		m.state.ConnectionError = cause.Error()
		return cause
	}

	identity := cloneOperatorIdentity(connected.Identity)
	m.state.Status = SessionConnected
	m.state.Identity = &identity
	m.state.Workspace = cloneWorkspaceSnapshot(connected.Workspace)
	m.client = connected.client
	m.sessionCtx = connectionContext
	return nil
}

func (m *SessionManager) ExecuteAuthenticatedAction(ctx context.Context, action AuthenticatedAction) error {
	if action == nil {
		return errors.New("authenticated action is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.RLock()
	if m.state.Status != SessionConnected || m.client == nil || m.sessionCtx == nil {
		m.mu.RUnlock()
		return ErrSessionNotConnected
	}
	client := m.client
	sessionCtx := m.sessionCtx
	generation := m.generation
	m.mu.RUnlock()

	actionCtx, cancel := context.WithCancelCause(sessionCtx)
	stopCallerCancellation := context.AfterFunc(ctx, func() {
		cancel(context.Cause(ctx))
	})
	defer func() {
		stopCallerCancellation()
		cancel(nil)
	}()
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}

	actionErr := action(actionCtx, client)
	if cause := context.Cause(actionCtx); cause != nil {
		return cause
	}

	m.mu.RLock()
	stillCurrent := generation == m.generation && m.state.Status == SessionConnected && m.client == client
	m.mu.RUnlock()
	if !stillCurrent {
		return ErrObsoleteProfileSwitch
	}
	return classifyAuthenticatedActionError(actionErr)
}

func (m *SessionManager) UpdateLocalState(state SessionLocalState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Status != SessionConnected {
		return ErrSessionNotConnected
	}
	m.state.Local = state
	return nil
}

func (m *SessionManager) Snapshot() SessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := m.state
	if state.Identity != nil {
		identity := cloneOperatorIdentity(*state.Identity)
		state.Identity = &identity
	}
	state.Workspace = cloneWorkspaceSnapshot(state.Workspace)
	return state
}

func cloneOperatorIdentity(identity OperatorIdentity) OperatorIdentity {
	identity.Roles = slices.Clone(identity.Roles)
	return identity
}

func cloneWorkspaceSnapshot(snapshot WorkspaceSnapshot) WorkspaceSnapshot {
	snapshot.EndpointIDs = slices.Clone(snapshot.EndpointIDs)
	return snapshot
}
