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
	version             string
	profiles            *ProfileService
	bootstrap           *BootstrapService
	sessions            *SessionManager
	workspace           *WorkspaceService
	endpointDetail      *EndpointDetailService
	fleetDetail         *FleetDetailService
	changeRequests      *ChangeRequestService
	changeControl       *ChangeControlService
	activity            *ActivityService
	gitSync             *GitSyncService
	enrollment          *EnrollmentTokenService
	deploymentTokens    *DeploymentTokenService
	endpointLabels      *EndpointLabelService
	endpointUpgrade     *EndpointUpgradeService
	fleetUpgrade        *FleetUpgradeService
	diagnostics         *DiagnosticCollectionService
	diagnosticBundles   *DiagnosticBundleSaveService
	endpointRemoval     *EndpointRemovalService
	readExport          *ReadExportService
	applicationPackages *ApplicationPackageService
	secretVersions      *SecretService
	rbacOperators       *RBACOperatorService
	setupMaintenance    *SetupMaintenanceService
	openExternal        ExternalLinkOpener
	writeClipboard      ClipboardWriter

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
		version:             version,
		profiles:            NewProfileService(defaultDesktopProfilesPath(), opconfig.DefaultPath()),
		bootstrap:           NewBootstrapService(),
		sessions:            NewSessionManager(connection.ConnectSession),
		workspace:           NewWorkspaceService(),
		endpointDetail:      NewEndpointDetailService(),
		fleetDetail:         NewFleetDetailService(),
		changeRequests:      NewChangeRequestService(),
		changeControl:       NewChangeControlService(defaultBaselineAdoptionOpenDialog),
		activity:            NewActivityService(),
		gitSync:             NewGitSyncService(),
		enrollment:          NewEnrollmentTokenService(),
		deploymentTokens:    NewDeploymentTokenService(defaultDeploymentTokenSaveDialog),
		endpointLabels:      NewEndpointLabelService(),
		endpointUpgrade:     NewEndpointUpgradeService(),
		fleetUpgrade:        NewFleetUpgradeService(),
		diagnostics:         NewDiagnosticCollectionService(),
		diagnosticBundles:   NewDiagnosticBundleSaveService(defaultDiagnosticBundleSaveDialog),
		endpointRemoval:     NewEndpointRemovalService(),
		readExport:          NewReadExportService(defaultReadExportSaveDialog),
		applicationPackages: NewApplicationPackageService(defaultApplicationPackageDialogs()),
		secretVersions:      defaultSecretService(),
		rbacOperators:       defaultRBACOperatorService(),
		setupMaintenance:    defaultSetupMaintenanceService(version),
		openExternal: func(ctx context.Context, target string) error {
			wailsruntime.BrowserOpenURL(ctx, target)
			return nil
		},
		writeClipboard: wailsruntime.ClipboardSetText,
		lifetime:       context.Background(),
	}
	for _, option := range options {
		if option != nil {
			option(app)
		}
	}
	return app
}

func WithClipboardWriter(writer ClipboardWriter) AppOption {
	return func(app *App) {
		if writer != nil {
			app.writeClipboard = writer
		}
	}
}

func WithExternalLinkOpener(opener ExternalLinkOpener) AppOption {
	return func(app *App) {
		if opener != nil {
			app.openExternal = opener
		}
	}
}

func WithDiagnosticBundleSaveDialog(dialog DiagnosticBundleSaveDialog) AppOption {
	return func(app *App) {
		if dialog != nil {
			app.diagnosticBundles = NewDiagnosticBundleSaveService(dialog)
		}
	}
}

func WithReadExportSaveDialog(dialog ReadExportSaveDialog) AppOption {
	return func(app *App) {
		if dialog != nil {
			app.readExport = NewReadExportService(dialog)
		}
	}
}

func WithDeploymentTokenSaveDialog(dialog DeploymentTokenSaveDialog) AppOption {
	return func(app *App) {
		if dialog != nil {
			app.deploymentTokens = NewDeploymentTokenService(dialog)
		}
	}
}

func WithBaselineAdoptionOpenDialog(dialog BaselineAdoptionOpenDialog) AppOption {
	return func(app *App) {
		if dialog != nil {
			app.changeControl = NewChangeControlService(dialog)
		}
	}
}

func WithApplicationPackageService(service *ApplicationPackageService) AppOption {
	return func(app *App) {
		if service != nil {
			app.applicationPackages = service
		}
	}
}

func WithSecretService(service *SecretService) AppOption {
	return func(app *App) {
		if service != nil {
			app.secretVersions = service
		}
	}
}

