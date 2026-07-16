package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestSessionManagerCancelsObsoleteProfileSwitch(t *testing.T) {
	slowStarted := make(chan struct{})
	slowCancelled := make(chan struct{})
	connector := func(ctx context.Context, profile ConnectionProfile) (ConnectedSession, error) {
		switch profile.Name {
		case "Old":
			return ConnectedSession{
				Identity:  OperatorIdentity{OperatorID: "operator-from-old-server", Roles: []string{"viewer"}},
				Workspace: WorkspaceSnapshot{EndpointIDs: []string{"old-endpoint"}},
			}, nil
		case "Slow":
			close(slowStarted)
			<-ctx.Done()
			close(slowCancelled)
			return ConnectedSession{
				Identity:  OperatorIdentity{OperatorID: "operator-from-obsolete-server"},
				Workspace: WorkspaceSnapshot{EndpointIDs: []string{"obsolete-endpoint"}},
			}, nil
		case "New":
			return ConnectedSession{
				Identity:  OperatorIdentity{OperatorID: "operator-from-new-server", Roles: []string{"operator"}},
				Workspace: WorkspaceSnapshot{EndpointIDs: []string{"new-endpoint-a", "new-endpoint-b"}},
			}, nil
		default:
			t.Fatalf("unexpected profile at connection boundary: %s", profile.Name)
			return ConnectedSession{}, errors.New("unreachable")
		}
	}
	manager := NewSessionManager(connector)
	oldProfile := validSessionProfile(t, "Old", "https://old.example:8443")
	if err := manager.SwitchProfile(t.Context(), oldProfile); err != nil {
		t.Fatalf("connect old profile: %v", err)
	}
	oldLocalState := SessionLocalState{
		SelectedEndpointID: "old-endpoint",
		Overlay:            "endpoint-detail",
		TransientResult:    "old-upgrade-request",
	}
	if err := manager.UpdateLocalState(oldLocalState); err != nil {
		t.Fatalf("record old local state: %v", err)
	}

	slowResult := make(chan error, 1)
	slowProfile := validSessionProfile(t, "Slow", "https://slow.example:8443")
	go func() {
		slowResult <- manager.SwitchProfile(t.Context(), slowProfile)
	}()
	<-slowStarted

	assertClearedSessionState(t, manager.Snapshot(), "Slow", SessionConnecting)

	newProfile := validSessionProfile(t, "New", "https://new.example:8443")
	if err := manager.SwitchProfile(t.Context(), newProfile); err != nil {
		t.Fatalf("connect new profile: %v", err)
	}
	<-slowCancelled
	if err := <-slowResult; !errors.Is(err, ErrObsoleteProfileSwitch) {
		t.Fatalf("obsolete slow switch error = %v, want ErrObsoleteProfileSwitch", err)
	}

	got := manager.Snapshot()
	if got.ProfileName != "New" || got.Status != SessionConnected {
		t.Fatalf("active session = %#v, want connected New profile", got)
	}
	wantIdentity := &OperatorIdentity{OperatorID: "operator-from-new-server", Roles: []string{"operator"}}
	if !reflect.DeepEqual(got.Identity, wantIdentity) {
		t.Fatalf("active identity = %#v, want %#v", got.Identity, wantIdentity)
	}
	wantWorkspace := WorkspaceSnapshot{EndpointIDs: []string{"new-endpoint-a", "new-endpoint-b"}}
	if !reflect.DeepEqual(got.Workspace, wantWorkspace) {
		t.Fatalf("active workspace = %#v, want %#v", got.Workspace, wantWorkspace)
	}
	if !reflect.DeepEqual(got.Local, SessionLocalState{}) {
		t.Fatalf("new session retained old local state: %#v", got.Local)
	}
}

func TestSessionManagerFailedSwitchDoesNotMixServers(t *testing.T) {
	errNewServer := errors.New("new server identity unavailable")
	connector := func(_ context.Context, profile ConnectionProfile) (ConnectedSession, error) {
		switch profile.Name {
		case "Old":
			return ConnectedSession{
				Identity:  OperatorIdentity{OperatorID: "operator-from-old-server"},
				Workspace: WorkspaceSnapshot{EndpointIDs: []string{"old-endpoint"}},
			}, nil
		case "Failing":
			return ConnectedSession{}, errNewServer
		default:
			t.Fatalf("unexpected profile at connection boundary: %s", profile.Name)
			return ConnectedSession{}, errors.New("unreachable")
		}
	}
	manager := NewSessionManager(connector)
	if err := manager.SwitchProfile(t.Context(), validSessionProfile(t, "Old", "https://old.example:8443")); err != nil {
		t.Fatalf("connect old profile: %v", err)
	}
	if err := manager.UpdateLocalState(SessionLocalState{
		SelectedEndpointID: "old-endpoint",
		Overlay:            "endpoint-detail",
		TransientResult:    "old-label-update",
	}); err != nil {
		t.Fatalf("record old local state: %v", err)
	}

	err := manager.SwitchProfile(t.Context(), validSessionProfile(t, "Failing", "https://failing.example:8443"))
	if !errors.Is(err, errNewServer) {
		t.Fatalf("failed switch error = %v, want %v", err, errNewServer)
	}
	got := manager.Snapshot()
	assertClearedSessionState(t, got, "Failing", SessionConnectionFailed)
	if got.ConnectionError != errNewServer.Error() {
		t.Fatalf("connection error = %q, want %q", got.ConnectionError, errNewServer)
	}
}

func validSessionProfile(t *testing.T, name, serverURL string) ConnectionProfile {
	t.Helper()
	return ConnectionProfile{
		Name:      name,
		ServerURL: serverURL,
		StateDir:  t.TempDir(),
	}
}

func assertClearedSessionState(t *testing.T, got SessionState, profileName string, status SessionStatus) {
	t.Helper()
	if got.ProfileName != profileName || got.Status != status {
		t.Fatalf("session = %#v, want profile %q with status %q", got, profileName, status)
	}
	if got.Identity != nil {
		t.Errorf("session retained Operator identity: %#v", got.Identity)
	}
	if len(got.Workspace.EndpointIDs) != 0 {
		t.Errorf("session retained workspace rows: %#v", got.Workspace)
	}
	if !reflect.DeepEqual(got.Local, SessionLocalState{}) {
		t.Errorf("session retained selection, overlay, or transient result: %#v", got.Local)
	}
}
