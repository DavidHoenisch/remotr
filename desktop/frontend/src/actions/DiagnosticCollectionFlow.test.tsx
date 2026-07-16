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
const capabilities = {
  collectors: [
    "system_info",
    "network_state",
    "journal_remotr",
    "journal_kernel",
    "journal_audit",
    "dmesg",
    "remotr_agent_state",
  ],
  maxTimeSpanSeconds: 7 * 24 * 60 * 60,
};

async function openDiagnostics(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Diagnostics" }));
  return screen.getByRole("region", { name: "Diagnostic collection" });
}

async function enterRequest(
  user: ReturnType<typeof userEvent.setup>,
  surface: HTMLElement,
  since: string,
  until: string,
) {
  await user.selectOptions(
    within(surface).getByRole("combobox", { name: "Diagnostic Endpoint" }),
    "endpoint-alpha",
  );
  await user.clear(
    within(surface).getByRole("textbox", { name: "Since timestamp" }),
  );
  await user.type(
    within(surface).getByRole("textbox", { name: "Since timestamp" }),
    since,
  );
  await user.clear(
    within(surface).getByRole("textbox", { name: "Until timestamp" }),
  );
  await user.type(
    within(surface).getByRole("textbox", { name: "Until timestamp" }),
    until,
  );
}

describe("Diagnostic collection flow", () => {
  it("previews the exact request and blocks empty, invalid, and over-limit intervals", async () => {
    const user = userEvent.setup();
    const requestDiagnosticCollection = vi.fn().mockResolvedValue({
      collectors: ["network_state", "journal_kernel"],
      endpointId: "endpoint-alpha",
      requestId: "diagnostic-42",
      since: "2026-02-25T05:05:07Z",
      status: "pending",
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
        workspace={workspace}
      />,
    );

    const surface = await openDiagnostics(user);
    await enterRequest(
      user,
      surface,
      "2026-03-03T05:05:07Z",
      "2026-03-04T05:05:07Z",
    );
    const review = within(surface).getByRole("button", {
      name: "Review diagnostic collection",
    });

    await user.click(review);
    expect(
      within(
        within(surface).getByRole("group", { name: "Collectors" }),
      ).getByRole("alert"),
    ).toHaveTextContent("Select at least one collector");
    expect(requestDiagnosticCollection).not.toHaveBeenCalled();

    await user.click(
      within(surface).getByRole("checkbox", { name: "network_state" }),
    );
    await user.clear(
      within(surface).getByRole("textbox", { name: "Since timestamp" }),
    );
    await user.type(
      within(surface).getByRole("textbox", { name: "Since timestamp" }),
      "2026-03-04T05:05:07Z",
    );
    await user.click(review);
    expect(
      within(
        within(surface).getByRole("group", { name: "Collection interval" }),
      ).getByRole("alert"),
    ).toHaveTextContent("Until must be after since");
    expect(requestDiagnosticCollection).not.toHaveBeenCalled();

    await user.clear(
      within(surface).getByRole("textbox", { name: "Since timestamp" }),
    );
    await user.type(
      within(surface).getByRole("textbox", { name: "Since timestamp" }),
      "2026-02-25T05:05:06Z",
    );
    await user.click(review);
    expect(
      within(
        within(surface).getByRole("group", { name: "Collection interval" }),
      ).getByRole("alert"),
    ).toHaveTextContent("7 days or less");
    expect(requestDiagnosticCollection).not.toHaveBeenCalled();

    await user.clear(
      within(surface).getByRole("textbox", { name: "Since timestamp" }),
    );
    await user.type(
      within(surface).getByRole("textbox", { name: "Since timestamp" }),
      "2026-02-25T05:05:07Z",
    );
    await user.click(
      within(surface).getByRole("checkbox", { name: "journal_kernel" }),
    );
    await user.click(review);
    expect(requestDiagnosticCollection).not.toHaveBeenCalled();

    const confirmation = within(surface).getByRole("group", {
      name: "Confirm diagnostic collection",
    });
    expect(within(confirmation).getByText("endpoint-alpha")).toBeVisible();
    expect(within(confirmation).getByText("network_state")).toBeVisible();
    expect(within(confirmation).getByText("journal_kernel")).toBeVisible();
    expect(
      within(confirmation).getByText("2026-02-25T05:05:07Z"),
    ).toBeVisible();
    expect(
      within(confirmation).getByText("2026-03-04T05:05:07Z"),
    ).toBeVisible();

    await user.click(
      within(confirmation).getByRole("button", {
        name: "Request diagnostic collection",
      }),
    );
    const result = await within(surface).findByRole("status", {
      name: "Diagnostic collection requested",
    });
    expect(requestDiagnosticCollection).toHaveBeenCalledOnce();
    expect(requestDiagnosticCollection).toHaveBeenCalledWith({
      collectors: ["network_state", "journal_kernel"],
      endpointId: "endpoint-alpha",
      since: "2026-02-25T05:05:07Z",
      until: "2026-03-04T05:05:07Z",
    });
    expect(within(result).getByText("diagnostic-42")).toBeVisible();
    expect(within(result).getByText("Pending")).toBeVisible();
    expect(result).not.toHaveTextContent(/collection completed/i);
  });

  it("presents an active request conflict once and offers the known request", async () => {
    const user = userEvent.setup();
    const requestDiagnosticCollection = vi.fn().mockRejectedValue({
      existingRequestId: "diagnostic-existing",
      guidance: "Inspect the existing request before starting another collection.",
      kind: "conflict",
      message: "This Endpoint already has an active diagnostic request.",
      retryable: false,
    });
    const onInspectDiagnosticRequest = vi.fn();
    render(
      <App
        connection={{
          operatorId: "operator-a",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        diagnosticCapabilities={capabilities}
        onInspectDiagnosticRequest={onInspectDiagnosticRequest}
        requestDiagnosticCollection={requestDiagnosticCollection}
        workspace={workspace}
      />,
    );

    const surface = await openDiagnostics(user);
    await enterRequest(
      user,
      surface,
      "2026-03-03T05:05:07Z",
      "2026-03-04T05:05:07Z",
    );
    await user.click(
      within(surface).getByRole("checkbox", { name: "system_info" }),
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

    const conflict = await within(surface).findByRole("alert", {
      name: "Active diagnostic request exists",
    });
    expect(conflict).toHaveTextContent(
      "This Endpoint already has an active diagnostic request.",
    );
    expect(conflict).toHaveTextContent("diagnostic-existing");
    expect(requestDiagnosticCollection).toHaveBeenCalledOnce();

    await user.click(
      within(conflict).getByRole("button", {
        name: "Inspect diagnostic request diagnostic-existing",
      }),
    );
    expect(onInspectDiagnosticRequest).toHaveBeenCalledOnce();
    expect(onInspectDiagnosticRequest).toHaveBeenCalledWith(
      "diagnostic-existing",
    );
    expect(requestDiagnosticCollection).toHaveBeenCalledOnce();
  });
});