func WithRBACOperatorService(service *RBACOperatorService) AppOption {
	return func(app *App) {
		if service != nil {
			app.rbacOperators = service
		}
	}
}

func WithSetupMaintenanceService(service *SetupMaintenanceService) AppOption {
	return func(app *App) {
		if service != nil {
			app.setupMaintenance = service
		}
	}
}

func (a *App) GetApplicationInfo() ApplicationInfo {
	return ApplicationInfo{
		Name:    "Remotr Desktop",
		Version: a.version,
	}
}

func (a *App) LoadSetupMaintenance() (SetupMaintenanceView, error) {
	profiles, err := a.profiles.LoadProfiles()
	if err != nil {
		return SetupMaintenanceView{}, err
	}
	return a.setupMaintenance.Load(profiles)
}

func (a *App) RunDesktopDoctor(profile ConnectionProfile) (DesktopDoctorReport, error) {
	return a.setupMaintenance.Doctor(a.applicationContext(), profile)
}

func (a *App) OpenRemotrDocumentation() error {
	if a.openExternal == nil {
		return errors.New("native external-link handoff is unavailable")
	}
	return a.openExternal(a.applicationContext(), remotrDocumentationURL)
}

func (a *App) CheckDesktopUpdate() (DesktopUpdateStatus, error) {
	return a.setupMaintenance.CheckUpdate(a.applicationContext())
}

func (a *App) CreateLocalPackage(request LocalPackageCreateRequest) (LocalPackageView, error) {
	return a.applicationPackages.CreateLocal(a.applicationContext(), request)
}

func (a *App) ChooseLocalPackageSource() (LocalPackageView, error) {
	return a.applicationPackages.ChooseSource(a.applicationContext())
}

func (a *App) BuildLocalPackage() (AppPackageArchiveView, error) {
	return a.applicationPackages.Build(a.applicationContext())
}

func (a *App) ChooseAppPackageArchive() (AppPackageArchiveView, error) {
	return a.applicationPackages.ChooseArchive(a.applicationContext())
}

func (a *App) ListAppPackages(prefix string) ([]AppPackageView, error) {
	var views []AppPackageView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		views, loadErr = a.applicationPackages.ListConnected(ctx, client, prefix)
		return loadErr
	})
	return views, err
}

func (a *App) LoadAppPackage(name, version string) (AppPackageView, error) {
	var view AppPackageView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.applicationPackages.LoadConnected(ctx, client, name, version)
		return loadErr
	})
	return view, err
}

func (a *App) PublishAppPackage(request AppPackagePublishRequest) (AppPackageView, error) {
	var view AppPackageView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var publishErr error
		view, publishErr = a.applicationPackages.PublishConnected(ctx, client, request)
		return publishErr
	})
	return view, err
}

func (a *App) DeleteAppPackage(request AppPackageDeleteRequest) (AppPackageDeleteResult, error) {
	var result AppPackageDeleteResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var deleteErr error
		result, deleteErr = a.applicationPackages.DeleteConnected(ctx, client, request)
		return deleteErr
	})
	return result, err
}

func (a *App) UploadSecretVersion(request SecretUploadRequest) (SecretVersionView, error) {
	var view SecretVersionView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var uploadErr error
		view, uploadErr = a.secretVersions.UploadConnected(ctx, client, request)
		return uploadErr
	})
	return view, err
}

func (a *App) ListSecretVersions(name string) ([]SecretVersionView, error) {
	var views []SecretVersionView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var listErr error
		views, listErr = a.secretVersions.ListConnected(ctx, client, name)
		return listErr
	})
	return views, err
}

func (a *App) ActivateSecretVersion(request SecretLifecycleRequest) (SecretVersionView, error) {
	var view SecretVersionView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var activateErr error
		view, activateErr = a.secretVersions.ActivateConnected(ctx, client, request)
		return activateErr
	})
	return view, err
}

func (a *App) RevokeSecretVersion(request SecretLifecycleRequest) (SecretVersionView, error) {
	var view SecretVersionView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var revokeErr error
		view, revokeErr = a.secretVersions.RevokeConnected(ctx, client, request)
		return revokeErr
	})
	return view, err
}

func (a *App) ListDesktopRBACRoles() ([]RBACRoleView, error) {
	var views []RBACRoleView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var listErr error
		views, listErr = a.rbacOperators.ListRoles(ctx, client)
		return listErr
	})
	return views, err
}

func (a *App) GetDesktopRBACRole(name string) (RBACRoleView, error) {
	var view RBACRoleView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.rbacOperators.GetRole(ctx, client, name)
		return loadErr
	})
	return view, err
}

