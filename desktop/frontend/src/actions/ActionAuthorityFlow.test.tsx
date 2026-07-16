// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2026-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const endpoint = {
  compliance: "compliant",
  desiredAgentVersion: "v2.1.0",
  endpointId: "endpoint-alpha",
  evidenceAt: loadedAt,
  fleet: "production",
  freshness: "recent",
  labels: [],
  releaseRef: "release-41",
  reportedAgentVersion: "v2.1.0",
  usernames: ["alice"],
};
const initialEvent = {
  action: "endpoint_sync",
  actor: "endpoint-alpha",
  details: [],
  eventId: "event-before",
  occurredAt: loadedAt,
  requestId: "request-before",
  resourceId: "endpoint-alpha",
  resourceType: "endpoint",
  status: "completed",
};
const fleetAudit = {
  action: "fleet_agent_upgrade_requested",
  actor: "operator-authoritative",
  details: [{ key: "version", value: "v2.2.0" }],
  eventId: "event-fleet-upgrade",
  occurredAt: "2026-03-04T05:06:07Z",
  requestId: "request-fleet-audit",
  resourceId: "production",
  resourceType: "fleet",
  status: "completed",
};
const diagnosticAudit = {
  action: "admin_diagnostics_collect",
  actor: "operator-authoritative",
  details: [{ key: "request_id", value: "diagnostic-42" }],
  eventId: "event-diagnostic",
  occurredAt: "2026-03-04T05:07:07Z",
  requestId: "request-diagnostic-audit",
  resourceId: "endpoint-alpha",
  resourceType: "endpoint",
  status: "completed",
};
const workspace = {
  activity: [initialEvent],
  changeRequests: [],
  endpoints: [endpoint],
  fleets: [
    {
      agentVersions: [{ count: 1, status: "v2.1.0" }],
      compliance: [{ count: 1, status: "compliant" }],
      endpointCount: 1,
      fleet: "production",
      freshness: [{ count: 1, status: "recent" }],
    },
  ],
  sections: {
    activity: ready,
    changeRequests: ready,
    endpoints: ready,
    fleets: ready,
    state: ready,
  },
};
const emptyActivityRequest = {
  action: "",
  actorType: "",
  cursor: "",
  seenEventIds: [],
  since: "",
  until: "",
};

