import {
  createContext,
  type PropsWithChildren,
  useContext,
} from "react";

import {
  ActivateSecretVersion,
  AddDesktopRBACRule,
  AuthorizeChangeRequest,
  BootstrapProfile,
  BuildLocalPackage,
  ChangeRequestLifecycle,
  ChooseBaselineAdoptionPlan,
  ChooseAIProjectRoot,
  ChooseAppPackageArchive,
  ChooseConfigRepository,
  ChooseLocalPackageSource,
  CheckDesktopUpdate,
  ClearDeploymentToken,
  ClearEnrollmentToken,
  ConnectProfile,
  CopyDeploymentToken,
  CopyEnrollmentToken,
  CreateBaselineAdoption,
  CreateDeploymentToken,
  CreateDesktopRBACRole,
  CreateEnrollmentToken,
  CreateLocalPackage,
  DeleteAppPackage,
  DeleteDesktopRBACRole,
  DiscoverConfigFleet,
  GetApplicationInfo,
  GetDesktopRBACRole,
  GetDiagnosticCapabilities,
  ImportConfigHubSnippet,
  InitializeConfigRepository,
  ListDeploymentTokens,
  ListDesktopRBACOperators,
  ListDesktopRBACRoles,
  ListConfigHubSnippets,
  ListAppPackages,
  ListAIIntegrations,
  ListSecretVersions,
  LoadActivityPage,
  LoadAppPackage,
  LoadAssetInventory,
  LoadAuditExportInfo,
  LoadChangeRequestDetail,
  LoadDiagnosticRequest,
  LoadDeploymentToken,
  LoadFirewallReport,
  LoadFleetOperationalReports,
  LoadSetupMaintenance,
  LoadWorkspace,
  OpenRemotrDocumentation,
  PromoteChangeBaseline,
  PublishAppPackage,
  RemoveEndpoint,
  RemoveDesktopRBACRule,
  RemoveEndpointLabel,
  RequestEndpointAgentUpgrade,
  RequestDiagnosticCollection,
  RequestFleetAgentUpgrade,
  RequestGitSync,
  RenderConfigRepository,
  RevokeDeploymentToken,
  RevokeSecretVersion,
  RunDesktopDoctor,
  SaveAssetInventory,
  SaveConfigRender,
  SaveDiagnosticBundle,
  SaveDeploymentToken,
  SaveFirewallReport,
  SaveProfile,
  SetEndpointLabel,
  SetDesktopOperatorRoles,
  StampDesktopOperatorCredential,
  SetupAIIntegration,
  UploadSecretVersion,
  UpgradeAIIntegration,
  ValidateConfigRepository,
} from "../../wailsjs/go/main/App";
import type { ActionAcknowledgement } from "../actions/useActionController";
import type {
  AppPackageArchiveView,
  AppPackageDeleteRequest,
  AppPackageDeleteResult,
  AppPackagePublishRequest,
  AppPackageView,
  LocalPackageCreateRequest,
  LocalPackageView,
} from "../actions/applicationPackage";
import type {
  BaselineAdoptionPreview,
  BaselineAdoptionRequest,
  ChangeActionResult,
  ChangeAuthorizationRequest,
  ChangeBaselinePromotionRequest,
  ChangeLifecycleRequest,
} from "../changes/changeControl";
import type {
  ConfigFleetDiscoverRequest,
  ConfigFleetDiscoveryView,
  ConfigHubImportRequest,
  ConfigHubImportResult,
  ConfigHubSnippetView,
  ConfigRenderRequest,
  ConfigRenderSaveRequest,
  ConfigRenderSaveResult,
  ConfigRenderView,
  ConfigRepositoryInitRequest,
  ConfigRepositoryInitResult,
  ConfigValidationView,
  ConfigWorkingTreeView,
} from "../configuration/configRepository";
import type { ChangeRequestDetailView } from "../changes/ChangeRequestDetail";
import type {
  ActivityPageRequest,
  ActivityPageView,
} from "../activity/ActivityPage";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "../actions/enrollmentToken";
import type {
  EndpointLabelRemoveRequest,
  EndpointLabelResult,
  EndpointLabelSetRequest,
} from "../actions/endpointLabel";
import type {
  EndpointUpgradeRequest,
  EndpointUpgradeResult,
} from "../actions/endpointUpgrade";
import type {
  FleetUpgradeRequest,
  FleetUpgradeResult,
} from "../actions/fleetUpgrade";
import type {
  DiagnosticCapabilities,
  DiagnosticCollectionRequest,
  DiagnosticCollectionResult,
} from "../actions/diagnosticCollection";
import type { DiagnosticBundleSaveResult } from "../actions/diagnosticBundle";
import type {
  EndpointRemovalRequest,
  EndpointRemovalResult,
} from "../actions/endpointRemoval";
import type {
  DeploymentTokenCreateRequest,
  DeploymentTokenCreateResult,
  DeploymentTokenRevokeRequest,
  DeploymentTokenSaveResult,
  DeploymentTokenView,
} from "../actions/deploymentToken";
import type {
  SecretLifecycleRequest,
  SecretUploadRequest,
  SecretVersionView,
} from "../actions/secret";
import type {
  OperatorCredentialStampRequest,
  OperatorCredentialStampResult,
  OperatorRolesRequest,
  RBACMutationResult,
  RBACOperatorView,
  RBACRoleCreateRequest,
  RBACRoleDeleteRequest,
  RBACRoleView,
  RBACRuleAddRequest,
  RBACRuleRemoveRequest,
  RBACRuleView,
} from "../actions/rbacOperator";
import type {
  AssetInventoryView,
  AuditExportInfoView,
  DiagnosticLifecycleView,
  FirewallExportRequest,
  FirewallReportView,
  FleetOperationalReportsView,
  ReadExportSaveResult,
  ReportSectionResult,
} from "../reports/readExport";
import type {
  ConnectionProfile,
  ConnectionView,
  DesktopDoctorReport,
  DesktopUpdateStatus,
  SetupMaintenanceView,
} from "../setup/setupMaintenance";
import type {
  AIIntegrationActionResult,
  AIIntegrationInstallRequest,
  AIIntegrationListRequest,
  AIIntegrationUpgradeRequest,
  AIIntegrationView,
  AIProjectRootView,
} from "../setup/aiIntegration";
import type { OverviewWorkspace } from "../overview/Overview";

export interface ApplicationInfo {
  name: string;
  version: string;
}

