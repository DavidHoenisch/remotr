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

const loadedAt = "2026-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const workspace = {
  activity: [],
  changeRequests: [],
  endpoints: [
    {
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
    },
  ],
  fleets: [],
  sections: {
    activity: ready,
    changeRequests: ready,
    endpoints: ready,
    fleets: ready,
    state: ready,
  },
};
const capabilities = {
  collectors: ["system_info"],
  maxTimeSpanSeconds: 604800,
};

async function requestCollection(
  user: ReturnType<typeof userEvent.setup>,
  status: string,
  saveDiagnosticBundle = vi.fn(),
) {
  const requestDiagnosticCollection = vi.fn().mockResolvedValue({
    collectors: ["system_info"],
    endpointId: "endpoint-alpha",
    requestId: `diagnostic-${status}`,
    since: "2026-03-03T05:05:07Z",
    status,
    until: "2026-03-04T05:05:07Z",
  });
  render(
    <App
      connection={{
        operatorId: "operator-a",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      diagnosticCapabilities={capabilities}
      requestDiagnosticCollection={requestDiagnosticCollection}
      saveDiagnosticBundle={saveDiagnosticBundle}
      workspace={workspace}
    />,
  );
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
  await within(surface).findByRole("status", {
    name: "Diagnostic collection requested",
  });
  return { saveDiagnosticBundle, surface };
}

describe("Diagnostic bundle native save flow", () => {
  it("saves a ready request through the native bridge and displays metadata only", async () => {
    const user = userEvent.setup();
    const saveDiagnosticBundle = vi.fn().mockResolvedValue({
      path: "/home/operator/Downloads/diagnostic-ready.tar.gz",
      sizeBytes: 4096,
      status: "saved",
    });
    const flow = await requestCollection(user, "ready", saveDiagnosticBundle);

    expect(saveDiagnosticBundle).not.toHaveBeenCalled();
    await user.click(
      within(flow.surface).getByRole("button", {
        name: "Save ready diagnostic bundle",
      }),
    );
    const saved = await within(flow.surface).findByRole("status", {
      name: "Diagnostic bundle saved",
    });
    expect(saveDiagnosticBundle).toHaveBeenCalledOnce();
    expect(saveDiagnosticBundle).toHaveBeenCalledWith("diagnostic-ready");
    expect(saved).toHaveTextContent(
      "/home/operator/Downloads/diagnostic-ready.tar.gz",
    );
    expect(saved).toHaveTextContent("4,096 bytes");
    expect(saved).not.toHaveTextContent("diagnostic-bundle-byte-canary");
  });

  it("does not offer native save while the request is pending", async () => {
    const user = userEvent.setup();
    const flow = await requestCollection(user, "pending");

    expect(
      within(flow.surface).queryByRole("button", {
        name: "Save ready diagnostic bundle",
      }),
    ).not.toBeInTheDocument();
    expect(flow.saveDiagnosticBundle).not.toHaveBeenCalled();
  });
});
