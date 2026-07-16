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
const ready = { snapshot: { loadedAt }, state: "ready" };
const production = {
  agentVersions: [
    { count: 5, status: "v2.0.0" },
    { count: 2, status: "v1.9.0" },
  ],
  compliance: [{ count: 7, status: "compliant" }],
  endpointCount: 7,
  fleet: "production",
  freshness: [{ count: 7, status: "recent" }],
};
const workspace = {
  activity: [],
  changeRequests: [],
  endpoints: [],
  fleets: [production],
  sections: {
    activity: ready,
    changeRequests: ready,
    endpoints: ready,
    fleets: ready,
    state: ready,
  },
};

describe("Fleet agent upgrade flow", () => {
  it("confirms exact Fleet/version/current count and reports only the server accepted count", async () => {
    const user = userEvent.setup();
    const requestFleetAgentUpgrade = vi.fn().mockResolvedValue({
      acceptedEndpoints: 3,
      fleet: "production",
      status: "requested",
      version: "v2.2.0",
    });
    render(
      <App
        connection={{
          operatorId: "operator-a",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        requestFleetAgentUpgrade={requestFleetAgentUpgrade}
        workspace={workspace}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Fleets" }));
    await user.click(
      screen.getByRole("button", {
        name: "Request agent upgrade for Fleet production",
      }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Request agent upgrade for Fleet production",
    });
    const version = within(dialog).getByRole("textbox", {
      name: "Requested agent version",
    });
    const review = within(dialog).getByRole("button", {
      name: "Review Fleet upgrade request",
    });
    expect(review).toBeDisabled();
    await user.type(version, "v2.2.0");
    await user.click(review);
    expect(requestFleetAgentUpgrade).not.toHaveBeenCalled();

    const confirmation = within(dialog).getByRole("group", {
      name: "Confirm Fleet agent upgrade",
    });
    expect(within(confirmation).getByText("production")).toBeVisible();
    expect(within(confirmation).getByText("v2.2.0")).toBeVisible();
    expect(within(confirmation).getByText("7 member Endpoints")).toBeVisible();
    await user.click(
      within(confirmation).getByRole("button", { name: "Request upgrade" }),
    );

    const result = await within(dialog).findByRole("status", {
      name: "Fleet upgrade requested",
    });
    expect(requestFleetAgentUpgrade).toHaveBeenCalledOnce();
    expect(requestFleetAgentUpgrade).toHaveBeenCalledWith({
      fleet: "production",
      version: "v2.2.0",
    });
    expect(within(result).getByText("Requested")).toBeVisible();
    expect(
      within(result).getByText("3 Endpoints accepted by server"),
    ).toBeVisible();
    expect(result).not.toHaveTextContent("7 Endpoints accepted by server");
    expect(result).not.toHaveTextContent(/completed the upgrade/i);
    expect(result).toHaveTextContent(/later Sync/i);
  });
});