export interface DesktopBridge {
  addRBACRule(request: RBACRuleAddRequest): Promise<RBACRuleView>;
  bootstrapProfile(
    profile: ConnectionProfile,
    token: string,
  ): Promise<ConnectionView>;
  activateSecretVersion(
    request: SecretLifecycleRequest,
  ): Promise<SecretVersionView>;
  buildLocalPackage(): Promise<AppPackageArchiveView>;
  checkDesktopUpdate(): Promise<DesktopUpdateStatus>;
  authorizeChangeRequest(
    request: ChangeAuthorizationRequest,
  ): Promise<ChangeActionResult>;
  changeRequestLifecycle(
    request: ChangeLifecycleRequest,
  ): Promise<ChangeActionResult>;
  chooseBaselineAdoptionPlan(fleet: string): Promise<BaselineAdoptionPreview>;
  chooseAIProjectRoot(): Promise<AIProjectRootView>;
  chooseAppPackageArchive(): Promise<AppPackageArchiveView>;
  chooseConfigRepository(): Promise<ConfigWorkingTreeView>;
  chooseLocalPackageSource(): Promise<LocalPackageView>;
  clearDeploymentToken(): Promise<void>;
  clearEnrollmentToken(): Promise<void>;
  connectProfile(profile: ConnectionProfile): Promise<ConnectionView>;
  copyDeploymentToken(): Promise<void>;
  copyEnrollmentToken(): Promise<void>;
  createEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<EnrollmentTokenResult>;
  createDeploymentToken(
    request: DeploymentTokenCreateRequest,
  ): Promise<DeploymentTokenCreateResult>;
  createLocalPackage(request: LocalPackageCreateRequest): Promise<LocalPackageView>;
  createRBACRole(request: RBACRoleCreateRequest): Promise<RBACRoleView>;
  createBaselineAdoption(
    request: BaselineAdoptionRequest,
  ): Promise<ChangeActionResult>;
  getApplicationInfo(): Promise<ApplicationInfo>;
  getDiagnosticCapabilities(): Promise<DiagnosticCapabilities>;
  deleteAppPackage(request: AppPackageDeleteRequest): Promise<AppPackageDeleteResult>;
  deleteRBACRole(request: RBACRoleDeleteRequest): Promise<RBACMutationResult>;
  discoverConfigFleet(
    request: ConfigFleetDiscoverRequest,
  ): Promise<ConfigFleetDiscoveryView>;
  getRBACRole(name: string): Promise<RBACRoleView>;
  importConfigHubSnippet(
    request: ConfigHubImportRequest,
  ): Promise<ConfigHubImportResult>;
  initializeConfigRepository(
    request: ConfigRepositoryInitRequest,
  ): Promise<ConfigRepositoryInitResult>;
  listAppPackages(prefix: string): Promise<AppPackageView[]>;
  listAIIntegrations(request: AIIntegrationListRequest): Promise<AIIntegrationView[]>;
  listSecretVersions(name: string): Promise<SecretVersionView[]>;
  listDeploymentTokens(): Promise<DeploymentTokenView[]>;
  listRBACOperators(): Promise<RBACOperatorView[]>;
  listRBACRoles(): Promise<RBACRoleView[]>;
  listConfigHubSnippets(workingTreeId: string): Promise<ConfigHubSnippetView[]>;
  loadAssetInventory(): Promise<AssetInventoryView>;
  loadAuditExportInfo(): Promise<AuditExportInfoView>;
  loadActivityPage(request: ActivityPageRequest): Promise<ActivityPageView>;
  loadChangeRequestDetail(
    changeRequestId: string,
  ): Promise<ChangeRequestDetailView>;
  loadAppPackage(name: string, version: string): Promise<AppPackageView>;
  loadDeploymentToken(label: string): Promise<DeploymentTokenView>;
  loadDiagnosticRequest(requestId: string): Promise<DiagnosticLifecycleView>;
  loadFirewallReport(endpointId: string): Promise<FirewallReportView>;
  loadFleetOperationalReports(
    fleet: string,
  ): Promise<FleetOperationalReportsView>;
  loadSetupMaintenance(): Promise<SetupMaintenanceView>;
  loadWorkspace(): Promise<OverviewWorkspace>;
  openRemotrDocumentation(): Promise<void>;
  removeEndpointLabel(
    request: EndpointLabelRemoveRequest,
  ): Promise<EndpointLabelResult>;
  promoteChangeBaseline(
    request: ChangeBaselinePromotionRequest,
  ): Promise<ChangeActionResult>;
  removeEndpoint(request: EndpointRemovalRequest): Promise<EndpointRemovalResult>;
  removeRBACRule(request: RBACRuleRemoveRequest): Promise<RBACMutationResult>;
  requestEndpointAgentUpgrade(
    request: EndpointUpgradeRequest,
  ): Promise<EndpointUpgradeResult>;
  requestDiagnosticCollection(
    request: DiagnosticCollectionRequest,
  ): Promise<DiagnosticCollectionResult>;
  requestFleetAgentUpgrade(
    request: FleetUpgradeRequest,
  ): Promise<FleetUpgradeResult>;
  requestGitSync(): Promise<ActionAcknowledgement>;
  renderConfigRepository(request: ConfigRenderRequest): Promise<ConfigRenderView>;
  runDesktopDoctor(profile: ConnectionProfile): Promise<DesktopDoctorReport>;
  publishAppPackage(request: AppPackagePublishRequest): Promise<AppPackageView>;
  revokeDeploymentToken(
    request: DeploymentTokenRevokeRequest,
  ): Promise<DeploymentTokenView>;
  revokeSecretVersion(
    request: SecretLifecycleRequest,
  ): Promise<SecretVersionView>;
  saveAssetInventory(format: "csv" | "json"): Promise<ReadExportSaveResult>;
  saveConfigRender(
    request: ConfigRenderSaveRequest,
  ): Promise<ConfigRenderSaveResult>;
  saveDiagnosticBundle(requestId: string): Promise<DiagnosticBundleSaveResult>;
  saveDeploymentToken(label: string): Promise<DeploymentTokenSaveResult>;
  saveFirewallReport(
    request: FirewallExportRequest,
  ): Promise<ReadExportSaveResult>;
  saveProfile(profile: ConnectionProfile): Promise<void>;
  setEndpointLabel(
    request: EndpointLabelSetRequest,
  ): Promise<EndpointLabelResult>;
  setOperatorRoles(request: OperatorRolesRequest): Promise<RBACOperatorView>;
  stampOperatorCredential(
    request: OperatorCredentialStampRequest,
  ): Promise<OperatorCredentialStampResult>;
  setupAIIntegration(
    request: AIIntegrationInstallRequest,
  ): Promise<AIIntegrationActionResult>;
  uploadSecretVersion(request: SecretUploadRequest): Promise<SecretVersionView>;
  upgradeAIIntegration(
    request: AIIntegrationUpgradeRequest,
  ): Promise<AIIntegrationActionResult>;
  validateConfigRepository(workingTreeId: string): Promise<ConfigValidationView>;
}

interface GeneratedGitSyncResult {
  acceptedAt: string;
  action: string;
  affectedEvidence: string[];
  summary: string;
  target: string;
}

interface GeneratedEnrollmentTokenResult {
  expiresAt: string;
  fleet: string;
  token: string;
}

interface GeneratedEndpointLabelResult {
  effect: string;
  endpointId: string;
  key: string;
  value: string;
  labels: Array<{ key: string; value: string }>;
}

interface GeneratedEndpointUpgradeResult {
  affectedEvidence: string[];
  endpointId: string;
  status: string;
  version: string;
}

interface GeneratedFleetUpgradeResult {
  acceptedEndpoints: number;
  fleet: string;
  status: string;
  version: string;
}

