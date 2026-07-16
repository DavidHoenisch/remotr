import { useRef, useState } from "react";

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
  type OverviewWorkspace,
} from "./overview/Overview";
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
  loadEndpointDetail?: (endpointId: string) => Promise<EndpointDetailView>;
  loadFleetDetail?: (fleet: string) => Promise<FleetDetailView>;
  onCreateEnrollmentToken?: () => void;
  onOpenEndpoint?: (endpointId: string) => void;
  workspace?: OverviewWorkspace;
}

interface EndpointDetailFailure {
  endpointId: string;
  guidance: string;
  message: string;
}

function filterSummary(filters: OverviewNavigationTarget["filters"]): string {
  return Object.entries(filters)
    .map(([name, values]) => `${name}: ${values.join(", ")}`)
    .join(" · ");
}

export function App({
  connection,
  fleetScope = "All Fleets",
  loadEndpointDetail,
  loadFleetDetail,
  onCreateEnrollmentToken,
  onOpenEndpoint,
  workspace,
}: AppProps) {
  const [activePage, setActivePage] = useState<AppPage>("overview");
  const [activeFilters, setActiveFilters] = useState<
    OverviewNavigationTarget["filters"]
  >({});
  const [endpointDetail, setEndpointDetail] = useState<EndpointDetailView>();
  const [endpointDetailFailure, setEndpointDetailFailure] =
    useState<EndpointDetailFailure>();
  const endpointDetailGeneration = useRef(0);
  const endpointDetailOrigin = useRef<HTMLElement | null>(null);

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
      overlay={endpointOverlay}
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
