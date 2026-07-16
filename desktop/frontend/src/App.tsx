import { GitPullRequest, KeyRound } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { ActivityDetail } from "./activity/ActivityDetail";
import { GitSyncPanel } from "./actions/GitSyncPanel";
import { EnrollmentTokenPanel } from "./actions/EnrollmentTokenPanel";
import { EndpointLabelPanel } from "./actions/EndpointLabelPanel";
import { EndpointUpgradePanel } from "./actions/EndpointUpgradePanel";
import { FleetUpgradePanel } from "./actions/FleetUpgradePanel";
import { DiagnosticCollectionPage } from "./actions/DiagnosticCollectionPage";
import { DeploymentTokenPage } from "./actions/DeploymentTokenPage";
import { ApplicationPackagePage } from "./actions/ApplicationPackagePage";
import type {
  AppPackageArchiveView,
  AppPackageDeleteRequest,
  AppPackageDeleteResult,
  AppPackagePublishRequest,
  AppPackageView,
  LocalPackageCreateRequest,
  LocalPackageView,
} from "./actions/applicationPackage";
import type {
  DeploymentTokenCreateRequest,
  DeploymentTokenCreateResult,
  DeploymentTokenRevokeRequest,
  DeploymentTokenSaveResult,
  DeploymentTokenView,
} from "./actions/deploymentToken";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "./actions/enrollmentToken";
import type {
  EndpointLabelRemoveRequest,
  EndpointLabelResult,
  EndpointLabelSetRequest,
} from "./actions/endpointLabel";
import type {
  EndpointUpgradeEvidence,
  EndpointUpgradeRequest,
  EndpointUpgradeResult,
} from "./actions/endpointUpgrade";
import type {
  FleetUpgradeRequest,
  FleetUpgradeResult,
} from "./actions/fleetUpgrade";
import type {
  DiagnosticCapabilities,
  DiagnosticCollectionRequest,
  DiagnosticCollectionResult,
} from "./actions/diagnosticCollection";
import type { DiagnosticBundleSaveResult } from "./actions/diagnosticBundle";
import type {
  EndpointRemovalRequest,
  EndpointRemovalResult,
} from "./actions/endpointRemoval";
import type { ActionAcknowledgement } from "./actions/useActionController";
import {
  ActivityPage,
  type ActivityEventView,
  type ActivityPageRequest,
  type ActivityPageView,
} from "./activity/ActivityPage";
import {
  ChangeRequestDetail,
  type ChangeRequestDetailView,
} from "./changes/ChangeRequestDetail";
import { ChangeRequestPage } from "./changes/ChangeRequestPage";
import type {
  BaselineAdoptionPreview,
  BaselineAdoptionRequest,
  ChangeActionResult,
  ChangeAuthorizationRequest,
  ChangeBaselinePromotionRequest,
  ChangeLifecycleRequest,
} from "./changes/changeControl";
import {
  EndpointInvestigation,
  type EndpointDetailView,
} from "./endpoints/EndpointInvestigation";
import { EndpointTable } from "./endpoints/EndpointTable";
import {
  FleetPage,
  type FleetDetailView,
} from "./fleets/FleetPage";
import {
  Overview,
  type OverviewNavigationTarget,
  type OverviewSectionResult,
  type OverviewWorkspace,
} from "./overview/Overview";
import { WorkspaceFreshness } from "./refresh/WorkspaceFreshness";
import { ReadExportPage } from "./reports/ReadExportPage";
import type {
  AssetInventoryView,
  AuditExportInfoView,
  DiagnosticLifecycleView,
  FirewallExportRequest,
  FirewallReportView,
  FleetOperationalReportsView,
  ReadExportSaveResult,
} from "./reports/readExport";
import {
  type RefreshClock,
  useWorkspaceRefresh,
  type WorkspaceVisibility,
} from "./refresh/useWorkspaceRefresh";
import { AppShell } from "./shell/AppShell";
import type { AppPage } from "./shell/AppShell";
import { DataState } from "./states/DataState";
import {
  InitialWorkspaceFailure,
  type InitialWorkspaceFailureView,
} from "./states/InitialWorkspaceFailure";

const pageLabels: Record<AppPage, string> = {
  overview: "Overview",
  endpoints: "Endpoints",
  fleets: "Fleets",
  "change-requests": "Change requests",
  diagnostics: "Diagnostics",
  "deployment-tokens": "Deployment tokens",
  "application-packages": "Application packages",
  reports: "Reports",
  activity: "Activity",
};

const firstActivityPageRequest: ActivityPageRequest = {
  action: "",
  actorType: "",
  cursor: "",
  seenEventIds: [],
  since: "",
  until: "",
};

interface ConnectedContext {
  connected?: boolean;
  operatorId: string;
  profileName: string;
  serverLabel: string;
}