interface GeneratedDiagnosticCapabilities {
  collectors: string[];
  maxTimeSpanSeconds: number;
}

interface GeneratedDiagnosticCollectionResult {
  collectors: string[];
  createdAt?: string;
  endpointId: string;
  expiresAt?: string;
  requestId: string;
  since: string;
  status: string;
  until: string;
}

interface GeneratedDiagnosticBundleSaveResult {
  path?: string;
  sizeBytes?: number;
  status: string;
}

interface GeneratedReadExportSaveResult {
  path?: string;
  sizeBytes?: number;
  status: string;
}

interface GeneratedDeploymentTokenSaveResult {
  path?: string;
  sizeBytes?: number;
  status: string;
}

interface GeneratedDeploymentTokenView {
  createdAt: string;
  expiresAt: string;
  fleet: string;
  id: string;
  label: string;
  lastUsedAt?: string;
  revokedAt?: string;
  status: string;
}

interface GeneratedDeploymentTokenCreateResult {
  metadata: GeneratedDeploymentTokenView;
  token: string;
}

interface GeneratedEndpointRemovalResult {
  affectedEvidence: string[];
  credentialStatus: string;
  endpointId: string;
  status: string;
}

interface GeneratedAppPackageArchiveView
  extends Omit<AppPackageArchiveView, "source"> {
  source: string;
}

interface GeneratedAppPackageDeleteResult
  extends Omit<AppPackageDeleteResult, "scope"> {
  scope: string;
}

function adaptAppPackageArchive(
  archive: GeneratedAppPackageArchiveView,
): AppPackageArchiveView {
  if (archive.source !== "built" && archive.source !== "selected") {
    throw new Error("The native bridge returned an unknown package archive source.");
  }
  return { ...archive, source: archive.source };
}

function adaptAppPackageDeleteResult(
  result: GeneratedAppPackageDeleteResult,
): AppPackageDeleteResult {
  if (result.scope !== "catalog_and_object" && result.scope !== "catalog_only") {
    throw new Error("The native bridge returned an unknown package deletion scope.");
  }
  return { ...result, scope: result.scope };
}

function adaptEndpointLabelEffect(
  effect: string,
): EndpointLabelResult["effect"] {
  if (effect === "added" || effect === "removed" || effect === "replaced") {
    return effect;
  }
  throw new Error("The native bridge returned an unknown Label effect.");
}

type GeneratedSecretVersionView = Awaited<
  ReturnType<typeof ActivateSecretVersion>
>;

function adaptSecretVersion(result: GeneratedSecretVersionView): SecretVersionView {
  if (result.scopeType !== "fleet" && result.scopeType !== "endpoint") {
    throw new Error("The native bridge returned an unknown Secret scope.");
  }
  if (
    result.status !== "inactive" &&
    result.status !== "active" &&
    result.status !== "activation_planned" &&
    result.status !== "revoked"
  ) {
    throw new Error("The native bridge returned an unknown Secret lifecycle state.");
  }
  return {
    activatedAt: result.activatedAt,
    activatedBy: result.activatedBy,
    activationGeneration: result.activationGeneration,
    createdAt: result.createdAt,
    createdBy: result.createdBy,
    endpointCopyStatus: result.endpointCopyStatus,
    fingerprint: result.fingerprint,
    name: result.name,
    resolutionBlocked: result.resolutionBlocked,
    revokedAt: result.revokedAt,
    revokedBy: result.revokedBy,
    rollouts: result.rollouts.map((rollout) => ({
      changeRequestId: rollout.changeRequestId,
      effectiveHash: rollout.effectiveHash,
      fleet: rollout.fleet,
      purpose: rollout.purpose,
      resourceAddress: rollout.resourceAddress,
      risk: rollout.risk,
    })),
    scopeId: result.scopeId,
    scopeType: result.scopeType,
    status: result.status,
    version: result.version,
  };
}

type GeneratedRBACRoleView = Awaited<ReturnType<typeof GetDesktopRBACRole>>;
type GeneratedRBACOperatorView = Awaited<
  ReturnType<typeof SetDesktopOperatorRoles>
>;
type GeneratedRBACRuleView = Awaited<ReturnType<typeof AddDesktopRBACRule>>;
type GeneratedRBACMutationResult = Awaited<
  ReturnType<typeof DeleteDesktopRBACRole>
>;
type GeneratedCredentialStampResult = Awaited<
  ReturnType<typeof StampDesktopOperatorCredential>
>;
type GeneratedConnectionView = Awaited<ReturnType<typeof ConnectProfile>>;
type GeneratedDoctorReport = Awaited<ReturnType<typeof RunDesktopDoctor>>;
type GeneratedSetupMaintenanceView = Awaited<
  ReturnType<typeof LoadSetupMaintenance>
>;
type GeneratedWorkspaceView = Awaited<ReturnType<typeof LoadWorkspace>>;
type GeneratedUpdateStatus = Awaited<ReturnType<typeof CheckDesktopUpdate>>;

function cloneConnectionProfile(profile: ConnectionProfile): ConnectionProfile {
  return { ...profile };
}

function adaptConnectionView(result: GeneratedConnectionView): ConnectionView {
  return {
    operatorId: result.operatorId,
    profileName: result.profileName,
    roles: [...result.roles],
    serverUrl: result.serverUrl,
  };
}

function adaptSetupMaintenance(
  result: GeneratedSetupMaintenanceView,
): SetupMaintenanceView {
  return {
    application: { ...result.application },
    desktopProfilesPath: result.desktopProfilesPath,
    profiles: result.profiles.map(cloneConnectionProfile),
    standardConfigPath: result.standardConfigPath,
  };
}

function adaptWorkspace(result: GeneratedWorkspaceView): OverviewWorkspace {
  const cloneSection = (
    section: GeneratedWorkspaceView["sections"]["activity"],
  ): OverviewWorkspace["sections"]["activity"] => ({
    ...(section.error ? { error: { ...section.error } } : {}),
    snapshot: { ...section.snapshot },
    state: section.state,
  });
  return {
    activity: result.activity.map((event) => ({
      ...event,
      details: event.details.map((detail) => ({ ...detail })),
    })),
    activityNextCursor: result.activityNextCursor,
    changeRequests: result.changeRequests.map((request) => ({ ...request })),
    endpoints: result.endpoints.map((endpoint) => ({
      ...endpoint,
      labels: endpoint.labels.map((label) => ({ ...label })),
      usernames: [...endpoint.usernames],
    })),
    fleets: result.fleets.map((fleet) => ({
      ...fleet,
      agentVersions: fleet.agentVersions.map((status) => ({ ...status })),
      compliance: fleet.compliance.map((status) => ({ ...status })),
      freshness: fleet.freshness.map((status) => ({ ...status })),
    })),
    sections: {
      activity: cloneSection(result.sections.activity),
      changeRequests: cloneSection(result.sections.changeRequests),
      endpoints: cloneSection(result.sections.endpoints),
      fleets: cloneSection(result.sections.fleets),
      state: cloneSection(result.sections.state),
    },
  };
}

