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
const endpoint = {
  compliance: "compliant",
  desiredAgentVersion: "v2.1.0",
  endpointId: "endpoint-alpha",
  evidenceAt: loadedAt,
  fleet: "production",
  freshness: "recent",
  labels: [{ key: "environment", value: "production" }],
  releaseRef: "release-41",
  reportedAgentVersion: "v2.0.0",
  usernames: ["alice"],
};
const workspace = {
  activity: [],
  changeRequests: [],
  endpoints: [endpoint],
  fleets: [
    {
      agentVersions: [{ count: 1, status: "v2.0.0" }],
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

function endpointDetail() {
  return {
    firewall: [],
    firewallTruncated: false,
    header: {
      ...endpoint,
      desiredAgentVersion: "v2.2.0",
      reportedAgentVersion: "v2.0.0",
    },
    schedules: [],
    schedulesTruncated: false,
    sections: {
      firewall: ready,
      overview: ready,
      schedules: ready,
      state: ready,
      system: ready,
    },
    state: {
      digest: "digest-alpha",
      endpointId: "endpoint-alpha",
      items: [],
      releaseRef: "release-41",
      reportedAt: loadedAt,
      status: "compliant",
    },
    stateTruncated: false,
    system: {
      cpu: "Test CPU",
      cpuCores: "4",
      digest: "system-alpha",
      hostname: "endpoint-alpha",
      kernel: "6.12.8",
      memory: "8 GiB",
      os: "Debian GNU/Linux 13",
      reportedAt: loadedAt,
    },
  };
}

describe("Endpoint agent upgrade flow", () => {
  it("submits one exact target/version and keeps request, desired, reported, and completion states distinct", async () => {
    const user = userEvent.setup();
    const requestEndpointAgentUpgrade = vi.fn().mockResolvedValue({
      affectedEvidence: [
        "desired_agent_version",
        "reported_agent_version",
        "activity",
      ],
      endpointId: "endpoint-alpha",
      status: "requested",
      version: "v2.2.0",
    });
    const loadEndpointDetail = vi.fn().mockResolvedValue(endpointDetail());
    const loadActivityPage = vi.fn().mockResolvedValue({
      events: [
        {
          action: "endpoint_agent_upgrade_requested",
          actor: "operator-a",
          details: [{ key: "version", value: "v2.2.0" }],
          eventId: "event-upgrade",
          occurredAt: loadedAt,
          requestId: "request-upgrade",
          resourceId: "endpoint-alpha",
          resourceType: "endpoint",
          status: "completed",
        },
      ],
      nextCursor: "",
      section: ready,
    });
    const loadWorkspace = vi.fn();
    render(
      <App
        connection={{
          operatorId: "operator-a",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        loadActivityPage={loadActivityPage}
        loadEndpointDetail={loadEndpointDetail}
        loadWorkspace={loadWorkspace}
        requestEndpointAgentUpgrade={requestEndpointAgentUpgrade}
        workspace={workspace}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    await user.click(
      screen.getByRole("button", {
        name: "Request agent upgrade for endpoint-alpha",
      }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Request agent upgrade for endpoint-alpha",
    });
    expect(within(dialog).getByText("endpoint-alpha")).toBeVisible();
    expect(within(dialog).getByText("v2.1.0")).toBeVisible();
    expect(within(dialog).getByText("v2.0.0")).toBeVisible();

    const version = within(dialog).getByRole("textbox", {
      name: "Requested agent version",
    });
    const review = within(dialog).getByRole("button", {
      name: "Review upgrade request",
    });
    expect(review).toBeDisabled();
    await user.type(version, "v2.2.0");
    expect(review).toBeEnabled();
    await user.click(review);
    expect(requestEndpointAgentUpgrade).not.toHaveBeenCalled();

    const confirmation = within(dialog).getByRole("group", {
      name: "Confirm Endpoint agent upgrade",
    });
    expect(within(confirmation).getByText("endpoint-alpha")).toBeVisible();
    expect(within(confirmation).getByText("v2.2.0")).toBeVisible();
    await user.click(
      within(confirmation).getByRole("button", { name: "Request upgrade" }),
    );

    const result = await within(dialog).findByRole("status", {
      name: "Endpoint upgrade requested",
    });
    expect(requestEndpointAgentUpgrade).toHaveBeenCalledOnce();
    expect(requestEndpointAgentUpgrade).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      version: "v2.2.0",
    });
    const requestState = within(result).getByRole("group", {
      name: "Upgrade state evidence",
    });
    const requested = within(requestState).getByRole("group", {
      name: "Request state",
    });
    expect(within(requested).getByText("Requested")).toBeVisible();
    expect(within(requested).getByText("v2.2.0")).toBeVisible();
    const desired = within(requestState).getByRole("group", {
      name: "Desired version evidence",
    });
    expect(within(desired).getByText("v2.2.0")).toBeVisible();
    const reported = within(requestState).getByRole("group", {
      name: "Reported version evidence",
    });
    expect(within(reported).getByText("v2.0.0")).toBeVisible();
    const completion = within(requestState).getByRole("group", {
      name: "Completion state",
    });
    expect(within(completion).getByText("Not completed")).toBeVisible();
    expect(result).toHaveTextContent(/later Sync/i);
    expect(result).not.toHaveTextContent("Upgrade completed");

    expect(loadEndpointDetail).toHaveBeenCalledOnce();
    expect(loadEndpointDetail).toHaveBeenCalledWith("endpoint-alpha");
    expect(loadActivityPage).toHaveBeenCalledOnce();
    expect(loadWorkspace).not.toHaveBeenCalled();
  });
});