func (a *App) CreateDesktopRBACRole(request RBACRoleCreateRequest) (RBACRoleView, error) {
	var view RBACRoleView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var createErr error
		view, createErr = a.rbacOperators.CreateRole(ctx, client, request)
		return createErr
	})
	return view, err
}

func (a *App) DeleteDesktopRBACRole(request RBACRoleDeleteRequest) (RBACMutationResult, error) {
	var result RBACMutationResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var deleteErr error
		result, deleteErr = a.rbacOperators.DeleteRole(ctx, client, request)
		return deleteErr
	})
	return result, err
}

func (a *App) AddDesktopRBACRule(request RBACRuleAddRequest) (RBACRuleView, error) {
	var view RBACRuleView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var addErr error
		view, addErr = a.rbacOperators.AddRule(ctx, client, request)
		return addErr
	})
	return view, err
}

func (a *App) RemoveDesktopRBACRule(request RBACRuleRemoveRequest) (RBACMutationResult, error) {
	var result RBACMutationResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var removeErr error
		result, removeErr = a.rbacOperators.RemoveRule(ctx, client, request)
		return removeErr
	})
	return result, err
}

func (a *App) ListDesktopRBACOperators() ([]RBACOperatorView, error) {
	var views []RBACOperatorView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var listErr error
		views, listErr = a.rbacOperators.ListOperators(ctx, client)
		return listErr
	})
	return views, err
}

func (a *App) SetDesktopOperatorRoles(request OperatorRolesRequest) (RBACOperatorView, error) {
	var view RBACOperatorView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var updateErr error
		view, updateErr = a.rbacOperators.SetOperatorRoles(ctx, client, request)
		return updateErr
	})
	return view, err
}

func (a *App) StampDesktopOperatorCredential(request OperatorCredentialStampRequest) (OperatorCredentialStampResult, error) {
	var result OperatorCredentialStampResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var stampErr error
		result, stampErr = a.rbacOperators.StampCredential(ctx, client, request)
		return stampErr
	})
	return result, err
}

func (a *App) LoadProfiles() ([]ConnectionProfile, error) {
	return a.profiles.LoadProfiles()
}

func (a *App) SaveProfile(profile ConnectionProfile) error {
	return a.profiles.SaveProfile(profile)
}

func (a *App) ConnectProfile(profile ConnectionProfile) (ConnectionView, error) {
	a.enrollment.Clear()
	a.deploymentTokens.Clear()
	a.changeControl.Clear()
	a.applicationPackages.Clear()
	a.secretVersions.Clear()
	a.rbacOperators.Clear()
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

func (a *App) CreateEnrollmentToken(request EnrollmentTokenRequest) (EnrollmentTokenResult, error) {
	var result EnrollmentTokenResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var createErr error
		result, createErr = a.enrollment.CreateConnected(ctx, client, request)
		return createErr
	})
	if err != nil {
		a.enrollment.Clear()
		return EnrollmentTokenResult{}, err
	}
	return result, nil
}

func (a *App) CopyEnrollmentToken() error {
	return a.enrollment.Copy(a.applicationContext(), a.writeClipboard)
}

func (a *App) ClearEnrollmentToken() {
	a.enrollment.Clear()
}

func (a *App) ListDeploymentTokens() ([]DeploymentTokenView, error) {
	var views []DeploymentTokenView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		views, loadErr = a.deploymentTokens.ListConnected(ctx, client)
		return loadErr
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}

func (a *App) LoadDeploymentToken(label string) (DeploymentTokenView, error) {
	var view DeploymentTokenView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.deploymentTokens.LoadConnected(ctx, client, label)
		return loadErr
	})
	if err != nil {
		return DeploymentTokenView{}, err
	}
	return view, nil
}

func (a *App) CreateDeploymentToken(request DeploymentTokenCreateRequest) (DeploymentTokenCreateResult, error) {
	var result DeploymentTokenCreateResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var createErr error
		result, createErr = a.deploymentTokens.CreateConnected(ctx, client, request)
		return createErr
	})
	if err != nil {
		return DeploymentTokenCreateResult{}, err
	}
	return result, nil
}

func (a *App) CopyDeploymentToken() error {
	return a.deploymentTokens.Copy(a.applicationContext(), a.writeClipboard)
}

func (a *App) SaveDeploymentToken(label string) (DeploymentTokenSaveResult, error) {
	return a.deploymentTokens.Save(a.applicationContext(), label)
}

func (a *App) ClearDeploymentToken() {
	a.deploymentTokens.Clear()
}