function adaptDoctorReport(result: GeneratedDoctorReport): DesktopDoctorReport {
  return {
    checks: result.checks.map((check) => {
      if (check.status !== "ok" && check.status !== "warn" && check.status !== "fail") {
        throw new Error("The native bridge returned an unknown doctor status.");
      }
      return { ...check, status: check.status };
    }),
    healthy: result.healthy,
    operatorId: result.operatorId,
    profileName: result.profileName,
    roles: [...result.roles],
  };
}

function adaptUpdateStatus(result: GeneratedUpdateStatus): DesktopUpdateStatus {
  if (result.installSupported) {
    throw new Error(
      "This build cannot advertise in-app installation without native artifact evidence.",
    );
  }
  return { ...result, installSupported: false };
}

function adaptRBACRule(result: GeneratedRBACRuleView): RBACRuleView {
  return {
    id: result.id,
    method: result.method,
    pathPattern: result.pathPattern,
    roleName: result.roleName,
  };
}

function adaptRBACRole(result: GeneratedRBACRoleView): RBACRoleView {
  return {
    builtIn: result.builtIn,
    description: result.description,
    name: result.name,
    rules: result.rules.map(adaptRBACRule),
  };
}

function adaptRBACOperator(result: GeneratedRBACOperatorView): RBACOperatorView {
  return {
    certFingerprint: result.certFingerprint,
    createdAt: result.createdAt,
    id: result.id,
    roles: [...result.roles],
  };
}

function adaptRBACMutation(
  result: GeneratedRBACMutationResult,
): RBACMutationResult {
  if (result.status !== "deleted" && result.status !== "removed") {
    throw new Error("The native bridge returned an unknown RBAC mutation state.");
  }
  return {
    name: result.name,
    ruleId: result.ruleId,
    status: result.status,
  };
}

function adaptCredentialStamp(
  result: GeneratedCredentialStampResult,
): OperatorCredentialStampResult {
  if (result.status !== "saved") {
    throw new Error("The native bridge returned an unknown credential output state.");
  }
  return {
    directoryName: result.directoryName,
    label: result.label,
    operatorId: result.operatorId,
    roles: [...result.roles],
    status: "saved",
  };
}

export interface GeneratedBindings {
  AddDesktopRBACRule?(
    request: RBACRuleAddRequest,
  ): Promise<GeneratedRBACRuleView>;
  BootstrapProfile?(
    profile: ConnectionProfile,
    token: string,
  ): Promise<GeneratedConnectionView>;
  ActivateSecretVersion?(
    request: SecretLifecycleRequest,
  ): Promise<GeneratedSecretVersionView>;
  AuthorizeChangeRequest?(
    request: ChangeAuthorizationRequest,
  ): Promise<ChangeActionResult>;
  BuildLocalPackage?(): Promise<GeneratedAppPackageArchiveView>;
  CheckDesktopUpdate?(): Promise<GeneratedUpdateStatus>;
  ChangeRequestLifecycle?(
    request: ChangeLifecycleRequest,
  ): Promise<ChangeActionResult>;
  ChooseBaselineAdoptionPlan?(fleet: string): Promise<BaselineAdoptionPreview>;
  ChooseAIProjectRoot?(): ReturnType<typeof ChooseAIProjectRoot>;
  ChooseAppPackageArchive?(): Promise<GeneratedAppPackageArchiveView>;
  ChooseConfigRepository?(): ReturnType<typeof ChooseConfigRepository>;
  ChooseLocalPackageSource?(): Promise<LocalPackageView>;
  ClearDeploymentToken(): Promise<void>;
  ClearEnrollmentToken(): Promise<void>;
  ConnectProfile?(profile: ConnectionProfile): Promise<GeneratedConnectionView>;
  CopyDeploymentToken(): Promise<void>;
  CopyEnrollmentToken(): Promise<void>;
  CreateEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<GeneratedEnrollmentTokenResult>;
  CreateDeploymentToken(
    request: DeploymentTokenCreateRequest,
  ): Promise<GeneratedDeploymentTokenCreateResult>;
  CreateLocalPackage?(request: LocalPackageCreateRequest): Promise<LocalPackageView>;
  CreateDesktopRBACRole?(
    request: RBACRoleCreateRequest,
  ): Promise<GeneratedRBACRoleView>;
  DeleteAppPackage?(request: AppPackageDeleteRequest): Promise<GeneratedAppPackageDeleteResult>;
  DeleteDesktopRBACRole?(
    request: RBACRoleDeleteRequest,
  ): Promise<GeneratedRBACMutationResult>;
  DiscoverConfigFleet?(
    request: ConfigFleetDiscoverRequest,
  ): ReturnType<typeof DiscoverConfigFleet>;
  CreateBaselineAdoption?(
    request: BaselineAdoptionRequest,
  ): Promise<ChangeActionResult>;
  GetApplicationInfo(): Promise<ApplicationInfo>;
  GetDesktopRBACRole?(name: string): Promise<GeneratedRBACRoleView>;
  GetDiagnosticCapabilities(): Promise<GeneratedDiagnosticCapabilities>;
  ImportConfigHubSnippet?(
    request: ConfigHubImportRequest,
  ): ReturnType<typeof ImportConfigHubSnippet>;
  InitializeConfigRepository?(
    request: ConfigRepositoryInitRequest,
  ): ReturnType<typeof InitializeConfigRepository>;
  ListAppPackages?(prefix: string): Promise<AppPackageView[]>;
  ListAIIntegrations?(
    request: AIIntegrationListRequest,
  ): ReturnType<typeof ListAIIntegrations>;
  ListSecretVersions?(name: string): Promise<GeneratedSecretVersionView[]>;
  ListDeploymentTokens(): Promise<GeneratedDeploymentTokenView[]>;
  ListDesktopRBACOperators?(): Promise<GeneratedRBACOperatorView[]>;
  ListDesktopRBACRoles?(): Promise<GeneratedRBACRoleView[]>;
  ListConfigHubSnippets?(
    workingTreeId: string,
  ): ReturnType<typeof ListConfigHubSnippets>;
  LoadAssetInventory(): Promise<AssetInventoryView>;
  LoadActivityPage?(
    request: ActivityPageRequest,
  ): Promise<ActivityPageView>;
  LoadAppPackage?(name: string, version: string): Promise<AppPackageView>;
  LoadAuditExportInfo(): Promise<AuditExportInfoView>;
  LoadChangeRequestDetail?(
    changeRequestId: string,
  ): Promise<ChangeRequestDetailView>;
  LoadDiagnosticRequest(requestId: string): Promise<DiagnosticLifecycleView>;
  LoadDeploymentToken(label: string): Promise<GeneratedDeploymentTokenView>;
  LoadFirewallReport(endpointId: string): Promise<FirewallReportView>;
  LoadFleetOperationalReports(
    fleet: string,
  ): Promise<FleetOperationalReportsView>;
  LoadSetupMaintenance?(): Promise<GeneratedSetupMaintenanceView>;
  LoadWorkspace?(): Promise<GeneratedWorkspaceView>;
  OpenRemotrDocumentation?(): Promise<void>;
  RemoveEndpoint(
    request: EndpointRemovalRequest,
  ): Promise<GeneratedEndpointRemovalResult>;
  RemoveDesktopRBACRule?(
    request: RBACRuleRemoveRequest,
  ): Promise<GeneratedRBACMutationResult>;
  RemoveEndpointLabel(
    request: EndpointLabelRemoveRequest,
  ): Promise<GeneratedEndpointLabelResult>;
  PromoteChangeBaseline?(
    request: ChangeBaselinePromotionRequest,
  ): Promise<ChangeActionResult>;
  PublishAppPackage?(request: AppPackagePublishRequest): Promise<AppPackageView>;
  RequestEndpointAgentUpgrade(
    request: EndpointUpgradeRequest,
  ): Promise<GeneratedEndpointUpgradeResult>;
  RequestDiagnosticCollection(
    request: DiagnosticCollectionRequest,
  ): Promise<GeneratedDiagnosticCollectionResult>;
  RequestFleetAgentUpgrade(
    request: FleetUpgradeRequest,
  ): Promise<GeneratedFleetUpgradeResult>;
  RequestGitSync(): Promise<GeneratedGitSyncResult>;
  RenderConfigRepository?(
    request: ConfigRenderRequest,
  ): ReturnType<typeof RenderConfigRepository>;
  RevokeDeploymentToken(
    request: DeploymentTokenRevokeRequest,
  ): Promise<GeneratedDeploymentTokenView>;
  RevokeSecretVersion?(
    request: SecretLifecycleRequest,
  ): Promise<GeneratedSecretVersionView>;
  RunDesktopDoctor?(
    profile: ConnectionProfile,
  ): Promise<GeneratedDoctorReport>;
  SaveAssetInventory(format: string): Promise<GeneratedReadExportSaveResult>;
  SaveConfigRender?(
    request: ConfigRenderSaveRequest,
  ): ReturnType<typeof SaveConfigRender>;
  SaveDiagnosticBundle(
    requestId: string,
  ): Promise<GeneratedDiagnosticBundleSaveResult>;
  SaveDeploymentToken(
    label: string,
  ): Promise<GeneratedDeploymentTokenSaveResult>;
  SaveFirewallReport(
    request: FirewallExportRequest,
  ): Promise<GeneratedReadExportSaveResult>;
  SaveProfile?(profile: ConnectionProfile): Promise<void>;
  SetEndpointLabel(
    request: EndpointLabelSetRequest,
  ): Promise<GeneratedEndpointLabelResult>;
  SetDesktopOperatorRoles?(
    request: OperatorRolesRequest,
  ): Promise<GeneratedRBACOperatorView>;
  StampDesktopOperatorCredential?(
    request: OperatorCredentialStampRequest,
  ): Promise<GeneratedCredentialStampResult>;
  SetupAIIntegration?(
    request: AIIntegrationInstallRequest,
  ): ReturnType<typeof SetupAIIntegration>;
  UploadSecretVersion?(
    request: SecretUploadRequest,
  ): Promise<GeneratedSecretVersionView>;
  UpgradeAIIntegration?(
    request: AIIntegrationUpgradeRequest,
  ): ReturnType<typeof UpgradeAIIntegration>;
  ValidateConfigRepository?(
    workingTreeId: string,
  ): ReturnType<typeof ValidateConfigRepository>;
}

