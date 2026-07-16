// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2032-03-04T05:05:07Z";
const readySection = { snapshot: { loadedAt }, state: "ready" };
const mixedMembers = [
  {
    compliance: "compliant",
    desiredAgentVersion: "v2.0.0",
    endpointId: "endpoint-a",
    evidenceAt: loadedAt,
    fleet: "mixed",
    freshness: "recent",
    labels: [{ key: "region", value: "west" }],
    releaseRef: "release-42",
    reportedAgentVersion: "v2.0.0",
    usernames: ["alice"],
  },
  {
    compliance: "drifted",
    desiredAgentVersion: "v2.0.0",
    endpointId: "endpoint-b",
    evidenceAt: "2032-03-04T04:57:07Z",
    fleet: "mixed",
    freshness: "stale",
    labels: [{ key: "region", value: "east" }],
    releaseRef: "release-41",
    reportedAgentVersion: "v1.9.0",
    usernames: ["bob"],
  },
  {
    compliance: "not_reported",
    desiredAgentVersion: "v2.0.0",
    endpointId: "endpoint-c",
    fleet: "mixed",
    freshness: "never_reported",
    labels: [],
    releaseRef: "",
    reportedAgentVersion: "",
    usernames: [],
  },
];

const mixedSummary = {
  agentVersions: [
    { count: 1, status: "v2.0.0" },
    { count: 1, status: "v1.9.0" },
    { count: 1, status: "not_reported" },
  ],
  compliance: [
    { count: 1, status: "compliant" },
    { count: 1, status: "drifted" },
    { count: 1, status: "not_reported" },
  ],
  endpointCount: 3,
  fleet: "mixed",
  freshness: [
    { count: 1, status: "recent" },
    { count: 1, status: "stale" },
    { count: 1, status: "never_reported" },
  ],
};

const emptySummary = {
  agentVersions: [],
  compliance: [],
  endpointCount: 0,
  fleet: "empty",
  freshness: [],
};

function fleetDetail(fleet: "empty" | "mixed") {
  if (fleet === "empty") {
    return {
      empty: true,
      emptyMessage: "No Endpoints are enrolled in Fleet empty.",
      fleet,
      members: [],
      sections: {
        members: { snapshot: { loadedAt }, state: "empty" },
        state: { snapshot: { loadedAt }, state: "empty" },
      },
      summary: emptySummary,
    };
  }
  return {
    empty: false,
    emptyMessage: "",
    fleet,
    members: mixedMembers,
    sections: {
      members: readySection,
      state: readySection,
    },
    summary: mixedSummary,
  };
}

type FleetDetailView = ReturnType<typeof fleetDetail>;
type FleetDetailLoader = (fleet: string) => Promise<FleetDetailView>;

function renderApp(loadFleetDetail: FleetDetailLoader) {
  render(
    <App
      connection={{
        operatorId: "operator-fleets",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      loadFleetDetail={loadFleetDetail}
      workspace={{
        activity: [],
        changeRequests: [],
        endpoints: mixedMembers,
        fleets: [emptySummary, mixedSummary],
        sections: {
          activity: readySection,
          changeRequests: readySection,
          endpoints: readySection,
          fleets: readySection,
          state: readySection,
        },
      }}
    />,
  );
}

describe("FleetPage", () => {
  it("keeps summary counts equal to members and filters from a distribution", async () => {
    const user = userEvent.setup();
    const loadFleetDetail = vi.fn<FleetDetailLoader>(async (fleet) =>
      fleetDetail(fleet as "empty" | "mixed"),
    );
    renderApp(loadFleetDetail);

    await user.click(screen.getByRole("button", { name: "Fleets" }));
    const inventory = screen.getByRole("region", { name: "Fleet inventory" });
    expect(within(inventory).getByText("empty")).toBeVisible();
    expect(within(inventory).getByText("mixed")).toBeVisible();
    expect(within(inventory).getByText("0 Endpoints")).toBeVisible();
    expect(within(inventory).getByText("3 Endpoints")).toBeVisible();

    await user.click(
      within(inventory).getByRole("button", { name: "Open Fleet mixed" }),
    );
    expect(loadFleetDetail).toHaveBeenCalledWith("mixed");
    const detail = await screen.findByRole("region", {
      name: "Fleet mixed detail",
    });
    expect(within(detail).getByText("3 member Endpoints")).toBeVisible();
    for (const name of [
      "1 compliant member",
      "1 drifted member",
      "1 not reported member",
      "1 recent member",
      "1 stale member",
      "1 never reported member",
    ]) {
      expect(within(detail).getByRole("button", { name })).toBeVisible();
    }
    const agentVersions = within(detail).getByRole("region", {
      name: "Agent versions",
    });
    expect(within(agentVersions).getByText("v2.0.0")).toBeVisible();
    expect(within(agentVersions).getByText("v1.9.0")).toBeVisible();
    expect(within(agentVersions).getByText("Not reported")).toBeVisible();

    const memberTable = within(detail).getByRole("table", {
      name: "Endpoints",
    });
    expect(within(memberTable).getByText("endpoint-a")).toBeVisible();
    expect(within(memberTable).getByText("endpoint-b")).toBeVisible();
    expect(within(memberTable).getByText("endpoint-c")).toBeVisible();

    await user.click(
      within(detail).getByRole("button", { name: "1 compliant member" }),
    );
    expect(within(detail).getByText("1 of 3 Endpoints")).toBeVisible();
    expect(within(memberTable).getByText("endpoint-a")).toBeVisible();
    expect(within(memberTable).queryByText("endpoint-b")).not.toBeInTheDocument();
    expect(within(memberTable).queryByText("endpoint-c")).not.toBeInTheDocument();
    expect(
      within(detail).getByRole("combobox", { name: "Compliance filter" }),
    ).toHaveValue("compliant");
  });

  it("keeps a zero-member Fleet visible and explains its empty detail", async () => {
    const user = userEvent.setup();
    const loadFleetDetail = vi.fn<FleetDetailLoader>(async (fleet) =>
      fleetDetail(fleet as "empty" | "mixed"),
    );
    renderApp(loadFleetDetail);

    await user.click(screen.getByRole("button", { name: "Fleets" }));
    const inventory = screen.getByRole("region", { name: "Fleet inventory" });
    await user.click(
      within(inventory).getByRole("button", { name: "Open Fleet empty" }),
    );

    expect(loadFleetDetail).toHaveBeenCalledWith("empty");
    expect(
      within(inventory).getByRole("button", { name: "Open Fleet empty" }),
    ).toBeVisible();
    const detail = await screen.findByRole("region", {
      name: "Fleet empty detail",
    });
    const empty = within(detail).getByRole("status", {
      name: "No Endpoints enrolled in Fleet empty",
    });
    expect(
      within(empty).getByText("No Endpoints are enrolled in Fleet empty."),
    ).toBeVisible();
    expect(within(detail).queryByRole("table", { name: "Endpoints" })).not.toBeInTheDocument();
  });
});
