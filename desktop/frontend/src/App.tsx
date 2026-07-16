import { GitPullRequest, KeyRound } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { ActivityDetail } from "./activity/ActivityDetail";
import { GitSyncPanel } from "./actions/GitSyncPanel";
import { EnrollmentTokenPanel } from "./actions/EnrollmentTokenPanel";
import { EndpointLabelPanel } from "./actions/EndpointLabelPanel";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "./actions/enrollmentToken";
import type {
  EndpointLabelRemoveRequest,
  EndpointLabelResult,
  EndpointLabelSetRequest,
} from "./actions/endpointLabel";
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
  activity: "Activity",
};

interface ConnectedContext {
  connected?: boolean;
  operatorId: string;
  profileName: string;
  serverLabel: string;
}

interface AppProps {
  clearEnrollmentToken?: () => Promise<void>;
  connection?: ConnectedContext;
  copyEnrollmentToken?: () => Promise<void>;
  createEnrollmentToken?: (
    request: EnrollmentTokenRequest,
  ) => Promise<EnrollmentTokenResult>;
  fleetScope?: string;
  loadActivityPage?: (request: ActivityPageRequest) => Promise<ActivityPageView>;
  loadChangeRequestDetail?: (
    changeRequestId: string,
  ) => Promise<ChangeRequestDetailView>;
  loadEndpointDetail?: (endpointId: string) => Promise<EndpointDetailView>;
  loadFleetDetail?: (fleet: string) => Promise<FleetDetailView>;
  loadWorkspace?: () => Promise<OverviewWorkspace>;
  onChooseProfile?: () => void;
  onCreateEnrollmentToken?: () => void;
  onOpenEndpoint?: (endpointId: string) => void;
  onRetryConnection?: () => void;
  refreshClock?: RefreshClock;
  removeEndpointLabel?: (
    request: EndpointLabelRemoveRequest,
  ) => Promise<EndpointLabelResult>;
  requestGitSync?: () => Promise<ActionAcknowledgement>;
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
      return workspace.sections.state;
  }
}

export function App({
  clearEnrollmentToken,
  connection,
  copyEnrollmentToken,
  createEnrollmentToken,
  fleetScope = "All Fleets",
  loadActivityPage,
  loadChangeRequestDetail,
  loadEndpointDetail,
  loadFleetDetail,
  loadWorkspace,
  onChooseProfile,
  onCreateEnrollmentToken,
  onOpenEndpoint,
  onRetryConnection,
  refreshClock,
  removeEndpointLabel,
  requestGitSync,
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
    const activityRequest = loadActivityPage
      ? loadActivityPage({
          action: "",
          actorType: "",
          cursor: "",
          seenEventIds: [],
          since: "",
          until: "",
        })
      : Promise.resolve(undefined);
    const [detail, activityPage] = await Promise.all([
      endpointRequest,
      activityRequest,
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

    if (activityPage) {
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
    }
  };

  const endpointOverlay = endpointDetail
    ? {
        content: (
          <EndpointInvestigation
            detail={endpointDetail}
            key={endpointDetail.header.endpointId}
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
            detail={changeRequestDetail}
            key={changeRequestDetail.summary.changeRequestId}
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
            />
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