function renderAuthorityFlow(overrides: Record<string, unknown>) {
  render(
    <App
      connection={{
        operatorId: "operator-authoritative",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      diagnosticCapabilities={{
        collectors: ["system_info"],
        maxTimeSpanSeconds: 604800,
      }}
      workspace={workspace}
      {...overrides}
    />,
  );
}

async function submitFleetUpgrade(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Fleets" }));
  await user.click(
    screen.getByRole("button", {
      name: "Request agent upgrade for Fleet production",
    }),
  );
  const dialog = screen.getByRole("dialog", {
    name: "Request agent upgrade for Fleet production",
  });
  await user.type(
    within(dialog).getByRole("textbox", {
      name: "Requested agent version",
    }),
    "v2.2.0",
  );
  await user.click(
    within(dialog).getByRole("button", {
      name: "Review Fleet upgrade request",
    }),
  );
  await user.click(
    within(dialog).getByRole("button", { name: "Request upgrade" }),
  );
  return dialog;
}

async function submitDiagnosticCollection(
  user: ReturnType<typeof userEvent.setup>,
) {
  await user.click(screen.getByRole("button", { name: "Diagnostics" }));
  const surface = screen.getByRole("region", {
    name: "Diagnostic collection",
  });
  await user.selectOptions(
    within(surface).getByRole("combobox", { name: "Diagnostic Endpoint" }),
    "endpoint-alpha",
  );
  await user.click(
    within(surface).getByRole("checkbox", { name: "system_info" }),
  );
  await user.type(
    within(surface).getByRole("textbox", { name: "Since timestamp" }),
    "2026-03-03T05:05:07Z",
  );
  await user.type(
    within(surface).getByRole("textbox", { name: "Until timestamp" }),
    "2026-03-04T05:05:07Z",
  );
  await user.click(
    within(surface).getByRole("button", {
      name: "Review diagnostic collection",
    }),
  );
  await user.click(
    within(surface).getByRole("button", {
      name: "Request diagnostic collection",
    }),
  );
  return surface;
}

describe("Action authority and Activity refresh", () => {
  it("keeps workspace evidence unchanged when distinct actions are forbidden", async () => {
    const user = userEvent.setup();
    const forbidden = {
      guidance: "Ask an administrator to review the Operator roles.",
      kind: "authorization",
      message: "The current Operator is not authorized for this action.",
      retryable: false,
    };
    const requestFleetAgentUpgrade = vi.fn().mockRejectedValue(forbidden);
    const requestDiagnosticCollection = vi.fn().mockRejectedValue(forbidden);
    const loadActivityPage = vi.fn();
    renderAuthorityFlow({
      loadActivityPage,
      requestDiagnosticCollection,
      requestFleetAgentUpgrade,
    });

    const fleetDialog = await submitFleetUpgrade(user);
    expect(
      await within(fleetDialog).findByRole("alert", {
        name: "Fleet upgrade request failed",
      }),
    ).toHaveTextContent("not authorized");
    expect(loadActivityPage).not.toHaveBeenCalled();
    expect(screen.getByText("1 Endpoint")).toBeVisible();
    await user.click(
      within(fleetDialog).getByRole("button", {
        name: "Close Request agent upgrade for Fleet production",
      }),
    );

    const diagnostics = await submitDiagnosticCollection(user);
    expect(
      await within(diagnostics).findByRole("alert", {
        name: "Diagnostic collection request failed",
      }),
    ).toHaveTextContent("not authorized");
    expect(loadActivityPage).not.toHaveBeenCalled();
    expect(requestFleetAgentUpgrade).toHaveBeenCalledOnce();
    expect(requestDiagnosticCollection).toHaveBeenCalledOnce();

    await user.click(screen.getByRole("button", { name: "Activity" }));
    const table = screen.getByRole("table", { name: "Activity" });
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    expect(within(table).getByText("event-before")).toBeVisible();
  });

  it("renders exact server audit rows after successful actions without client-authored duplicates", async () => {
    const user = userEvent.setup();
    const loadActivityPage = vi
      .fn()
      .mockResolvedValueOnce({
        events: [fleetAudit, initialEvent],
        nextCursor: "",
        section: ready,
      })
      .mockResolvedValueOnce({
        events: [diagnosticAudit, fleetAudit, initialEvent],
        nextCursor: "",
        section: ready,
      });
    renderAuthorityFlow({
      loadActivityPage,
      requestDiagnosticCollection: vi.fn().mockResolvedValue({
        collectors: ["system_info"],
        endpointId: "endpoint-alpha",
        requestId: "diagnostic-42",
        since: "2026-03-03T05:05:07Z",
        status: "pending",
        until: "2026-03-04T05:05:07Z",
      }),
      requestFleetAgentUpgrade: vi.fn().mockResolvedValue({
        acceptedEndpoints: 1,
        fleet: "production",
        status: "requested",
        version: "v2.2.0",
      }),
    });

    const fleetDialog = await submitFleetUpgrade(user);
    expect(
      await within(fleetDialog).findByRole("status", {
        name: "Fleet upgrade requested",
      }),
    ).toBeVisible();
    await waitFor(() => expect(loadActivityPage).toHaveBeenCalledTimes(1));
    expect(loadActivityPage).toHaveBeenNthCalledWith(1, emptyActivityRequest);
    await user.click(
      within(fleetDialog).getByRole("button", {
        name: "Close Request agent upgrade for Fleet production",
      }),
    );

    const diagnostics = await submitDiagnosticCollection(user);
    expect(
      await within(diagnostics).findByRole("status", {
        name: "Diagnostic collection requested",
      }),
    ).toBeVisible();
    await waitFor(() => expect(loadActivityPage).toHaveBeenCalledTimes(2));
    expect(loadActivityPage).toHaveBeenNthCalledWith(2, emptyActivityRequest);

    await user.click(screen.getByRole("button", { name: "Activity" }));
    const table = screen.getByRole("table", { name: "Activity" });
    const rows = within(table).getAllByRole("row");
    expect(rows).toHaveLength(4);
    for (const event of [diagnosticAudit, fleetAudit, initialEvent]) {
      const row = within(
        within(table).getByRole("cell", { name: event.eventId }).closest("tr")!,
      );
      for (const value of [
        event.actor,
        event.action,
        event.resourceId,
        event.resourceType,
        event.requestId,
        event.occurredAt,
        event.status,
      ]) {
        expect(row.getAllByText(value)[0]).toBeVisible();
      }
    }
    expect(screen.queryByText(/client-authored/i)).not.toBeInTheDocument();
  });
});