const generatedBindings: GeneratedBindings = {
  ActivateSecretVersion,
  AddDesktopRBACRule,
  AuthorizeChangeRequest,
  BootstrapProfile,
  BuildLocalPackage,
  CheckDesktopUpdate,
  ChangeRequestLifecycle,
  ChooseBaselineAdoptionPlan,
  ChooseAIProjectRoot,
  ChooseAppPackageArchive,
  ChooseConfigRepository,
  ChooseLocalPackageSource,
  ClearDeploymentToken,
  ClearEnrollmentToken,
  ConnectProfile,
  CopyDeploymentToken,
  CopyEnrollmentToken,
  CreateDeploymentToken,
  CreateDesktopRBACRole,
  CreateBaselineAdoption,
  CreateEnrollmentToken,
  CreateLocalPackage,
  DeleteAppPackage,
  DeleteDesktopRBACRole,
  DiscoverConfigFleet,
  GetApplicationInfo,
  GetDesktopRBACRole,
  GetDiagnosticCapabilities,
  ImportConfigHubSnippet,
  InitializeConfigRepository,
  ListAppPackages,
  ListAIIntegrations,
  ListSecretVersions,
  ListDeploymentTokens,
  ListDesktopRBACOperators,
  ListDesktopRBACRoles,
  ListConfigHubSnippets,
  LoadActivityPage,
  LoadAppPackage,
  LoadAssetInventory,
  LoadAuditExportInfo,
  LoadChangeRequestDetail,
  LoadDiagnosticRequest,
  LoadDeploymentToken,
  LoadFirewallReport,
  LoadFleetOperationalReports,
  LoadSetupMaintenance,
  LoadWorkspace,
  OpenRemotrDocumentation,
  PromoteChangeBaseline,
  PublishAppPackage,
  RemoveEndpoint,
  RemoveDesktopRBACRule,
  RemoveEndpointLabel,
  RequestEndpointAgentUpgrade,
  RequestDiagnosticCollection,
  RequestFleetAgentUpgrade,
  RequestGitSync,
  RenderConfigRepository,
  RevokeDeploymentToken,
  RevokeSecretVersion,
  RunDesktopDoctor,
  SaveAssetInventory,
  SaveConfigRender,
  SaveDiagnosticBundle,
  SaveDeploymentToken,
  SaveFirewallReport,
  SaveProfile,
  SetEndpointLabel,
  SetDesktopOperatorRoles,
  StampDesktopOperatorCredential,
  SetupAIIntegration,
  UploadSecretVersion,
  UpgradeAIIntegration,
  ValidateConfigRepository,
};

function adaptEndpointLabelResult(
  result: GeneratedEndpointLabelResult,
): EndpointLabelResult {
  return {
    effect: adaptEndpointLabelEffect(result.effect),
    endpointId: result.endpointId,
    key: result.key,
    labels: result.labels.map((label) => ({ ...label })),
    value: result.value,
  };
}

function cloneReportSection(section: ReportSectionResult): ReportSectionResult {
  return {
    ...(section.error ? { error: { ...section.error } } : {}),
    snapshot: { ...section.snapshot },
    state: section.state,
  };
}

function adaptReadExportSaveResult(
  result: GeneratedReadExportSaveResult,
): ReadExportSaveResult {
  if (result.status !== "saved" && result.status !== "canceled") {
    throw new Error("The native bridge returned an unknown export save state.");
  }
  return {
    ...(result.path ? { path: result.path } : {}),
    ...(typeof result.sizeBytes === "number"
      ? { sizeBytes: result.sizeBytes }
      : {}),
    status: result.status,
  };
}

function cloneDeploymentToken(
  token: GeneratedDeploymentTokenView,
): DeploymentTokenView {
  return {
    ...token,
    lastUsedAt: token.lastUsedAt ?? "",
    revokedAt: token.revokedAt ?? "",
  };
}

