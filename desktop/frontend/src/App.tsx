import { useRef, useState } from "react";

import { ActivityDetail } from "./activity/ActivityDetail";
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

const pageLabels: Record<AppPage, string> = {
  overview: "Overview",
  endpoints: "Endpoints",
  fleets: "Fleets",
  "change-requests": "Change requests",
  diagnostics: "Diagnostics",
  activity: "Activity",
};

interface ConnectedContext {
  operatorId: string;
  profileName: string;
  serverLabel: string;
}

interface AppProps {
  connection?: ConnectedContext;
  fleetScope?: string;
  loadActivityPage?: (request: ActivityPageRequest) => Promise<ActivityPageView>;
  loadChangeRequestDetail?: (
    changeRequestId: string,
  ) => Promise<ChangeRequestDetailView>;
  loadEndpointDetail?: (endpointId: string) => Promise<EndpointDetailView>;
  loadFleetDetail?: (fleet: string) => Promise<FleetDetailView>;
  loadWorkspace?: () => Promise<OverviewWorkspace>;
  onCreateEnrollmentToken?: () => void;
  onOpenEndpoint?: (endpointId: string) => void;
  refreshClock?: RefreshClock;
  workspace?: OverviewWorkspace;
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
  connection,
  fleetScope = "All Fleets",
  loadActivityPage,
  loadChangeRequestDetail,
  loadEndpointDetail,
  loadFleetDetail,
  loadWorkspace,
  onCreateEnrollmentToken,
  onOpenEndpoint,
  refreshClock,
  workspace: suppliedWorkspace,
  workspaceVisibility,
}: AppProps) {
  const {
    failure: workspaceRefreshFailure,
    refresh: refreshWorkspace,
    refreshing: workspaceRefreshing,
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
  const endpointDetailGeneration = useRef(0);
  const endpointDetailOrigin = useRef<HTMLElement | null>(null);
  const changeRequestDetailGeneration = useRef(0);
  const changeRequestDetailOrigin = useRef<HTMLElement | null>(null);
  const activityDetailOrigin = useRef<HTMLElement | null>(null);

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

  return (
    <AppShell
      activePage={activePage}
      connection={{
        connected: connection !== undefined,
        operatorId: connection?.operatorId ?? "No operator",
        profileName: connection?.profileName ?? "No profile selected",
        serverLabel: connection?.serverLabel ?? "Select a profile to begin",
      }}
      fleetScope={fleetScope}
      onPageChange={selectNavigationPage}
      onRefresh={loadWorkspace ? refreshWorkspace : undefined}
      overlay={endpointOverlay ?? changeRequestOverlay ?? activityOverlay}
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
              labelColumns={["environment", "region"]}
              onCreateEnrollmentToken={onCreateEnrollmentToken}
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
