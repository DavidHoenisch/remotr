// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2032-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const initialLabels = [
  { key: "environment", value: "production" },
  { key: "region", value: "west" },
];
const endpoint = {
  compliance: "compliant",
  desiredAgentVersion: "v2.1.0",
  endpointId: "endpoint-alpha",
  evidenceAt: loadedAt,
  fleet: "production",
  freshness: "recent",
  labels: initialLabels,
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

function endpointDetail(labels: Array<{ key: string; value: string }>) {
  return {
    firewall: [],
    firewallTruncated: false,
    header: { ...endpoint, labels },
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

function activityPage(action: string, eventId: string) {
  return {
    events: [
      {
        action,
        actor: "operator-a",
        details: [],
        eventId,
        occurredAt: loadedAt,
        requestId: `request-${eventId}`,
        resourceId: "endpoint-alpha",
        resourceType: "endpoint",
        status: "completed",
      },
    ],
    nextCursor: "",
    section: ready,
  };
}

function renderLabelFlow(overrides: Record<string, unknown> = {}) {
  const props = {
    connection: {
      operatorId: "operator-a",
      profileName: "Production",
      serverLabel: "remotr.example:8443",
    },
    fleetScope: "All Fleets",
    workspace,
    ...overrides,
  };
  render(<App {...props} />);
}

async function openLabelEditor(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole("button", {
      name: "Manage Labels for endpoint-alpha",
    }),
  );
  return screen.getByRole("dialog", {
    name: "Manage Labels for endpoint-alpha",
  });
}

async function setLabel(
  user: ReturnType<typeof userEvent.setup>,
  dialog: HTMLElement,
  key: string,
  value: string,
) {
  const keyInput = within(dialog).getByRole("textbox", { name: "Label key" });
  const valueInput = within(dialog).getByRole("textbox", {
    name: "Label value",
  });
  await user.clear(keyInput);
  await user.type(keyInput, key);
  await user.clear(valueInput);
  await user.type(valueInput, value);
  await user.click(within(dialog).getByRole("button", { name: "Set Label" }));
}

describe("Endpoint Label user flow", () => {
  it("adds, replaces, and removes only the selected Endpoint Label with targeted evidence refresh", async () => {
    const user = userEvent.setup();
    const labelsAfterAdd = [
      ...initialLabels,
      { key: "site", value: "berlin" },
    ];
    const labelsAfterReplace = labelsAfterAdd.map((label) =>
      label.key === "environment" ? { ...label, value: "staging" } : label,
    );
    const labelsAfterRemove = labelsAfterReplace.filter(
      (label) => label.key !== "region",
    );
    const setEndpointLabel = vi
      .fn()
      .mockResolvedValueOnce({
        effect: "added",
        endpointId: "endpoint-alpha",
        key: "site",
        labels: labelsAfterAdd,
        value: "berlin",
      })
      .mockResolvedValueOnce({
        effect: "replaced",
        endpointId: "endpoint-alpha",
        key: "environment",
        labels: labelsAfterReplace,
        value: "staging",
      });
    const removeEndpointLabel = vi.fn().mockResolvedValue({
      effect: "removed",
      endpointId: "endpoint-alpha",
      key: "region",
      labels: labelsAfterRemove,
      value: "",
    });
    const loadEndpointDetail = vi
      .fn()
      .mockResolvedValueOnce(endpointDetail(labelsAfterAdd))
      .mockResolvedValueOnce(endpointDetail(labelsAfterReplace))
      .mockResolvedValueOnce(endpointDetail(labelsAfterRemove));
    const loadActivityPage = vi
      .fn()
      .mockResolvedValueOnce(activityPage("endpoint_label_added", "event-add"))
      .mockResolvedValueOnce(
        activityPage("endpoint_label_replaced", "event-replace"),
      )
      .mockResolvedValueOnce(
        activityPage("endpoint_label_removed", "event-remove"),
      );
    const loadWorkspace = vi.fn();
    renderLabelFlow({
      loadActivityPage,
      loadEndpointDetail,
      loadWorkspace,
      removeEndpointLabel,
      setEndpointLabel,
    });

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    let dialog = await openLabelEditor(user);
    expect(within(dialog).getByText("endpoint-alpha")).toBeVisible();
    await setLabel(user, dialog, "site", "berlin");
    let result = await within(dialog).findByRole("status", {
      name: "Endpoint Label updated",
    });
    expect(within(result).getByText("Label added")).toBeVisible();
    expect(setEndpointLabel).toHaveBeenLastCalledWith({
      endpointId: "endpoint-alpha",
      key: "site",
      value: "berlin",
    });
    await user.click(within(result).getByRole("button", { name: "Close" }));
    expect(await screen.findByRole("columnheader", { name: "Site" })).toBeVisible();
    expect(screen.getByRole("cell", { name: "berlin" })).toBeVisible();

    dialog = await openLabelEditor(user);
    await setLabel(user, dialog, "environment", "staging");
    result = await within(dialog).findByRole("status", {
      name: "Endpoint Label updated",
    });
    expect(within(result).getByText("Label replaced")).toBeVisible();
    expect(setEndpointLabel).toHaveBeenLastCalledWith({
      endpointId: "endpoint-alpha",
      key: "environment",
      value: "staging",
    });
    await user.click(within(result).getByRole("button", { name: "Close" }));
    expect(screen.getByRole("cell", { name: "staging" })).toBeVisible();

    dialog = await openLabelEditor(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Remove region" }),
    );
    const confirmation = within(dialog).getByRole("group", {
      name: "Remove Label region",
    });
    expect(within(confirmation).getByText("endpoint-alpha")).toBeVisible();
    expect(within(confirmation).getByText("region")).toBeVisible();
    await user.click(
      within(confirmation).getByRole("button", {
        name: "Remove Label region",
      }),
    );
    result = await within(dialog).findByRole("status", {
      name: "Endpoint Label updated",
    });
    expect(within(result).getByText("Label removed")).toBeVisible();
    expect(removeEndpointLabel).toHaveBeenCalledOnce();
    expect(removeEndpointLabel).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      key: "region",
    });
    await user.click(within(result).getByRole("button", { name: "Close" }));

    expect(screen.getByRole("cell", { name: "staging" })).toBeVisible();
    expect(screen.getByRole("cell", { name: "berlin" })).toBeVisible();
    expect(
      screen.queryByRole("columnheader", { name: "Region" }),
    ).not.toBeInTheDocument();
    expect(loadEndpointDetail).toHaveBeenCalledTimes(3);
    expect(loadEndpointDetail).toHaveBeenCalledWith("endpoint-alpha");
    expect(loadActivityPage).toHaveBeenCalledTimes(3);
    expect(loadWorkspace).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Activity" }));
    expect(screen.getByText("Endpoint label removed")).toBeVisible();
    expect(screen.getByText("endpoint-alpha")).toBeVisible();
  });

  it("matches every current Label validation boundary before submission", async () => {
    const user = userEvent.setup();
    const setEndpointLabel = vi.fn();
    renderLabelFlow({
      removeEndpointLabel: vi.fn(),
      setEndpointLabel,
    });

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    const dialog = await openLabelEditor(user);
    const keyInput = within(dialog).getByRole("textbox", { name: "Label key" });
    const valueInput = within(dialog).getByRole("textbox", {
      name: "Label value",
    });
    const submit = within(dialog).getByRole("button", { name: "Set Label" });

    for (const invalidKey of [
      ".hidden",
      "bad key",
      "bad=key",
      "bad\tkey",
      "k".repeat(65),
    ]) {
      fireEvent.change(keyInput, { target: { value: invalidKey } });
      fireEvent.change(valueInput, { target: { value: "valid" } });
      expect(submit).toBeDisabled();
    }
    fireEvent.change(keyInput, { target: { value: "valid" } });
    fireEvent.change(valueInput, { target: { value: "v".repeat(513) } });
    expect(submit).toBeDisabled();
    expect(setEndpointLabel).not.toHaveBeenCalled();

    fireEvent.change(keyInput, { target: { value: "k".repeat(64) } });
    fireEvent.change(valueInput, { target: { value: "v".repeat(512) } });
    expect(submit).toBeEnabled();
  });
});