function adaptDeploymentTokenSaveResult(
  result: GeneratedDeploymentTokenSaveResult,
): DeploymentTokenSaveResult {
  if (result.status !== "saved" && result.status !== "canceled") {
    throw new Error(
      "The native bridge returned an unknown deployment token save state.",
    );
  }
  return {
    ...(result.path ? { path: result.path } : {}),
    ...(typeof result.sizeBytes === "number"
      ? { sizeBytes: result.sizeBytes }
      : {}),
    status: result.status,
  };
}

function cloneChangeRequestDetail(
  detail: ChangeRequestDetailView,
): ChangeRequestDetailView {
  return {
    ...detail,
    approvals: detail.approvals.map((approval) => ({ ...approval })),
    history: detail.history.map((entry) => ({ ...entry })),
    outcomes: detail.outcomes.map((outcome) => ({ ...outcome })),
    resources: detail.resources.map((resource) => ({
      ...resource,
      activationTargets: [...resource.activationTargets],
      dependsOn: [...resource.dependsOn],
      predictedEffects: [...resource.predictedEffects],
    })),
    summary: { ...detail.summary },
    targets: detail.targets.map((target) => ({ ...target })),
  };
}

function cloneChangeActionResult(
  result: ChangeActionResult,
): ChangeActionResult {
  return {
    ...result,
    affectedEvidence: [...result.affectedEvidence],
    ...(result.authorization
      ? {
          authorization: {
            ...result.authorization,
            executionWindows: result.authorization.executionWindows.map(
              (window) => ({ ...window, weekdays: [...window.weekdays] }),
            ),
          },
        }
      : {}),
    ...(result.baseline ? { baseline: { ...result.baseline } } : {}),
    changeRequest: cloneChangeRequestDetail(result.changeRequest),
  };
}

function unavailableBinding(name: string): never {
  throw new Error(`The native ${name} binding is unavailable.`);
}

function adaptEndpointUpgradeResult(
  result: GeneratedEndpointUpgradeResult,
): EndpointUpgradeResult {
  if (result.status !== "requested") {
    throw new Error("The native bridge returned an unknown upgrade state.");
  }
  return {
    affectedEvidence: [...result.affectedEvidence],
    endpointId: result.endpointId,
    status: "requested",
    version: result.version,
  };
}

function adaptFleetUpgradeResult(
  result: GeneratedFleetUpgradeResult,
): FleetUpgradeResult {
  if (result.status !== "requested") {
    throw new Error(
      "The native bridge returned an unknown Fleet upgrade state.",
    );
  }
  return {
    acceptedEndpoints: result.acceptedEndpoints,
    fleet: result.fleet,
    status: "requested",
    version: result.version,
  };
}

