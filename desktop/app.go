package main

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/DavidHoenisch/remotr/internal/admin"
	opconfig "github.com/DavidHoenisch/remotr/internal/operator/config"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type ExternalLinkOpener func(context.Context, string) error

type AppOption func(*App)

type App struct {
	version        string
	profiles       *ProfileService
	bootstrap      *BootstrapService
	sessions       *SessionManager
	workspace      *WorkspaceService
	endpointDetail *EndpointDetailService
	fleetDetail    *FleetDetailService
	openExternal   ExternalLinkOpener

	contextMu      sync.RWMutex
	lifetime       context.Context
	cancelLifetime context.CancelFunc
}

type ApplicationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func NewApp(version string, options ...AppOption) *App {
	connection := NewConnectionService()
	app := &App{
		version:        version,
		profiles:       NewProfileService(defaultDesktopProfilesPath(), opconfig.DefaultPath()),
		bootstrap:      NewBootstrapService(),
		sessions:       NewSessionManager(connection.ConnectSession),
		workspace:      NewWorkspaceService(),
		endpointDetail: NewEndpointDetailService(),
		fleetDetail:    NewFleetDetailService(),
		openExternal: func(ctx context.Context, target string) error {
			wailsruntime.BrowserOpenURL(ctx, target)
			return nil
		},
		lifetime: context.Background(),
	}
	for _, option := range options {
		if option != nil {
			option(app)
		}
	}
	return app
}

func WithExternalLinkOpener(opener ExternalLinkOpener) AppOption {
	return func(app *App) {
		if opener != nil {
			app.openExternal = opener
		}
	}
}

func (a *App) GetApplicationInfo() ApplicationInfo {
	return ApplicationInfo{
		Name:    "Remotr Desktop",
		Version: a.version,
	}
}

func (a *App) LoadProfiles() ([]ConnectionProfile, error) {
	return a.profiles.LoadProfiles()
}

func (a *App) SaveProfile(profile ConnectionProfile) error {
	return a.profiles.SaveProfile(profile)
}

func (a *App) ConnectProfile(profile ConnectionProfile) (ConnectionView, error) {
	profile = normalizeProfile(profile)
	if err := a.sessions.SwitchProfile(a.applicationContext(), profile); err != nil {
		return ConnectionView{}, err
	}

	state := a.sessions.Snapshot()
	if state.Status != SessionConnected || state.Identity == nil {
		return ConnectionView{}, errors.New("desktop connection did not establish an Operator session")
	}
	return ConnectionView{
		ProfileName: state.ProfileName,
		ServerURL:   profile.ServerURL,
		OperatorID:  state.Identity.OperatorID,
		Roles:       slices.Clone(state.Identity.Roles),
	}, nil
}

func (a *App) BootstrapProfile(profile ConnectionProfile, token string) (ConnectionView, error) {
	attempt := &BootstrapAttempt{
		Profile: profile,
		Token:   []byte(token),
	}
	return a.bootstrap.Bootstrap(a.applicationContext(), attempt)
}

func (a *App) LoadWorkspace() (WorkspaceView, error) {
	var workspace WorkspaceView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		state := a.sessions.Snapshot()
		if state.Identity == nil {
			return ErrSessionNotConnected
		}
		var loadErr error
		workspace, loadErr = a.workspace.loadConnected(ctx, *state.Identity, client)
		return loadErr
	})
	if err != nil {
		return WorkspaceView{}, err
	}
	return workspace, nil
}

func (a *App) LoadEndpointDetail(endpointID string) (EndpointDetailView, error) {
	var detail EndpointDetailView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		detail, loadErr = a.endpointDetail.LoadConnected(ctx, client, endpointID)
		return loadErr
	})
	if err != nil {
		return EndpointDetailView{}, err
	}
	return detail, nil
}

func (a *App) LoadFleetDetail(fleet string) (FleetDetailView, error) {
	var detail FleetDetailView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		detail, loadErr = a.fleetDetail.LoadConnected(ctx, client, fleet)
		return loadErr
	})
	if err != nil {
		return FleetDetailView{}, err
	}
	return detail, nil
}

func (a *App) OpenExternalLink(target string) error {
	target = strings.TrimSpace(target)
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return errors.New("external links must use an absolute HTTPS URL without credentials")
	}
	if a.openExternal == nil {
		return errors.New("native external-link handoff is unavailable")
	}
	return a.openExternal(a.applicationContext(), target)
}

func (a *App) startup(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	lifetime, cancel := context.WithCancel(ctx)

	a.contextMu.Lock()
	previousCancel := a.cancelLifetime
	a.lifetime = lifetime
	a.cancelLifetime = cancel
	a.contextMu.Unlock()

	if previousCancel != nil {
		previousCancel()
	}
}

func (a *App) shutdown(context.Context) {
	a.contextMu.Lock()
	cancel := a.cancelLifetime
	a.cancelLifetime = nil
	a.lifetime = context.Background()
	a.contextMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) applicationContext() context.Context {
	a.contextMu.RLock()
	defer a.contextMu.RUnlock()
	return a.lifetime
}

func defaultDesktopProfilesPath() string {
	return filepath.Join(filepath.Dir(opconfig.DefaultPath()), "desktop-profiles.json")
}