func (a *App) RevokeDeploymentToken(request DeploymentTokenRevokeRequest) (DeploymentTokenView, error) {
	var view DeploymentTokenView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var revokeErr error
		view, revokeErr = a.deploymentTokens.RevokeConnected(ctx, client, request)
		return revokeErr
	})
	if err != nil {
		return DeploymentTokenView{}, err
	}
	return view, nil
}

func (a *App) SetEndpointLabel(request EndpointLabelSetRequest) (EndpointLabelResultView, error) {
	var result EndpointLabelResultView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var setErr error
		result, setErr = a.endpointLabels.SetConnected(ctx, client, request)
		return setErr
	})
	if err != nil {
		return EndpointLabelResultView{}, err
	}
	return result, nil
}

func (a *App) RemoveEndpointLabel(request EndpointLabelRemoveRequest) (EndpointLabelResultView, error) {
	var result EndpointLabelResultView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var removeErr error
		result, removeErr = a.endpointLabels.RemoveConnected(ctx, client, request)
		return removeErr
	})
	if err != nil {
		return EndpointLabelResultView{}, err
	}
	return result, nil
}

func (a *App) RequestEndpointAgentUpgrade(request EndpointUpgradeRequest) (EndpointUpgradeResult, error) {
	var result EndpointUpgradeResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var requestErr error
		result, requestErr = a.endpointUpgrade.RequestConnected(ctx, client, request)
		return requestErr
	})
	if err != nil {
		return EndpointUpgradeResult{}, err
	}
	return result, nil
}

func (a *App) RequestFleetAgentUpgrade(request FleetUpgradeRequest) (FleetUpgradeResult, error) {
	var result FleetUpgradeResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var requestErr error
		result, requestErr = a.fleetUpgrade.RequestConnected(ctx, client, request)
		return requestErr
	})
	if err != nil {
		return FleetUpgradeResult{}, err
	}
	return result, nil
}

func (a *App) GetDiagnosticCapabilities() DiagnosticCapabilities {
	return a.diagnostics.Capabilities()
}

func (a *App) RequestDiagnosticCollection(request DiagnosticCollectionRequest) (DiagnosticCollectionResult, error) {
	var result DiagnosticCollectionResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var requestErr error
		result, requestErr = a.diagnostics.RequestConnected(ctx, client, request)
		return requestErr
	})
	if err != nil {
		return DiagnosticCollectionResult{}, err
	}
	return result, nil
}

func (a *App) SaveDiagnosticBundle(requestID string) (DiagnosticBundleSaveResult, error) {
	var result DiagnosticBundleSaveResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var saveErr error
		result, saveErr = a.diagnosticBundles.SaveConnected(ctx, client, requestID)
		return saveErr
	})
	if err != nil {
		return DiagnosticBundleSaveResult{}, err
	}
	return result, nil
}

func (a *App) RemoveEndpoint(request EndpointRemovalRequest) (EndpointRemovalResult, error) {
	var result EndpointRemovalResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var removeErr error
		result, removeErr = a.endpointRemoval.RemoveConnected(ctx, client, request)
		return removeErr
	})
	if err != nil {
		return EndpointRemovalResult{}, err
	}
	return result, nil
}

func (a *App) LoadAssetInventory() (AssetInventoryView, error) {
	var view AssetInventoryView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.readExport.LoadAssetInventoryConnected(ctx, client)
		return loadErr
	})
	if err != nil {
		return AssetInventoryView{}, err
	}
	return view, nil
}

func (a *App) SaveAssetInventory(format string) (ReadExportSaveResult, error) {
	var result ReadExportSaveResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var saveErr error
		result, saveErr = a.readExport.SaveAssetInventoryConnected(ctx, client, format)
		return saveErr
	})
	if err != nil {
		return ReadExportSaveResult{}, err
	}
	return result, nil
}

func (a *App) LoadFleetOperationalReports(fleet string) (FleetOperationalReportsView, error) {
	var view FleetOperationalReportsView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.readExport.LoadFleetOperationalReportsConnected(ctx, client, fleet)
		return loadErr
	})
	if err != nil {
		return FleetOperationalReportsView{}, err
	}
	return view, nil
}

func (a *App) LoadFirewallReport(endpointID string) (FirewallReportView, error) {
	var view FirewallReportView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.readExport.LoadFirewallReportConnected(ctx, client, endpointID)
		return loadErr
	})
	if err != nil {
		return FirewallReportView{}, err
	}
	return view, nil
}

func (a *App) SaveFirewallReport(request FirewallExportRequest) (ReadExportSaveResult, error) {
	var result ReadExportSaveResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var saveErr error
		result, saveErr = a.readExport.SaveFirewallReportConnected(ctx, client, request)
		return saveErr
	})
	if err != nil {
		return ReadExportSaveResult{}, err
	}
	return result, nil
}

