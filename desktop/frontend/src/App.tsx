import { useState } from "react";

import { EndpointTable } from "./endpoints/EndpointTable";
import {
  Overview,
  type OverviewNavigationTarget,
  type OverviewWorkspace,
} from "./overview/Overview";
import { AppShell } from "./shell/AppShell";
import type { AppPage } from "./shell/AppShell";

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
  onCreateEnrollmentToken?: () => void;
  onOpenEndpoint?: (endpointId: string) => void;
  workspace?: OverviewWorkspace;
}

function filterSummary(filters: OverviewNavigationTarget["filters"]): string {
  return Object.entries(filters)
    .map(([name, values]) => `${name}: ${values.join(", ")}`)
    .join(" · ");
}

export function App({
  connection,
  fleetScope = "All Fleets",
  onCreateEnrollmentToken,
  onOpenEndpoint,
  workspace,
}: AppProps) {
  const [activePage, setActivePage] = useState<AppPage>("overview");
  const [activeFilters, setActiveFilters] = useState<
    OverviewNavigationTarget["filters"]
  >({});

  const navigateFromOverview = (target: OverviewNavigationTarget) => {
    setActiveFilters(target.filters);
    setActivePage(target.page);
  };

  const selectNavigationPage = (page: AppPage) => {
    setActiveFilters({});
    setActivePage(page);
  };

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
      renderPage={(page) => {
        if (page === "overview" && workspace) {
          return (
            <Overview
              onNavigate={navigateFromOverview}
              workspace={workspace}
            />
          );
        }

        if (
          page === "endpoints" &&
          workspace &&
          Object.keys(activeFilters).length === 0
        ) {
          return (
            <EndpointTable
              endpoints={workspace.endpoints}
              labelColumns={["environment", "region"]}
              onCreateEnrollmentToken={onCreateEnrollmentToken}
              onOpenEndpoint={onOpenEndpoint}
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
