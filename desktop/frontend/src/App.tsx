import type { AppPage } from "./shell/AppShell";
import { AppShell } from "./shell/AppShell";

const pageLabels: Record<AppPage, string> = {
  overview: "Overview",
  endpoints: "Endpoints",
  fleets: "Fleets",
  "change-requests": "Change requests",
  diagnostics: "Diagnostics",
  activity: "Activity",
};

export function App() {
  return (
    <AppShell
      connection={{
        connected: false,
        operatorId: "No operator",
        profileName: "No profile selected",
        serverLabel: "Select a profile to begin",
      }}
      fleetScope="All Fleets"
      renderPage={(page) => (
        <section
          aria-label={`${pageLabels[page]} workspace`}
          className="shell-start-state"
        >
          <span className="page-kicker">Workspace ready</span>
          <h2>{pageLabels[page]}</h2>
          <p>Connect an operator profile to load live fleet data.</p>
        </section>
      )}
    />
  );
}