export function createWailsBridge(
  bindings: GeneratedBindings = generatedBindings,
): DesktopBridge {
  return {
    async activateSecretVersion(request) {
      const binding = bindings.ActivateSecretVersion;
      if (!binding) unavailableBinding("Secret activation");
      return adaptSecretVersion(await binding({ ...request }));
    },
    async bootstrapProfile(profile, token) {
      const binding = bindings.BootstrapProfile;
      if (!binding) unavailableBinding("Operator bootstrap");
      return adaptConnectionView(await binding(cloneConnectionProfile(profile), token));
    },
    async addRBACRule(request) {
      const binding = bindings.AddDesktopRBACRule;
      if (!binding) unavailableBinding("RBAC rule creation");
      return adaptRBACRule(await binding({ ...request }));
    },
    async buildLocalPackage() {
      const binding = bindings.BuildLocalPackage;
      if (!binding) unavailableBinding("local package build");
      return adaptAppPackageArchive(await binding());
    },
    async checkDesktopUpdate() {
      const binding = bindings.CheckDesktopUpdate;
      if (!binding) unavailableBinding("desktop update check");
      return adaptUpdateStatus(await binding());
    },
    async authorizeChangeRequest(request) {
      const binding = bindings.AuthorizeChangeRequest;
      if (!binding) unavailableBinding("Change authorization");
      return cloneChangeActionResult(await binding({
        ...request,
        executionWindows: request.executionWindows.map((window) => ({
          ...window,
          weekdays: [...window.weekdays],
        })),
      }));
    },
    async changeRequestLifecycle(request) {
      const binding = bindings.ChangeRequestLifecycle;
      if (!binding) unavailableBinding("Change lifecycle");
      return cloneChangeActionResult(await binding({ ...request }));
    },
    async chooseBaselineAdoptionPlan(fleet) {
      const binding = bindings.ChooseBaselineAdoptionPlan;
      if (!binding) unavailableBinding("baseline adoption preparation");
      const preview = await binding(fleet);
      return {
        ...preview,
        resourceAddresses: [...preview.resourceAddresses],
      };
    },
    async chooseAIProjectRoot() {
      const binding = bindings.ChooseAIProjectRoot;
      if (!binding) unavailableBinding("AI project selection");
      return { ...(await binding()) };
    },
    async chooseAppPackageArchive() {
      const binding = bindings.ChooseAppPackageArchive;
      if (!binding) unavailableBinding("application package archive");
      return adaptAppPackageArchive(await binding());
    },
    async chooseConfigRepository() {
      const binding = bindings.ChooseConfigRepository;
      if (!binding) unavailableBinding("Configuration repository selection");
      return { ...(await binding()) };
    },
    async chooseLocalPackageSource() {
      const binding = bindings.ChooseLocalPackageSource;
      if (!binding) unavailableBinding("local package source");
      return { ...(await binding()) };
    },
    async clearDeploymentToken() {
      await bindings.ClearDeploymentToken();
    },
    async clearEnrollmentToken() {
      await bindings.ClearEnrollmentToken();
    },
    async connectProfile(profile) {
      const binding = bindings.ConnectProfile;
      if (!binding) unavailableBinding("profile connection");
      return adaptConnectionView(await binding(cloneConnectionProfile(profile)));
    },
    async copyDeploymentToken() {
      await bindings.CopyDeploymentToken();
    },
    async copyEnrollmentToken() {
      await bindings.CopyEnrollmentToken();
    },
    async createEnrollmentToken(request) {
      const result = await bindings.CreateEnrollmentToken({ ...request });

      return {
        expiresAt: result.expiresAt,
        fleet: result.fleet,
        token: result.token,
      };
    },
    async createDeploymentToken(request) {
      const result = await bindings.CreateDeploymentToken({ ...request });
      return {
        metadata: cloneDeploymentToken(result.metadata),
        token: result.token,
      };
    },
    async createLocalPackage(request) {
      const binding = bindings.CreateLocalPackage;
      if (!binding) unavailableBinding("local package creation");
      return { ...(await binding({ ...request })) };
    },
    async createRBACRole(request) {
      const binding = bindings.CreateDesktopRBACRole;
      if (!binding) unavailableBinding("RBAC role creation");
      return adaptRBACRole(await binding({ ...request }));
    },
    async createBaselineAdoption(request) {
      const binding = bindings.CreateBaselineAdoption;
      if (!binding) unavailableBinding("baseline adoption");
      return cloneChangeActionResult(await binding({ ...request }));
    },
    async getApplicationInfo() {
      const info = await bindings.GetApplicationInfo();

      return { name: info.name, version: info.version };
    },
    async getDiagnosticCapabilities() {
      const capabilities = await bindings.GetDiagnosticCapabilities();
      return {
        collectors: [...capabilities.collectors],
        maxTimeSpanSeconds: capabilities.maxTimeSpanSeconds,
      };
    },
    async deleteAppPackage(request) {
      const binding = bindings.DeleteAppPackage;
      if (!binding) unavailableBinding("application package deletion");
      return adaptAppPackageDeleteResult(await binding({ ...request }));
    },
    async deleteRBACRole(request) {
      const binding = bindings.DeleteDesktopRBACRole;
      if (!binding) unavailableBinding("RBAC role deletion");
      return adaptRBACMutation(await binding({ ...request }));
    },
    async discoverConfigFleet(request) {
      const binding = bindings.DiscoverConfigFleet;
      if (!binding) unavailableBinding("Configuration Fleet discovery");
      const result = await binding({ ...request });
      return {
        ...result,
        applications: [...result.applications],
        capabilityRequirements: [...result.capabilityRequirements],
        crons: [...result.crons],
        diagnostics: result.diagnostics.map((diagnostic) => ({ ...diagnostic })),
        modules: [...result.modules],
        resourceKinds: [...result.resourceKinds],
      };
    },
    async getRBACRole(name) {
      const binding = bindings.GetDesktopRBACRole;
      if (!binding) unavailableBinding("RBAC role detail");
      return adaptRBACRole(await binding(name));
    },
    async importConfigHubSnippet(request) {
      const binding = bindings.ImportConfigHubSnippet;
      if (!binding) unavailableBinding("Hub snippet import");
      return { ...(await binding({ ...request })) };
    },
    async initializeConfigRepository(request) {
      const binding = bindings.InitializeConfigRepository;
      if (!binding) unavailableBinding("Configuration repository initialization");
      const result = await binding({ ...request });
      return { ...result, workingTree: { ...result.workingTree } };
    },
    async listAppPackages(prefix) {
      const binding = bindings.ListAppPackages;
      if (!binding) unavailableBinding("application package catalog");
      return (await binding(prefix)).map((item) => ({ ...item }));
    },
    async listAIIntegrations(request) {
      const binding = bindings.ListAIIntegrations;
      if (!binding) unavailableBinding("AI integration inventory");
      return (await binding({ ...request })).map((integration) => ({ ...integration }));
    },
    async listSecretVersions(name) {
      const binding = bindings.ListSecretVersions;
      if (!binding) unavailableBinding("Secret version inventory");
      return (await binding(name)).map(adaptSecretVersion);
    },
    async listDeploymentTokens() {
      const result = await bindings.ListDeploymentTokens();
      return result.map(cloneDeploymentToken);
    },
    async listRBACOperators() {
      const binding = bindings.ListDesktopRBACOperators;
      if (!binding) unavailableBinding("Operator inventory");
      return (await binding()).map(adaptRBACOperator);
    },
    async listRBACRoles() {
      const binding = bindings.ListDesktopRBACRoles;
      if (!binding) unavailableBinding("RBAC role inventory");
      return (await binding()).map(adaptRBACRole);
    },
    async listConfigHubSnippets(workingTreeId) {
      const binding = bindings.ListConfigHubSnippets;
      if (!binding) unavailableBinding("Hub snippet catalog");
      return (await binding(workingTreeId)).map((snippet) => ({
        ...snippet,
        distros: [...snippet.distros],
        tags: [...snippet.tags],
      }));
    },
    async loadAssetInventory() {
      const result = await bindings.LoadAssetInventory();
      return {
        omittedEndpointIds: [...result.omittedEndpointIds],
        rows: result.rows.map((row) => ({ ...row })),
        section: cloneReportSection(result.section),
      };
    },
    async loadActivityPage(request) {
      const binding = bindings.LoadActivityPage;
      if (!binding) unavailableBinding("Activity page");
      const page = await binding({
        ...request,
        seenEventIds: [...request.seenEventIds],
      });
      return {
        events: page.events.map((event) => ({
          ...event,
          details: event.details.map((detail) => ({ ...detail })),
        })),
        nextCursor: page.nextCursor,
        section: {
          ...(page.section.error ? { error: { ...page.section.error } } : {}),
          snapshot: { ...page.section.snapshot },
          state: page.section.state,
        },
      };
    },
    async loadAuditExportInfo() {
      const result = await bindings.LoadAuditExportInfo();
      return { exportPath: result.exportPath, pathKey: result.pathKey };
    },
    async loadChangeRequestDetail(changeRequestId) {
      const binding = bindings.LoadChangeRequestDetail;
      if (!binding) unavailableBinding("Change request detail");
      return cloneChangeRequestDetail(await binding(changeRequestId));
    },
    async loadAppPackage(name, version) {
      const binding = bindings.LoadAppPackage;
      if (!binding) unavailableBinding("application package detail");
      return { ...(await binding(name, version)) };
    },
    async loadDiagnosticRequest(requestId) {
      const result = await bindings.LoadDiagnosticRequest(requestId);
      return { ...result, collectors: [...result.collectors] };
    },
    async loadDeploymentToken(label) {
      return cloneDeploymentToken(await bindings.LoadDeploymentToken(label));
    },
    async loadFirewallReport(endpointId) {
      const result = await bindings.LoadFirewallReport(endpointId);
      return {
        audit: result.audit.map((event) => ({
          ...event,
          ports: [...event.ports],
          sources: [...event.sources],
        })),
        endpointId: result.endpointId,
        live: {
          ...result.live,
          zones: result.live.zones.map((zone) => ({
            ...zone,
            ports: [...zone.ports],
            richRules: [...zone.richRules],
            services: [...zone.services],
            sources: [...zone.sources],
          })),
        },
        sections: {
          audit: cloneReportSection(result.sections.audit),
          live: cloneReportSection(result.sections.live),
        },
      };
    },
    async loadFleetOperationalReports(fleet) {
      const result = await bindings.LoadFleetOperationalReports(fleet);
      return {
        fleet: result.fleet,
        schedules: result.schedules.map((schedule) => ({ ...schedule })),
        sections: {
          schedules: cloneReportSection(result.sections.schedules),
          state: cloneReportSection(result.sections.state),
        },
        states: result.states.map((state) => ({
          ...state,
          items: state.items.map((item) => ({
            ...item,
            subresults: item.subresults.map((subresult) => ({ ...subresult })),
          })),
        })),
      };
    },
    async loadSetupMaintenance() {
      const binding = bindings.LoadSetupMaintenance;
      if (!binding) unavailableBinding("setup and maintenance");
      return adaptSetupMaintenance(await binding());
    },
    async loadWorkspace() {
      const binding = bindings.LoadWorkspace;
      if (!binding) unavailableBinding("workspace loading");
      return adaptWorkspace(await binding());
    },
    async openRemotrDocumentation() {
      const binding = bindings.OpenRemotrDocumentation;
      if (!binding) unavailableBinding("documentation handoff");
      await binding();
    },
    async removeEndpointLabel(request) {
      return adaptEndpointLabelResult(
        await bindings.RemoveEndpointLabel({ ...request }),
      );
    },
    async removeEndpoint(request) {
      const result = await bindings.RemoveEndpoint({ ...request });
      if (
        result.status !== "removed" ||
        result.credentialStatus !== "not_enrolled"
      ) {
        throw new Error(
          "The native bridge returned an unknown Endpoint removal state.",
        );
      }
      return {
        affectedEvidence: [...result.affectedEvidence],
        credentialStatus: "not_enrolled",
        endpointId: result.endpointId,
        status: "removed",
      };
    },
    async removeRBACRule(request) {
      const binding = bindings.RemoveDesktopRBACRule;
      if (!binding) unavailableBinding("RBAC rule removal");
      return adaptRBACMutation(await binding({ ...request }));
    },
    async promoteChangeBaseline(request) {
      const binding = bindings.PromoteChangeBaseline;
      if (!binding) unavailableBinding("baseline promotion");
      return cloneChangeActionResult(await binding({ ...request }));
    },
    async requestEndpointAgentUpgrade(request) {
      return adaptEndpointUpgradeResult(
        await bindings.RequestEndpointAgentUpgrade({ ...request }),
      );
    },
    async requestDiagnosticCollection(request) {
      const result = await bindings.RequestDiagnosticCollection({
        ...request,
        collectors: [...request.collectors],
      });
      return {
        collectors: [...result.collectors],
        ...(result.createdAt ? { createdAt: result.createdAt } : {}),
        endpointId: result.endpointId,
        ...(result.expiresAt ? { expiresAt: result.expiresAt } : {}),
        requestId: result.requestId,
        since: result.since,
        status: result.status,
        until: result.until,
      };
    },
    async requestFleetAgentUpgrade(request) {
      return adaptFleetUpgradeResult(
        await bindings.RequestFleetAgentUpgrade({ ...request }),
      );
    },
    async requestGitSync() {
      const result = await bindings.RequestGitSync();

      return {
        acceptedAt: result.acceptedAt,
        action: result.action,
        affectedEvidence: [...result.affectedEvidence],
        summary: result.summary,
        target: result.target,
      };
    },
    async renderConfigRepository(request) {
      const binding = bindings.RenderConfigRepository;
      if (!binding) unavailableBinding("Configuration render preview");
      const result = await binding({ ...request });
      return { ...result, artifacts: result.artifacts.map((artifact) => ({ ...artifact })) };
    },
    async runDesktopDoctor(profile) {
      const binding = bindings.RunDesktopDoctor;
      if (!binding) unavailableBinding("desktop doctor");
      return adaptDoctorReport(await binding(cloneConnectionProfile(profile)));
    },
    async publishAppPackage(request) {
      const binding = bindings.PublishAppPackage;
      if (!binding) unavailableBinding("application package publication");
      return { ...(await binding({ ...request })) };
    },
    async revokeDeploymentToken(request) {
      return cloneDeploymentToken(
        await bindings.RevokeDeploymentToken({ ...request }),
      );
    },
    async revokeSecretVersion(request) {
      const binding = bindings.RevokeSecretVersion;
      if (!binding) unavailableBinding("Secret revocation");
      return adaptSecretVersion(await binding({ ...request }));
    },
    async saveAssetInventory(format) {
      return adaptReadExportSaveResult(
        await bindings.SaveAssetInventory(format),
      );
    },
    async saveConfigRender(request) {
      const binding = bindings.SaveConfigRender;
      if (!binding) unavailableBinding("Configuration render save");
      return { ...(await binding({ ...request })) };
    },
    async saveDiagnosticBundle(requestId) {
      const result = await bindings.SaveDiagnosticBundle(requestId);
      if (result.status !== "saved" && result.status !== "canceled") {
        throw new Error(
          "The native bridge returned an unknown diagnostic save state.",
        );
      }
      return {
        ...(result.path ? { path: result.path } : {}),
        ...(typeof result.sizeBytes === "number"
          ? { sizeBytes: result.sizeBytes }
          : {}),
        status: result.status,
      };
    },
    async saveDeploymentToken(label) {
      return adaptDeploymentTokenSaveResult(
        await bindings.SaveDeploymentToken(label),
      );
    },
    async saveFirewallReport(request) {
      return adaptReadExportSaveResult(
        await bindings.SaveFirewallReport({ ...request }),
      );
    },
    async saveProfile(profile) {
      const binding = bindings.SaveProfile;
      if (!binding) unavailableBinding("profile persistence");
      await binding(cloneConnectionProfile(profile));
    },
    async setEndpointLabel(request) {
      return adaptEndpointLabelResult(
        await bindings.SetEndpointLabel({ ...request }),
      );
    },
    async setOperatorRoles(request) {
      const binding = bindings.SetDesktopOperatorRoles;
      if (!binding) unavailableBinding("Operator role assignment");
      return adaptRBACOperator(
        await binding({ ...request, roles: [...request.roles] }),
      );
    },
    async stampOperatorCredential(request) {
      const binding = bindings.StampDesktopOperatorCredential;
      if (!binding) unavailableBinding("protected Operator credential output");
      return adaptCredentialStamp(
        await binding({ ...request, roles: [...request.roles] }),
      );
    },
    async setupAIIntegration(request) {
      const binding = bindings.SetupAIIntegration;
      if (!binding) unavailableBinding("AI integration setup");
      const result = await binding({ ...request });
      return { ...result, integration: { ...result.integration } };
    },
    async uploadSecretVersion(request) {
      const binding = bindings.UploadSecretVersion;
      if (!binding) unavailableBinding("protected Secret upload");
      return adaptSecretVersion(await binding({ ...request }));
    },
    async upgradeAIIntegration(request) {
      const binding = bindings.UpgradeAIIntegration;
      if (!binding) unavailableBinding("AI integration upgrade");
      const result = await binding({ ...request });
      return { ...result, integration: { ...result.integration } };
    },
    async validateConfigRepository(workingTreeId) {
      const binding = bindings.ValidateConfigRepository;
      if (!binding) unavailableBinding("Configuration repository validation");
      const result = await binding(workingTreeId);
      return {
        ...result,
        diagnostics: result.diagnostics.map((diagnostic) => ({ ...diagnostic })),
        issues: result.issues.map((issue) => ({ ...issue })),
        ok: [...result.ok],
      };
    },
  };
}

const DesktopBridgeContext = createContext<DesktopBridge | undefined>(undefined);

export function BridgeProvider({
  bridge,
  children,
}: PropsWithChildren<{ bridge: DesktopBridge }>) {
  return (
    <DesktopBridgeContext.Provider value={bridge}>
      {children}
    </DesktopBridgeContext.Provider>
  );
}

export function useDesktopBridge(): DesktopBridge {
  const bridge = useContext(DesktopBridgeContext);
  if (bridge === undefined) {
    throw new Error("useDesktopBridge must be used within BridgeProvider");
  }

  return bridge;
}
