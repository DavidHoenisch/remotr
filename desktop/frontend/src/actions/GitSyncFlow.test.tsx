// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";
import type { ActionAcknowledgement } from "./useActionController";

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
  reportedAgentVersion: "v2.1.0",
  usernames: ["alice"],
};
const workspace = {
  activity: [
    {
      action: "endpoint_sync",
      actor: "endpoint-alpha",
      details: [],
      eventId: "event-1",
      occurredAt: loadedAt,
      requestId: "request-1",
      resourceId: "endpoint-alpha",
      resourceType: "endpoint",
      status: "completed",
    },
  ],
  changeRequests: [
    {
      approvalCount: 0,
      changeRequestId: "change-1",
      fleet: "production",
      lifecycle: "pending",
      releaseRef: "release-41",
      requiredApprovals: 1,
      risk: "normal",
      targetCount: 1,
      updatedAt: loadedAt,
    },
  ],
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
const refreshedWorkspace = {
  ...workspace,
  activity: [
    {
      action: "git_sync",
      actor: "operator-a",
      details: [{ key: "release_ref", value: "release-42" }],
      eventId: "event-2",
      occurredAt: "2032-03-04T05:06:07Z",
      requestId: "request-204",
      resourceId: "config-repo",
      resourceType: "git",
      status: "accepted",
    },
    ...workspace.activity,
  ],
  changeRequests: workspace.changeRequests.map((change) => ({
    ...change,
    releaseRef: "release-42",
  })),
  endpoints: [{ ...endpoint, releaseRef: "release-42" }],
};
const accepted: ActionAcknowledgement = {
  acceptedAt: "2032-03-04T05:06:07Z",
  action: "git_sync",
  affectedEvidence: ["release_ref", "activity"],
  requestId: "request-204",
  summary: "Server accepted Git sync for the Production profile.",
  target: "config-repo",
};

function renderGitSync(
  requestGitSync: () => Promise<ActionAcknowledgement>,
  loadWorkspace = vi.fn().mockResolvedValue(refreshedWorkspace),
) {
  render(
    <App
      connection={{
        operatorId: "operator-a",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      fleetScope="All Fleets"
      loadWorkspace={loadWorkspace}
      requestGitSync={requestGitSync}
      workspace={workspace}
    />,
  );
  return loadWorkspace;
}

describe("Git sync user flow", () => {
  it("cancels without a request, then shows exact acceptance and refreshed evidence", async () => {
    const user = userEvent.setup();
    let resolveSync!: (result: ActionAcknowledgement) => void;
    const requestGitSync = vi.fn(
      () =>
        new Promise<ActionAcknowledgement>((resolve) => {
          resolveSync = resolve;
        }),
    );
    const loadWorkspace = renderGitSync(requestGitSync);

    await user.click(screen.getByRole("button", { name: "Sync from Git" }));
    let dialog = screen.getByRole("dialog", { name: "Sync from Git" });
    expect(within(dialog).getByText("Production")).toBeVisible();
    expect(within(dialog).getByText("remotr.example:8443")).toBeVisible();
    expect(within(dialog).getByText("release-41")).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(requestGitSync).not.toHaveBeenCalled();
    expect(loadWorkspace).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog", { name: "Sync from Git" })).not.toBeInTheDocument();
    expect(screen.getByText("release-41")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Sync from Git" }));
    dialog = screen.getByRole("dialog", { name: "Sync from Git" });
    const confirm = within(dialog).getByRole("button", {
      name: "Request Git sync",
    });
    fireEvent.click(confirm);
    fireEvent.click(confirm);
    expect(requestGitSync).toHaveBeenCalledOnce();
    expect(
      within(dialog).getByRole("button", { name: "Requesting Git sync" }),
    ).toBeDisabled();

    await act(async () => resolveSync(accepted));
    const result = await within(dialog).findByRole("status", {
      name: "Git sync accepted",
    });
    expect(
      within(result).getByText(
        "Server accepted Git sync for the Production profile.",
      ),
    ).toBeVisible();
    expect(within(result).getByText("Request request-204")).toBeVisible();
    expect(result).toHaveTextContent("Awaiting refreshed server evidence");
    expect(result).not.toHaveTextContent(/release ref advanced|converged/i);
    expect(loadWorkspace).toHaveBeenCalledOnce();
    expect(await screen.findByText("release-42")).toBeVisible();
    expect(screen.getByText("Git sync")).toBeVisible();
  });

  it("retains the active profile and observed Release ref on failure with retry", async () => {
    const user = userEvent.setup();
    const unsafeCanary = "git-sync-server-body-canary";
    const requestGitSync = vi
      .fn<() => Promise<ActionAcknowledgement>>()
      .mockRejectedValueOnce({
        debugContext: unsafeCanary,
        guidance: "Check authorization and server connectivity, then retry.",
        kind: "authorization",
        message: "The server rejected this Git sync request.",
        retryable: true,
      })
      .mockResolvedValueOnce(accepted);
    const loadWorkspace = renderGitSync(requestGitSync);

    await user.click(screen.getByRole("button", { name: "Sync from Git" }));
    const dialog = screen.getByRole("dialog", { name: "Sync from Git" });
    await user.click(
      within(dialog).getByRole("button", { name: "Request Git sync" }),
    );

    const failure = await within(dialog).findByRole("alert", {
      name: "Git sync failed",
    });
    expect(within(failure).getByText("Production")).toBeVisible();
    expect(within(failure).getByText("release-41")).toBeVisible();
    expect(within(failure).getByText(loadedAt)).toBeVisible();
    expect(screen.queryByText(unsafeCanary)).not.toBeInTheDocument();
    expect(loadWorkspace).not.toHaveBeenCalled();
    expect(
      within(screen.getByRole("main")).getByText("release-41"),
    ).toBeVisible();

    await user.click(
      within(failure).getByRole("button", { name: "Retry Git sync" }),
    );
    expect(requestGitSync).toHaveBeenCalledTimes(2);
    expect(
      await within(dialog).findByRole("status", {
        name: "Git sync accepted",
      }),
    ).toBeVisible();
    expect(loadWorkspace).toHaveBeenCalledOnce();
  });
});