func (a *App) LoadAuditExportInfo() (AuditExportInfoView, error) {
	var view AuditExportInfoView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.readExport.LoadAuditExportInfoConnected(ctx, client)
		return loadErr
	})
	if err != nil {
		return AuditExportInfoView{}, err
	}
	return view, nil
}

func (a *App) LoadDiagnosticRequest(requestID string) (DiagnosticLifecycleView, error) {
	var view DiagnosticLifecycleView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		view, loadErr = a.readExport.LoadDiagnosticRequestConnected(ctx, client, requestID)
		return loadErr
	})
	if err != nil {
		return DiagnosticLifecycleView{}, err
	}
	return view, nil
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

func (a *App) LoadChangeRequestDetail(changeRequestID string) (ChangeRequestDetailView, error) {
	var detail ChangeRequestDetailView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		detail, loadErr = a.changeRequests.LoadDetailConnected(ctx, client, changeRequestID)
		return loadErr
	})
	if err != nil {
		return ChangeRequestDetailView{}, err
	}
	return detail, nil
}

func (a *App) AuthorizeChangeRequest(request ChangeAuthorizationRequest) (ChangeActionResult, error) {
	var result ChangeActionResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var actionErr error
		result, actionErr = a.changeControl.AuthorizeConnected(ctx, client, request)
		return actionErr
	})
	if err != nil {
		return ChangeActionResult{}, err
	}
	return result, nil
}

func (a *App) ChangeRequestLifecycle(request ChangeLifecycleRequest) (ChangeActionResult, error) {
	var result ChangeActionResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var actionErr error
		result, actionErr = a.changeControl.LifecycleConnected(ctx, client, request)
		return actionErr
	})
	if err != nil {
		return ChangeActionResult{}, err
	}
	return result, nil
}

func (a *App) PromoteChangeBaseline(request ChangeBaselinePromotionRequest) (ChangeActionResult, error) {
	var result ChangeActionResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var actionErr error
		result, actionErr = a.changeControl.PromoteBaselineConnected(ctx, client, request)
		return actionErr
	})
	if err != nil {
		return ChangeActionResult{}, err
	}
	return result, nil
}

func (a *App) ChooseBaselineAdoptionPlan(fleet string) (BaselineAdoptionPreview, error) {
	var preview BaselineAdoptionPreview
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, _ *admin.Client) error {
		var chooseErr error
		preview, chooseErr = a.changeControl.ChooseBaselineAdoptionPlan(ctx, fleet)
		return chooseErr
	})
	if err != nil {
		a.changeControl.Clear()
		return BaselineAdoptionPreview{}, err
	}
	return preview, nil
}

func (a *App) CreateBaselineAdoption(request BaselineAdoptionRequest) (ChangeActionResult, error) {
	var result ChangeActionResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var actionErr error
		result, actionErr = a.changeControl.CreateBaselineAdoptionConnected(ctx, client, request)
		return actionErr
	})
	if err != nil {
		return ChangeActionResult{}, err
	}
	return result, nil
}

func (a *App) LoadActivityPage(request ActivityPageRequest) (ActivityPageView, error) {
	var page ActivityPageView
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var loadErr error
		page, loadErr = a.activity.LoadPageConnected(ctx, client, request)
		return loadErr
	})
	if err != nil {
		return ActivityPageView{}, err
	}
	return page, nil
}

func (a *App) RequestGitSync() (GitSyncResult, error) {
	state := a.sessions.Snapshot()
	if state.Status != SessionConnected || state.Identity == nil {
		return GitSyncResult{}, ErrSessionNotConnected
	}

	var result GitSyncResult
	err := a.sessions.ExecuteAuthenticatedAction(a.applicationContext(), func(ctx context.Context, client *admin.Client) error {
		var requestErr error
		result, requestErr = a.gitSync.RequestConnected(ctx, client, state.ProfileName)
		return requestErr
	})
	if err != nil {
		return GitSyncResult{}, err
	}
	return result, nil
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
	a.enrollment.Clear()
	a.deploymentTokens.Clear()
	a.changeControl.Clear()
	a.applicationPackages.Clear()
	a.secretVersions.Clear()
	a.rbacOperators.Clear()
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
	a.enrollment.Clear()
	a.deploymentTokens.Clear()
	a.changeControl.Clear()
	a.applicationPackages.Clear()
	a.secretVersions.Clear()
	a.rbacOperators.Clear()
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