interface AppProps {
  buildLocalPackage?: () => Promise<AppPackageArchiveView>;
  authorizeChangeRequest?: (
    request: ChangeAuthorizationRequest,
  ) => Promise<ChangeActionResult>;
  changeRequestLifecycle?: (
    request: ChangeLifecycleRequest,
  ) => Promise<ChangeActionResult>;
  chooseBaselineAdoptionPlan?: (
    fleet: string,
  ) => Promise<BaselineAdoptionPreview>;
  chooseAppPackageArchive?: () => Promise<AppPackageArchiveView>;
  chooseLocalPackageSource?: () => Promise<LocalPackageView>;
  clearDeploymentToken?: () => Promise<void>;
  clearEnrollmentToken?: () => Promise<void>;
  connection?: ConnectedContext;
  copyEnrollmentToken?: () => Promise<void>;
  createEnrollmentToken?: (
    request: EnrollmentTokenRequest,
  ) => Promise<EnrollmentTokenResult>;
  createBaselineAdoption?: (
    request: BaselineAdoptionRequest,
  ) => Promise<ChangeActionResult>;
  createLocalPackage?: (
    request: LocalPackageCreateRequest,
  ) => Promise<LocalPackageView>;
  copyDeploymentToken?: () => Promise<void>;
  createDeploymentToken?: (
    request: DeploymentTokenCreateRequest,
  ) => Promise<DeploymentTokenCreateResult>;
  fleetScope?: string;
  diagnosticCapabilities?: DiagnosticCapabilities;
  deleteAppPackage?: (
    request: AppPackageDeleteRequest,
  ) => Promise<AppPackageDeleteResult>;
  loadDiagnosticCapabilities?: () => Promise<DiagnosticCapabilities>;
  loadAssetInventory?: () => Promise<AssetInventoryView>;
  loadAuditExportInfo?: () => Promise<AuditExportInfoView>;
  loadActivityPage?: (request: ActivityPageRequest) => Promise<ActivityPageView>;
  loadChangeRequestDetail?: (
    changeRequestId: string,
  ) => Promise<ChangeRequestDetailView>;
  loadDiagnosticRequest?: (
    requestId: string,
  ) => Promise<DiagnosticLifecycleView>;
  listDeploymentTokens?: () => Promise<DeploymentTokenView[]>;
  listAppPackages?: (prefix: string) => Promise<AppPackageView[]>;
  loadAppPackage?: (name: string, version: string) => Promise<AppPackageView>;
  loadDeploymentToken?: (label: string) => Promise<DeploymentTokenView>;
  loadEndpointDetail?: (endpointId: string) => Promise<EndpointDetailView>;
  loadFirewallReport?: (endpointId: string) => Promise<FirewallReportView>;
  loadFleetDetail?: (fleet: string) => Promise<FleetDetailView>;
  loadFleetOperationalReports?: (
    fleet: string,
  ) => Promise<FleetOperationalReportsView>;
  loadWorkspace?: () => Promise<OverviewWorkspace>;
  onChooseProfile?: () => void;
  onCreateEnrollmentToken?: () => void;
  onInspectDiagnosticRequest?: (requestId: string) => void;
  onOpenEndpoint?: (endpointId: string) => void;
  onRetryConnection?: () => void;
  promoteChangeBaseline?: (
    request: ChangeBaselinePromotionRequest,
  ) => Promise<ChangeActionResult>;
  publishAppPackage?: (
    request: AppPackagePublishRequest,
  ) => Promise<AppPackageView>;
  refreshClock?: RefreshClock;
  removeEndpointLabel?: (
    request: EndpointLabelRemoveRequest,
  ) => Promise<EndpointLabelResult>;
  removeEndpoint?: (
    request: EndpointRemovalRequest,
  ) => Promise<EndpointRemovalResult>;
  requestEndpointAgentUpgrade?: (
    request: EndpointUpgradeRequest,
  ) => Promise<EndpointUpgradeResult>;
  requestDiagnosticCollection?: (
    request: DiagnosticCollectionRequest,
  ) => Promise<DiagnosticCollectionResult>;
  requestFleetAgentUpgrade?: (
    request: FleetUpgradeRequest,
  ) => Promise<FleetUpgradeResult>;
  requestGitSync?: () => Promise<ActionAcknowledgement>;
  revokeDeploymentToken?: (
    request: DeploymentTokenRevokeRequest,
  ) => Promise<DeploymentTokenView>;
  saveDiagnosticBundle?: (
    requestId: string,
  ) => Promise<DiagnosticBundleSaveResult>;
  saveDeploymentToken?: (
    label: string,
  ) => Promise<DeploymentTokenSaveResult>;
  saveAssetInventory?: (
    format: "csv" | "json",
  ) => Promise<ReadExportSaveResult>;
  saveFirewallReport?: (
    request: FirewallExportRequest,
  ) => Promise<ReadExportSaveResult>;
  setEndpointLabel?: (
    request: EndpointLabelSetRequest,
  ) => Promise<EndpointLabelResult>;
  workspace?: OverviewWorkspace;
  workspaceFailure?: InitialWorkspaceFailureView;
  workspaceVisibility?: WorkspaceVisibility;
}

interface EndpointDetailFailure {
  endpointId: string;
  guidance: string;
  message: string;
}

interface ChangeRequestDetailFailure {
  changeRequestId: string;
  guidance: string;
  message: string;
}

function filterSummary(filters: OverviewNavigationTarget["filters"]): string {
  return Object.entries(filters)
    .map(([name, values]) => `${name}: ${values.join(", ")}`)
    .join(" · ");
}

