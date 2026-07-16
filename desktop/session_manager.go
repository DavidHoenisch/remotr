package main

import (
	"context"
	"errors"
	"slices"
	"sync"
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

type SessionManager struct {
	mu         sync.RWMutex
	connect    ProfileConnector
	generation uint64
	cancel     context.CancelCauseFunc
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
	m.cancel = nil
	cancel(nil)
	if connectErr != nil {
		m.state.Status = SessionConnectionFailed
		m.state.ConnectionError = connectErr.Error()
		return connectErr
	}
	if cause != nil {
		m.state.Status = SessionConnectionFailed
		m.state.ConnectionError = cause.Error()
		return cause
	}

	identity := cloneOperatorIdentity(connected.Identity)
	m.state.Status = SessionConnected
	m.state.Identity = &identity
	m.state.Workspace = cloneWorkspaceSnapshot(connected.Workspace)
	return nil
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
