// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";
import type {
  EndpointRemovalRequest,
  EndpointRemovalResult,
} from "./endpointRemoval";

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
  labels: [],
  releaseRef: "release-41",
  reportedAgentVersion: "v2.1.0",
  usernames: ["alice"],
};
const workspace = {
  activity: [],
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

function endpointDetail() {
  return {
    firewall: [],
    firewallTruncated: false,
    header: endpoint,
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

async function openRemoval(
  user: ReturnType<typeof userEvent.setup>,
  removeEndpoint: (
    request: EndpointRemovalRequest,
  ) => Promise<EndpointRemovalResult>,
  loadWorkspace = vi.fn(),
) {
  render(
    <App
      connection={{
        operatorId: "operator-a",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      loadEndpointDetail={vi.fn().mockResolvedValue(endpointDetail())}
      loadWorkspace={loadWorkspace}
      removeEndpoint={removeEndpoint}
      workspace={workspace}
    />,
  );
  await user.click(screen.getByRole("button", { name: "Endpoints" }));
  await user.click(
    screen.getByRole("button", { name: "Inspect endpoint-alpha" }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Endpoint endpoint-alpha",
  });
  await user.click(
    within(dialog).getByRole("button", {
      name: "Remove Endpoint endpoint-alpha",
    }),
  );
  return { dialog, loadWorkspace };
}

describe("Endpoint removal flow", () => {
  it("requires exact case-sensitive identity and removes refreshed inventory after success", async () => {
    const user = userEvent.setup();
    const removeEndpoint = vi
      .fn<
        (
          request: EndpointRemovalRequest,
        ) => Promise<EndpointRemovalResult>
      >()
      .mockResolvedValue({
      affectedEvidence: ["inventory", "activity"],
      credentialStatus: "not_enrolled",
      endpointId: "endpoint-alpha",
      status: "removed",
      });
    const loadWorkspace = vi.fn().mockResolvedValue({
      ...workspace,
      endpoints: [],
      fleets: [{ ...workspace.fleets[0], endpointCount: 0 }],
    });
    const flow = await openRemoval(user, removeEndpoint, loadWorkspace);
    const confirmation = within(flow.dialog).getByRole("group", {
      name: "Confirm Endpoint removal",
    });
    const identity = within(confirmation).getByRole("textbox", {
      name: "Type endpoint-alpha to confirm",
    });
    const submit = within(confirmation).getByRole("button", {
      name: "Remove Endpoint",
    });
    expect(submit).toBeDisabled();
    await user.type(identity, "Endpoint-Alpha");
    expect(submit).toBeDisabled();
    expect(removeEndpoint).not.toHaveBeenCalled();
    await user.clear(identity);
    await user.type(identity, "endpoint-alpha");
    expect(submit).toBeEnabled();
    await user.click(submit);

    expect(removeEndpoint).toHaveBeenCalledOnce();
    expect(removeEndpoint).toHaveBeenCalledWith({
      confirmation: "endpoint-alpha",
      endpointId: "endpoint-alpha",
    });
    expect(loadWorkspace).toHaveBeenCalledOnce();
    expect(
      screen.queryByRole("dialog", { name: "Endpoint endpoint-alpha" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Inspect endpoint-alpha" }),
    ).not.toBeInTheDocument();
    const result = screen.getByRole("status", { name: "Endpoint removed" });
    expect(result).toHaveTextContent("endpoint-alpha");
    expect(result).toHaveTextContent("credential is no longer enrolled");
  });

  it("keeps the Endpoint detail, clears confirmation, and preserves inventory after failure", async () => {
    const user = userEvent.setup();
    const removeEndpoint = vi
      .fn<
        (
          request: EndpointRemovalRequest,
        ) => Promise<EndpointRemovalResult>
      >()
      .mockRejectedValue({
      guidance: "Keep the Endpoint enrolled and retry after checking the server.",
      kind: "connection",
      message: "The server could not remove this Endpoint.",
      retryable: true,
      });
    const flow = await openRemoval(user, removeEndpoint);
    const identity = within(flow.dialog).getByRole("textbox", {
      name: "Type endpoint-alpha to confirm",
    });
    await user.type(identity, "endpoint-alpha");
    await user.click(
      within(flow.dialog).getByRole("button", { name: "Remove Endpoint" }),
    );

    const failure = await within(flow.dialog).findByRole("alert", {
      name: "Endpoint removal failed",
    });
    expect(failure).toHaveTextContent(
      "The server could not remove this Endpoint.",
    );
    expect(identity).toHaveValue("");
    expect(flow.loadWorkspace).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: "Endpoint endpoint-alpha" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Inspect endpoint-alpha" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("status", { name: "Endpoint removed" })).not.toBeInTheDocument();
  });
});