function sectionForPage(
  workspace: OverviewWorkspace,
  page: AppPage,
): OverviewSectionResult {
  switch (page) {
    case "endpoints":
      return workspace.sections.endpoints;
    case "fleets":
      return workspace.sections.fleets;
    case "change-requests":
      return workspace.sections.changeRequests;
    case "activity":
      return workspace.sections.activity;
    case "overview":
    case "diagnostics":
    case "deployment-tokens":
    case "application-packages":
    case "reports":
      return workspace.sections.state;
  }
}

export function App({
  authorizeChangeRequest,
  buildLocalPackage,
  changeRequestLifecycle,
  chooseAppPackageArchive,
  chooseBaselineAdoptionPlan,
  chooseLocalPackageSource,
  clearDeploymentToken,
  clearEnrollmentToken,
  connection,
  copyEnrollmentToken,
  copyDeploymentToken,
  createDeploymentToken,
  createEnrollmentToken,
  createBaselineAdoption,
  createLocalPackage,
  diagnosticCapabilities,
  deleteAppPackage,
  fleetScope = "All Fleets",
  loadAssetInventory,
  loadAuditExportInfo,
  loadDiagnosticCapabilities,
  loadActivityPage,
  loadChangeRequestDetail,
  loadDiagnosticRequest,
  listAppPackages,
  listDeploymentTokens,
  loadAppPackage,
  loadDeploymentToken,
  loadEndpointDetail,
  loadFirewallReport,
  loadFleetDetail,
  loadFleetOperationalReports,
  loadWorkspace,
  onChooseProfile,
  onCreateEnrollmentToken,
  onInspectDiagnosticRequest,
  onOpenEndpoint,
  onRetryConnection,
  promoteChangeBaseline,
  publishAppPackage,
  refreshClock,
  removeEndpointLabel,
  removeEndpoint,
  requestDiagnosticCollection,
  requestEndpointAgentUpgrade,
  requestFleetAgentUpgrade,
  requestGitSync,
  revokeDeploymentToken,
  saveAssetInventory,
  saveDeploymentToken,
  saveDiagnosticBundle,
  saveFirewallReport,
  setEndpointLabel,
  workspace: suppliedWorkspace,
  workspaceFailure,
  workspaceVisibility,
}: AppProps) {
  const {
    failure: workspaceRefreshFailure,
    refresh: refreshWorkspace,
    refreshing: workspaceRefreshing,
    updateWorkspace,
    workspace,
  } = useWorkspaceRefresh({
    clock: refreshClock,
    loadWorkspace,
    visibility: workspaceVisibility,
    workspace: suppliedWorkspace,
  });
  const [activePage, setActivePage] = useState<AppPage>("overview");
  const [activeFilters, setActiveFilters] = useState<
    OverviewNavigationTarget["filters"]
  >({});
  const [endpointDetail, setEndpointDetail] = useState<EndpointDetailView>();
  const [endpointDetailFailure, setEndpointDetailFailure] =
    useState<EndpointDetailFailure>();
  const [changeRequestDetail, setChangeRequestDetail] =
    useState<ChangeRequestDetailView>();
  const [changeRequestDetailFailure, setChangeRequestDetailFailure] =
    useState<ChangeRequestDetailFailure>();
  const [activityDetail, setActivityDetail] = useState<ActivityEventView>();
  const [enrollmentTokenOpen, setEnrollmentTokenOpen] = useState(false);
  const [enrollmentTokenPending, setEnrollmentTokenPending] = useState(false);
  const [gitSyncOpen, setGitSyncOpen] = useState(false);
  const [gitSyncPending, setGitSyncPending] = useState(false);
  const [labelEndpointId, setLabelEndpointId] = useState<string>();
  const [labelPending, setLabelPending] = useState(false);
  const [upgradeEndpointId, setUpgradeEndpointId] = useState<string>();
  const [upgradePending, setUpgradePending] = useState(false);
  const [fleetUpgradeName, setFleetUpgradeName] = useState<string>();
  const [fleetUpgradePending, setFleetUpgradePending] = useState(false);
  const [endpointRemovalResult, setEndpointRemovalResult] =
    useState<EndpointRemovalResult>();
  const endpointDetailGeneration = useRef(0);
  const endpointDetailOrigin = useRef<HTMLElement | null>(null);
  const changeRequestDetailGeneration = useRef(0);
  const changeRequestDetailOrigin = useRef<HTMLElement | null>(null);
  const activityDetailOrigin = useRef<HTMLElement | null>(null);

  useEffect(() => {
    setEnrollmentTokenOpen(false);
    setEnrollmentTokenPending(false);
    setGitSyncOpen(false);
    setGitSyncPending(false);
    setLabelEndpointId(undefined);
    setLabelPending(false);
    setUpgradeEndpointId(undefined);
    setUpgradePending(false);
    setFleetUpgradeName(undefined);
    setFleetUpgradePending(false);
    setEndpointRemovalResult(undefined);
    changeRequestDetailGeneration.current += 1;
    setChangeRequestDetail(undefined);
    setChangeRequestDetailFailure(undefined);
  }, [connection?.profileName, connection?.serverLabel]);

  const navigateFromOverview = (target: OverviewNavigationTarget) => {
    setActiveFilters(target.filters);
    setActivePage(target.page);
  };

  const selectNavigationPage = (page: AppPage) => {
    setActiveFilters({});
    setActivePage(page);
  };

  const inspectEndpoint = (endpointId: string) => {
    onOpenEndpoint?.(endpointId);
    if (!loadEndpointDetail) {
      return;
    }

    if (document.activeElement instanceof HTMLElement) {
      endpointDetailOrigin.current = document.activeElement;
    }
    const generation = ++endpointDetailGeneration.current;
    setEndpointDetail(undefined);
    setEndpointDetailFailure(undefined);
    void loadEndpointDetail(endpointId)
      .then((detail) => {
        if (generation !== endpointDetailGeneration.current) {
          return;
        }
        if (detail.header.endpointId !== endpointId) {
          setEndpointDetailFailure({
            endpointId,
            guidance: "Close this surface and select the Endpoint again.",
            message: "The returned evidence did not match the selected Endpoint.",
          });
          return;
        }
        setEndpointDetail(detail);
      })
      .catch((error: unknown) => {
        if (generation !== endpointDetailGeneration.current) {
          return;
        }
        const classified =
          typeof error === "object" && error !== null
            ? (error as { guidance?: unknown; message?: unknown })
            : undefined;
        setEndpointDetailFailure({
          endpointId,
          guidance:
            typeof classified?.guidance === "string"
              ? classified.guidance
              : "Check the connection and select the Endpoint again.",
          message:
            typeof classified?.message === "string"
              ? classified.message
              : "Endpoint evidence could not be loaded safely.",
        });
      });
  };

  const closeEndpointDetail = () => {
    endpointDetailGeneration.current += 1;
    setEndpointDetail(undefined);
    setEndpointDetailFailure(undefined);
    endpointDetailOrigin.current?.focus();
  };

  const completeEndpointRemoval = (result: EndpointRemovalResult) => {
    if (result.endpointId !== endpointDetail?.header.endpointId) {
      throw new Error(
        "The removal result did not match the selected Endpoint.",
      );
    }
    endpointDetailGeneration.current += 1;
    setEndpointDetail(undefined);
    setEndpointDetailFailure(undefined);
    setEndpointRemovalResult(result);
    updateWorkspace((current) =>
      current
        ? {
            ...current,
            endpoints: current.endpoints.filter(
              (endpoint) => endpoint.endpointId !== result.endpointId,
            ),
          }
        : current,
    );
    document
      .querySelector<HTMLButtonElement>(
        '.navigation-item[aria-current="page"]',
      )
      ?.focus();
    refreshWorkspace();
  };

  const inspectChangeRequest = (changeRequestId: string) => {
    if (!loadChangeRequestDetail) {
      return;
    }

    if (document.activeElement instanceof HTMLElement) {
      changeRequestDetailOrigin.current = document.activeElement;
    }
    const generation = ++changeRequestDetailGeneration.current;
    setChangeRequestDetail(undefined);
    setChangeRequestDetailFailure(undefined);
    void loadChangeRequestDetail(changeRequestId)
      .then((detail) => {
        if (generation !== changeRequestDetailGeneration.current) {
          return;
        }
        if (detail.summary.changeRequestId !== changeRequestId) {
          setChangeRequestDetailFailure({
            changeRequestId,
            guidance: "Close this surface and select the Change request again.",
            message:
              "The returned evidence did not match the selected Change request.",
          });
          return;
        }
        setChangeRequestDetail(detail);
      })
      .catch((error: unknown) => {
        if (generation !== changeRequestDetailGeneration.current) {
          return;
        }
        const classified =
          typeof error === "object" && error !== null
            ? (error as { guidance?: unknown; message?: unknown })
            : undefined;
        setChangeRequestDetailFailure({
          changeRequestId,
          guidance:
            typeof classified?.guidance === "string"
              ? classified.guidance
              : "Check the connection and select the Change request again.",
          message:
            typeof classified?.message === "string"
              ? classified.message
              : "Change request evidence could not be loaded safely.",
        });
      });
  };

  const closeChangeRequestDetail = () => {
    changeRequestDetailGeneration.current += 1;
    setChangeRequestDetail(undefined);
    setChangeRequestDetailFailure(undefined);
    changeRequestDetailOrigin.current?.focus();
  };

  const updateChangeRequestEvidence = (detail: ChangeRequestDetailView) => {
    setChangeRequestDetail(detail);
    updateWorkspace((current) => {
      if (!current) {
        return current;
      }
      const nextSummary = detail.summary;
      const exists = current.changeRequests.some(
        (change) => change.changeRequestId === nextSummary.changeRequestId,
      );
      return {
        ...current,
        changeRequests: exists
          ? current.changeRequests.map((change) =>
              change.changeRequestId === nextSummary.changeRequestId
                ? nextSummary
                : change,
            )
          : [...current.changeRequests, nextSummary],
      };
    });
  };

  const completeChangeControlAction = (result: ChangeActionResult) => {
    updateChangeRequestEvidence(result.changeRequest);
  };

  const inspectActivity = (event: ActivityEventView) => {
    if (document.activeElement instanceof HTMLElement) {
      activityDetailOrigin.current = document.activeElement;
    }
    setActivityDetail(event);
  };

  const closeActivityDetail = () => {
    setActivityDetail(undefined);
    activityDetailOrigin.current?.focus();
  };

  const closeEndpointLabels = () => {
    if (!labelPending) {
      setLabelEndpointId(undefined);
    }
  };

  const refreshServerActivity = async () => {
    if (!loadActivityPage) {
      return;
    }
    const activityPage = await loadActivityPage(firstActivityPageRequest);
    updateWorkspace((current) =>
      current
        ? {
            ...current,
            activity: [...activityPage.events],
            activityNextCursor: activityPage.nextCursor,
            sections: {
              ...current.sections,
              activity: activityPage.section,
            },
          }
        : current,
    );
  };

  const refreshEndpointLabelEvidence = async (
    result: EndpointLabelResult,
  ) => {
    if (result.endpointId !== labelEndpointId) {
      throw new Error("The Label result did not match the selected Endpoint.");
    }

    updateWorkspace((current) =>
      current
        ? {
            ...current,
            endpoints: current.endpoints.map((endpoint) =>
              endpoint.endpointId === result.endpointId
                ? { ...endpoint, labels: [...result.labels] }
                : endpoint,
            ),
          }
        : current,
    );

    const endpointRequest = loadEndpointDetail
      ? loadEndpointDetail(result.endpointId)
      : Promise.resolve(undefined);
    const [detail] = await Promise.all([
      endpointRequest,
      refreshServerActivity(),
    ]);

    if (detail) {
      if (detail.header.endpointId !== result.endpointId) {
        throw new Error(
          "Refreshed Endpoint evidence did not match the Label target.",
        );
      }
      updateWorkspace((current) =>
        current
          ? {
              ...current,
              endpoints: current.endpoints.map((endpoint) =>
                endpoint.endpointId === result.endpointId
                  ? detail.header
                  : endpoint,
              ),
            }
          : current,
      );
      setEndpointDetail((current) =>
        current?.header.endpointId === result.endpointId ? detail : current,
      );
    }
  };

  const closeEndpointUpgrade = () => {
    if (!upgradePending) {
      setUpgradeEndpointId(undefined);
    }
  };

  const refreshEndpointUpgradeEvidence = async (
    result: EndpointUpgradeResult,
  ): Promise<EndpointUpgradeEvidence> => {
    if (result.endpointId !== upgradeEndpointId) {
      throw new Error(
        "The upgrade result did not match the selected Endpoint.",
      );
    }

    const endpointRequest = loadEndpointDetail
      ? loadEndpointDetail(result.endpointId)
      : Promise.resolve(undefined);
    const [detail] = await Promise.all([
      endpointRequest,
      refreshServerActivity(),
    ]);

    if (detail) {
      if (detail.header.endpointId !== result.endpointId) {
        throw new Error(
          "Refreshed Endpoint evidence did not match the upgrade target.",
        );
      }
      updateWorkspace((current) =>
        current
          ? {
              ...current,
              endpoints: current.endpoints.map((endpoint) =>
                endpoint.endpointId === result.endpointId
                  ? detail.header
                  : endpoint,
              ),
            }
          : current,
      );
      setEndpointDetail((current) =>
        current?.header.endpointId === result.endpointId ? detail : current,
      );
    }

    const currentEndpoint = detail?.header ?? workspace?.endpoints.find(
      (endpoint) => endpoint.endpointId === result.endpointId,
    );
    return {
      desiredAgentVersion: currentEndpoint?.desiredAgentVersion ?? "",
      reportedAgentVersion: currentEndpoint?.reportedAgentVersion ?? "",
    };
  };

  const endpointOverlay = endpointDetail
    ? {
        content: (
          <EndpointInvestigation
            detail={endpointDetail}
            key={endpointDetail.header.endpointId}
            onEndpointRemoved={
              removeEndpoint ? completeEndpointRemoval : undefined
            }
            removeEndpoint={removeEndpoint}
          />
        ),
        onClose: closeEndpointDetail,
        title: `Endpoint ${endpointDetail.header.endpointId}`,
      }
    : endpointDetailFailure
      ? {
          content: (
            <DataState
              guidance={endpointDetailFailure.guidance}
              kind="unexpected"
              message={endpointDetailFailure.message}
              title="Endpoint detail unavailable"
            />
          ),
          onClose: closeEndpointDetail,
          title: `Endpoint ${endpointDetailFailure.endpointId}`,
        }
      : undefined;

  const changeRequestOverlay = changeRequestDetail
    ? {
        content: (
          <ChangeRequestDetail
            authorizeChangeRequest={authorizeChangeRequest}
            changeRequestLifecycle={changeRequestLifecycle}
            chooseBaselineAdoptionPlan={chooseBaselineAdoptionPlan}
            clock={refreshClock}
            createBaselineAdoption={createBaselineAdoption}
            detail={changeRequestDetail}
            key={changeRequestDetail.summary.changeRequestId}
            loadChangeRequestDetail={loadChangeRequestDetail}
            onChanged={completeChangeControlAction}
            onDetailObserved={updateChangeRequestEvidence}
            promoteChangeBaseline={promoteChangeBaseline}
            refreshActivity={refreshServerActivity}
            visibility={workspaceVisibility}
          />
        ),
        onClose: closeChangeRequestDetail,
        title: `Change request ${changeRequestDetail.summary.changeRequestId}`,
      }
    : changeRequestDetailFailure
      ? {
          content: (
            <DataState
              guidance={changeRequestDetailFailure.guidance}
              kind="unexpected"
              message={changeRequestDetailFailure.message}
              title="Change request detail unavailable"
            />
          ),
          onClose: closeChangeRequestDetail,
          title: `Change request ${changeRequestDetailFailure.changeRequestId}`,
        }
      : undefined;

  const activityOverlay = activityDetail
    ? {
        content: <ActivityDetail event={activityDetail} />,
        onClose: closeActivityDetail,
        title: `Activity event ${activityDetail.eventId}`,
      }
    : undefined;

  const labelEndpoint = workspace?.endpoints.find(
    (endpoint) => endpoint.endpointId === labelEndpointId,
  );
  const endpointLabelOverlay =
    labelEndpoint && setEndpointLabel && removeEndpointLabel
      ? {
          canClose: !labelPending,
          content: (
            <EndpointLabelPanel
              endpointId={labelEndpoint.endpointId}
              labels={labelEndpoint.labels}
              onClose={closeEndpointLabels}
              onPendingChange={setLabelPending}
              refreshAffected={refreshEndpointLabelEvidence}
              removeEndpointLabel={removeEndpointLabel}
              setEndpointLabel={setEndpointLabel}
            />
          ),
          onClose: closeEndpointLabels,
          title: `Manage Labels for ${labelEndpoint.endpointId}`,
        }
      : undefined;

  const upgradeEndpoint = workspace?.endpoints.find(
    (endpoint) => endpoint.endpointId === upgradeEndpointId,
  );
  const endpointUpgradeOverlay =
    upgradeEndpoint && requestEndpointAgentUpgrade
      ? {
          canClose: !upgradePending,
          content: (
            <EndpointUpgradePanel
              endpoint={upgradeEndpoint}
              onClose={closeEndpointUpgrade}
              onPendingChange={setUpgradePending}
              refreshAffected={refreshEndpointUpgradeEvidence}
              requestEndpointAgentUpgrade={requestEndpointAgentUpgrade}
            />
          ),
          onClose: closeEndpointUpgrade,
          title: `Request agent upgrade for ${upgradeEndpoint.endpointId}`,
        }
      : undefined;

  const closeFleetUpgrade = () => {
    if (!fleetUpgradePending) {
      setFleetUpgradeName(undefined);
    }
  };
  const fleetUpgradeSummary = workspace?.fleets.find(
    (fleet) => fleet.fleet === fleetUpgradeName,
  );
  const fleetUpgradeOverlay =
    fleetUpgradeSummary && requestFleetAgentUpgrade
      ? {
          canClose: !fleetUpgradePending,
          content: (
            <FleetUpgradePanel
              fleet={fleetUpgradeSummary.fleet}
              memberCount={fleetUpgradeSummary.endpointCount}
              onClose={closeFleetUpgrade}
              onPendingChange={setFleetUpgradePending}
              refreshActivity={refreshServerActivity}
              requestFleetAgentUpgrade={requestFleetAgentUpgrade}
            />
          ),
          onClose: closeFleetUpgrade,
          title: `Request agent upgrade for Fleet ${fleetUpgradeSummary.fleet}`,
        }
      : undefined;

  const availableFleets = workspace
    ? workspace.fleets.map((fleet) => fleet.fleet).toSorted()
    : [];
  const enrollmentTokenAvailable = Boolean(
    createEnrollmentToken &&
      copyEnrollmentToken &&
      clearEnrollmentToken &&
      connection &&
      connection.connected !== false &&
      workspace,
  );
  const closeEnrollmentToken = () => {
    if (!enrollmentTokenPending) {
      setEnrollmentTokenOpen(false);
    }
  };
  const enrollmentTokenOverlay =
    enrollmentTokenOpen &&
    createEnrollmentToken &&
    copyEnrollmentToken &&
    clearEnrollmentToken
      ? {
          canClose: !enrollmentTokenPending,
          content: (
            <EnrollmentTokenPanel
              clearEnrollmentToken={clearEnrollmentToken}
              copyEnrollmentToken={copyEnrollmentToken}
              createEnrollmentToken={createEnrollmentToken}
              fleets={availableFleets}
              onClose={closeEnrollmentToken}
              onPendingChange={setEnrollmentTokenPending}
              refreshAffected={() => refreshWorkspace()}
            />
          ),
          onClose: closeEnrollmentToken,
          title: "Create enrollment token",
        }
      : undefined;

  const releaseRefs = workspace
    ? Array.from(
        new Set(
          [
            ...workspace.endpoints.map((endpoint) => endpoint.releaseRef),
            ...workspace.changeRequests.map((change) => change.releaseRef),
          ].filter(
            (releaseRef): releaseRef is string =>
              typeof releaseRef === "string" && releaseRef.length > 0,
          ),
        ),
      ).toSorted()
    : [];
  const releaseEvidenceObservedAt = workspace
    ? Array.from(
        new Set(
          [
            ...workspace.endpoints
              .filter((endpoint) => endpoint.releaseRef)
              .map((endpoint) => endpoint.evidenceAt),
            ...workspace.changeRequests
              .filter((change) => change.releaseRef)
              .map((change) => change.updatedAt),
          ].filter(
            (observedAt): observedAt is string =>
              typeof observedAt === "string" && observedAt.length > 0,
          ),
        ),
      ).toSorted()
    : [];
  const closeGitSync = () => {
    if (!gitSyncPending) {
      setGitSyncOpen(false);
    }
  };
  const gitSyncOverlay =
    gitSyncOpen && requestGitSync && connection && workspace
      ? {
          canClose: !gitSyncPending,
          content: (
            <GitSyncPanel
              evidenceObservedAt={releaseEvidenceObservedAt}
              onCancel={closeGitSync}
              onPendingChange={setGitSyncPending}
              profileName={connection.profileName}
              refreshAffected={() => refreshWorkspace()}
              releaseRefs={releaseRefs}
              requestGitSync={requestGitSync}
              serverLabel={connection.serverLabel}
            />
          ),
          onClose: closeGitSync,
          title: "Sync from Git",
        }
      : undefined;

  const initialWorkspaceFailure = !workspace
    ? workspaceFailure ?? workspaceRefreshFailure
    : undefined;
  const gitSyncAvailable = Boolean(
    requestGitSync &&
      connection &&
      connection.connected !== false &&
      workspace,
  );
  const endpointLabelColumns = workspace
    ? Array.from(
        new Set(
          workspace.endpoints.flatMap((endpoint) =>
            endpoint.labels.map((label) => label.key),
          ),
        ),
      ).toSorted()
    : [];
  const pageActions =
    (activePage === "endpoints" && enrollmentTokenAvailable) ||
    gitSyncAvailable ? (
      <>
        {activePage === "endpoints" && enrollmentTokenAvailable ? (
          <button
            className="enrollment-token-trigger"
            onClick={() => setEnrollmentTokenOpen(true)}
            type="button"
          >
            <KeyRound aria-hidden="true" size={14} strokeWidth={1.8} />
            Create enrollment token
          </button>
        ) : null}
        {gitSyncAvailable ? (
          <button
            className="git-sync-trigger"
            onClick={() => setGitSyncOpen(true)}
            type="button"
          >
            <GitPullRequest aria-hidden="true" size={14} strokeWidth={1.8} />
            Sync from Git
          </button>
        ) : null}
      </>
    ) : undefined;

  return (
    <AppShell
      activePage={activePage}
      connection={{
        connected: connection?.connected ?? connection !== undefined,
        operatorId: connection?.operatorId ?? "No operator",
        profileName: connection?.profileName ?? "No profile selected",
        serverLabel: connection?.serverLabel ?? "Select a profile to begin",
      }}
      fleetScope={fleetScope}
      onPageChange={selectNavigationPage}
      onRefresh={loadWorkspace && workspace ? refreshWorkspace : undefined}
      overlay={
        fleetUpgradeOverlay ??
        endpointUpgradeOverlay ??
        endpointLabelOverlay ??
        enrollmentTokenOverlay ??
        gitSyncOverlay ??
        endpointOverlay ??
        changeRequestOverlay ??
        activityOverlay
      }
      pageActions={pageActions}
      refreshing={workspaceRefreshing}
      workspaceStatus={
        workspaceRefreshFailure && workspace ? (
          <WorkspaceFreshness
            failure={workspaceRefreshFailure}
            loadedAt={sectionForPage(workspace, activePage).snapshot.loadedAt}
            onRefresh={refreshWorkspace}
          />
        ) : undefined
      }
      renderPage={(page) => {
        if (initialWorkspaceFailure) {
          return (
            <InitialWorkspaceFailure
              failure={initialWorkspaceFailure}
              onChooseProfile={onChooseProfile}
              onRetry={
                onRetryConnection ??
                (loadWorkspace ? refreshWorkspace : undefined)
              }
              profileName={connection?.profileName ?? "No profile selected"}
              serverLabel={connection?.serverLabel ?? "Not configured"}
            />
          );
        }

        if (page === "overview" && workspace) {
          return (
            <Overview
              onNavigate={navigateFromOverview}
              workspace={workspace}
            />
          );
        }

        if (page === "endpoints" && workspace) {
          return (
            <>
              {endpointRemovalResult ? (
                <section
                  aria-label="Endpoint removed"
                  className="endpoint-removal-result"
                  role="status"
                >
                  <strong data-mono>{endpointRemovalResult.endpointId}</strong>
                  <p>
                    Removed from inventory. Its credential is no longer
                    enrolled.
                  </p>
                </section>
              ) : null}
              <EndpointTable
                endpoints={workspace.endpoints}
                initialFilters={activeFilters}
                labelColumns={endpointLabelColumns}
                onCreateEnrollmentToken={
                  enrollmentTokenAvailable
                    ? () => setEnrollmentTokenOpen(true)
                    : onCreateEnrollmentToken
                }
                onManageLabels={
                  setEndpointLabel && removeEndpointLabel
                    ? (endpoint) => setLabelEndpointId(endpoint.endpointId)
                    : undefined
                }
                onOpenEndpoint={
                  loadEndpointDetail || onOpenEndpoint
                    ? inspectEndpoint
                    : undefined
                }
                onRequestAgentUpgrade={
                  requestEndpointAgentUpgrade
                    ? (endpoint) => setUpgradeEndpointId(endpoint.endpointId)
                    : undefined
                }
              />
            </>
          );
        }

        if (page === "fleets" && workspace) {
          return (
            <FleetPage
              loadFleetDetail={loadFleetDetail}
              onOpenEndpoint={
                loadEndpointDetail || onOpenEndpoint
                  ? inspectEndpoint
                  : undefined
              }
              onRequestAgentUpgrade={
                requestFleetAgentUpgrade
                  ? (summary) => setFleetUpgradeName(summary.fleet)
                  : undefined
              }
              summaries={workspace.fleets}
            />
          );
        }

        if (page === "change-requests" && workspace) {
          return (
            <ChangeRequestPage
              initialLifecycleFilters={activeFilters.lifecycle}
              onInspect={
                loadChangeRequestDetail ? inspectChangeRequest : undefined
              }
              summaries={workspace.changeRequests}
            />
          );
        }

        if (page === "activity" && workspace) {
          return (
            <ActivityPage
              initialEvents={workspace.activity}
              initialNextCursor={workspace.activityNextCursor}
              initialSection={workspace.sections.activity}
              loadPage={loadActivityPage}
              onInspect={inspectActivity}
            />
          );
        }

        if (page === "reports" && workspace) {
          return (
            <ReadExportPage
              endpoints={workspace.endpoints}
              fleets={availableFleets}
              loadAssetInventory={loadAssetInventory}
              loadAuditExportInfo={loadAuditExportInfo}
              loadDiagnosticRequest={loadDiagnosticRequest}
              loadFirewallReport={loadFirewallReport}
              loadFleetOperationalReports={loadFleetOperationalReports}
              saveAssetInventory={saveAssetInventory}
              saveFirewallReport={saveFirewallReport}
            />
          );
        }

        if (
          page === "application-packages" &&
          workspace &&
          buildLocalPackage &&
          chooseAppPackageArchive &&
          chooseLocalPackageSource &&
          createLocalPackage &&
          deleteAppPackage &&
          listAppPackages &&
          loadAppPackage &&
          publishAppPackage
        ) {
          return (
            <ApplicationPackagePage
              buildLocalPackage={buildLocalPackage}
              chooseAppPackageArchive={chooseAppPackageArchive}
              chooseLocalPackageSource={chooseLocalPackageSource}
              createLocalPackage={createLocalPackage}
              deleteAppPackage={deleteAppPackage}
              listAppPackages={listAppPackages}
              loadAppPackage={loadAppPackage}
              publishAppPackage={publishAppPackage}
              refreshActivity={refreshServerActivity}
            />
          );
        }

        if (
          page === "deployment-tokens" &&
          workspace &&
          clearDeploymentToken &&
          copyDeploymentToken &&
          createDeploymentToken &&
          listDeploymentTokens &&
          loadDeploymentToken &&
          revokeDeploymentToken &&
          saveDeploymentToken
        ) {
          return (
            <DeploymentTokenPage
              clearDeploymentToken={clearDeploymentToken}
              copyDeploymentToken={copyDeploymentToken}
              createDeploymentToken={createDeploymentToken}
              fleets={availableFleets}
              listDeploymentTokens={listDeploymentTokens}
              loadDeploymentToken={loadDeploymentToken}
              refreshActivity={refreshServerActivity}
              revokeDeploymentToken={revokeDeploymentToken}
              saveDeploymentToken={saveDeploymentToken}
            />
          );
        }

        if (page === "diagnostics" && workspace && requestDiagnosticCollection) {
          return (
            <DiagnosticCollectionPage
              capabilities={diagnosticCapabilities}
              endpoints={workspace.endpoints}
              loadCapabilities={loadDiagnosticCapabilities}
              onInspectDiagnosticRequest={onInspectDiagnosticRequest}
              refreshActivity={refreshServerActivity}
              requestDiagnosticCollection={requestDiagnosticCollection}
              saveDiagnosticBundle={saveDiagnosticBundle}
            />
          );
        }

        const appliedFilters = filterSummary(activeFilters);
        return (
          <section
            aria-label={`${pageLabels[page]} workspace`}
            className="shell-start-state"
          >
            <span className="page-kicker">
              {workspace ? "Data surface pending" : "Workspace ready"}
            </span>
            <h2>{pageLabels[page]}</h2>
            <p>
              {appliedFilters
                ? `Applied filters — ${appliedFilters}`
                : "Connect an operator profile to load live fleet data."}
            </p>
          </section>
        );
      }}
    />
  );
}
