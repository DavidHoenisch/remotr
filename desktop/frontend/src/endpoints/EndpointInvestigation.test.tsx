// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import {
  act,
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

function readySection() {
  return { snapshot: { loadedAt }, state: "ready" };
}

const endpointRows = [
  {
    compliance: "compliant",
    desiredAgentVersion: "v2.1.0",
    endpointId: "endpoint-alpha",
    evidenceAt: loadedAt,
    fleet: "production",
    freshness: "recent",
    labels: [
      { key: "owner", value: "platform" },
      { key: "region", value: "west" },
    ],
    releaseRef: "release-42",
    reportedAgentVersion: "v2.0.0",
    usernames: ["alice"],
  },
  {
    compliance: "drifted",
    desiredAgentVersion: "v2.1.0",
    endpointId: "endpoint-beta",
    evidenceAt: "2032-03-04T05:04:07Z",
    fleet: "production",
    freshness: "stale",
    labels: [{ key: "owner", value: "payments" }],
    releaseRef: "release-42",
    reportedAgentVersion: "v2.0.0",
    usernames: ["bob"],
  },
];

const workspace = {
  activity: [],
  changeRequests: [],
  endpoints: endpointRows,
  fleets: [
    {
      agentVersions: [{ count: 2, status: "v2.0.0" }],
      compliance: [
        { count: 1, status: "compliant" },
        { count: 1, status: "drifted" },
      ],
      endpointCount: 2,
      fleet: "production",
      freshness: [
        { count: 1, status: "recent" },
        { count: 1, status: "stale" },
      ],
    },
  ],
  sections: {
    activity: readySection(),
    changeRequests: readySection(),
    endpoints: readySection(),
    fleets: readySection(),
    state: readySection(),
  },
};

function endpointDetail(
  endpointId: "endpoint-alpha" | "endpoint-beta",
) {
  const header = endpointRows.find((endpoint) => endpoint.endpointId === endpointId)!;
  const alpha = endpointId === "endpoint-alpha";
  return {
    firewall: [],
    firewallTruncated: false,
    header,
    schedules: [],
    schedulesTruncated: false,
    sections: {
      firewall: {
        error: {
          guidance: "Ask an administrator for firewall-audit access.",
          kind: "authorization",
          message: "Firewall evidence is forbidden for this Operator.",
        },
        snapshot: { failedAt: loadedAt, loadedAt },
        state: "unavailable",
      },
      overview: readySection(),
      schedules: {
        error: {
          guidance: "Retry after the Endpoint reports schedules again.",
          kind: "connection",
          message: "Schedule evidence could not be loaded.",
        },
        snapshot: { failedAt: loadedAt, loadedAt },
        state: "unavailable",
      },
      state: readySection(),
      system: readySection(),
    },
    state: {
      digest: alpha ? "digest-alpha" : "digest-beta",
      endpointId,
      items: [
        {
          address: "base/packages",
          description: "Required system packages",
          desiredSummary: alpha ? "curl installed" : "jq installed",
          name: "Packages",
          observedSummary: alpha ? "curl 8.7.1" : "jq 1.7",
          provider: "packages",
          reasonCode: "matches",
          status: alpha ? "compliant" : "drifted",
          subresults: [],
          subresultsTruncated: false,
        },
      ],
      releaseRef: "release-42",
      reportedAt: loadedAt,
      status: header.compliance,
    },
    stateTruncated: false,
    system: {
      cpu: "AMD EPYC",
      cpuCores: "4",
      digest: alpha ? "system-alpha" : "system-beta",
      hostname: alpha ? "alpha.example" : "beta.example",
      kernel: "6.8.0",
      memory: "8 GiB",
      os: "Debian 13",
      reportedAt: loadedAt,
    },
  };
}

type DetailView = ReturnType<typeof endpointDetail>;
type DetailLoader = (endpointId: string) => Promise<DetailView>;

function renderApp(loadEndpointDetail: DetailLoader) {
  render(
    <App
      connection={{
        operatorId: "operator-investigation",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      fleetScope="All Fleets"
      loadEndpointDetail={loadEndpointDetail}
      workspace={workspace}
    />,
  );
}

async function openInventory(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Endpoints" }));
  return screen.getByRole("table", { name: "Endpoints" });
}

describe("Endpoint investigation flow", () => {
  it.each([
    { height: 900, label: "default", width: 1440 },
    { height: 720, label: "minimum supported", width: 1100 },
  ])(
    "supports the OS-DFV-033 keyboard-only contract at the $label viewport",
    async ({ height, width }) => {
      Object.defineProperty(window, "innerWidth", {
        configurable: true,
        value: width,
      });
      Object.defineProperty(window, "innerHeight", {
        configurable: true,
        value: height,
      });
      window.dispatchEvent(new Event("resize"));

      const user = userEvent.setup();
      const loadEndpointDetail = vi
        .fn<DetailLoader>()
        .mockResolvedValue(endpointDetail("endpoint-alpha"));
      renderApp(loadEndpointDetail);

      const endpointNavigation = screen.getByRole("button", {
        name: "Endpoints",
      });
      endpointNavigation.focus();
      expect(endpointNavigation).toHaveFocus();
      await user.keyboard("{Enter}");

      const inventory = screen.getByRole("table", { name: "Endpoints" });
      expect(inventory).toBeVisible();
      await user.keyboard("/");
      const search = screen.getByRole("searchbox", {
        name: "Search Endpoints",
      });
      expect(search).toHaveFocus();
      await user.keyboard("alpha");
      expect(screen.getByText("1 of 2 Endpoints")).toBeVisible();

      const origin = screen.getByRole("button", {
        name: "Inspect endpoint-alpha",
      });
      origin.focus();
      expect(origin).toHaveFocus();
      await user.keyboard("{Enter}");

      const dialog = await screen.findByRole("dialog", {
        name: "Endpoint endpoint-alpha",
      });
      expect(dialog).toHaveAttribute("aria-modal", "true");
      const close = within(dialog).getByRole("button", {
        name: "Close Endpoint endpoint-alpha",
      });
      expect(close).toHaveFocus();

      await user.keyboard("{Shift>}{Tab}{/Shift}");
      const overviewTab = within(dialog).getByRole("tab", {
        name: "Overview",
      });
      expect(overviewTab).toHaveFocus();
      expect(overviewTab).toHaveAttribute("aria-selected", "true");

      await user.keyboard("{ArrowRight}");
      const stateTab = within(dialog).getByRole("tab", { name: "State" });
      expect(stateTab).toHaveFocus();
      expect(stateTab).toHaveAttribute("aria-selected", "true");
      expect(within(dialog).getByRole("tabpanel")).toHaveAccessibleName(
        "State",
      );

      await user.keyboard("{End}");
      const systemTab = within(dialog).getByRole("tab", { name: "System" });
      expect(systemTab).toHaveFocus();
      expect(systemTab).toHaveAttribute("aria-selected", "true");

      await user.keyboard("{Escape}");
      expect(
        screen.queryByRole("dialog", { name: "Endpoint endpoint-alpha" }),
      ).not.toBeInTheDocument();
      expect(origin).toHaveFocus();
      expect(search).toHaveValue("alpha");
    },
  );

  it("keeps inventory and overlay status understandable without color", async () => {
    const user = userEvent.setup();
    const loadEndpointDetail = vi
      .fn<DetailLoader>()
      .mockResolvedValue(endpointDetail("endpoint-alpha"));
    renderApp(loadEndpointDetail);
    const inventory = await openInventory(user);

    const betaRow = within(inventory).getByText("endpoint-beta").closest("tr");
    expect(betaRow).not.toBeNull();
    for (const label of ["Drifted", "Stale"]) {
      const text = within(betaRow!).getByText(label);
      const status = text.closest(".endpoint-status");
      expect(status).not.toBeNull();
      expect(status?.querySelector("svg")).not.toBeNull();
    }

    await user.click(
      within(inventory).getByRole("button", {
        name: "Inspect endpoint-alpha",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Endpoint endpoint-alpha",
    });
    for (const label of ["Compliant", "Recent"]) {
      const text = within(dialog).getByText(label);
      const status = text.closest(".investigation-status");
      expect(status).not.toBeNull();
      expect(status?.querySelector("svg")).not.toBeNull();
    }

    await user.click(within(dialog).getByRole("tab", { name: "Schedules" }));
    const unavailable = within(dialog).getByRole("alert", {
      name: "Schedule evidence unavailable",
    });
    expect(
      within(unavailable).getByText("Schedule evidence could not be loaded."),
    ).toBeVisible();
    expect(unavailable.querySelector(".data-state-icon svg")).not.toBeNull();
  });

  it("opens exact identity and keeps ready tabs usable beside classified failures", async () => {
    const user = userEvent.setup();
    const loadEndpointDetail = vi
      .fn<DetailLoader>()
      .mockResolvedValue(endpointDetail("endpoint-alpha"));
    renderApp(loadEndpointDetail);
    await openInventory(user);

    await user.click(
      screen.getByRole("button", { name: "Inspect endpoint-alpha" }),
    );

    expect(loadEndpointDetail).toHaveBeenCalledOnce();
    expect(loadEndpointDetail).toHaveBeenCalledWith("endpoint-alpha");
    const dialog = await screen.findByRole("dialog", {
      name: "Endpoint endpoint-alpha",
    });
    expect(within(dialog).getByText("production")).toBeVisible();
    expect(within(dialog).getByText("Compliant")).toBeVisible();
    expect(within(dialog).getByText("Recent")).toBeVisible();
    expect(within(dialog).getByText("v2.0.0")).toBeVisible();
    expect(within(dialog).getByText("release-42")).toBeVisible();
    expect(within(dialog).getByText("owner")).toBeVisible();
    expect(within(dialog).getByText("platform")).toBeVisible();
    expect(
      within(dialog).queryByRole("button", { name: "Apply changes" }),
    ).not.toBeInTheDocument();

    const tabs = within(dialog).getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      "Overview",
      "State",
      "Schedules",
      "Firewall",
      "System",
    ]);

    await user.click(within(dialog).getByRole("tab", { name: "State" }));
    expect(
      within(dialog).getByRole("note", {
        name: "Git desired-state boundary",
      }),
    ).toHaveTextContent(
      "Git review is required before server sync can advance a Release ref",
    );
    expect(within(dialog).getByText("curl installed")).toBeVisible();

    await user.click(within(dialog).getByRole("tab", { name: "Schedules" }));
    const scheduleFailure = within(dialog).getByRole("alert", {
      name: "Schedule evidence unavailable",
    });
    expect(
      within(scheduleFailure).getByText(
        "Retry after the Endpoint reports schedules again.",
      ),
    ).toBeVisible();

    await user.click(within(dialog).getByRole("tab", { name: "Firewall" }));
    const firewallFailure = within(dialog).getByRole("status", {
      name: "Firewall evidence unavailable",
    });
    expect(
      within(firewallFailure).getByText(
        "Ask an administrator for firewall-audit access.",
      ),
    ).toBeVisible();

    await user.click(within(dialog).getByRole("tab", { name: "System" }));
    expect(within(dialog).getByText("alpha.example")).toBeVisible();
    expect(dialog).toBeVisible();
  });

  it("restores filtered, sorted, scrolled inventory context after close and Escape", async () => {
    const user = userEvent.setup();
    const loadEndpointDetail = vi
      .fn<DetailLoader>()
      .mockResolvedValue(endpointDetail("endpoint-alpha"));
    renderApp(loadEndpointDetail);
    await openInventory(user);

    const search = screen.getByRole("searchbox", { name: "Search Endpoints" });
    const compliance = screen.getByRole("combobox", {
      name: "Compliance filter",
    });
    await user.type(search, "prod");
    await user.selectOptions(compliance, "compliant");
    await user.click(screen.getByRole("button", { name: "Sort by Fleet" }));
    const scrollFrame = document.querySelector<HTMLElement>(
      ".endpoint-table-scroll",
    )!;
    scrollFrame.scrollLeft = 275;
    fireEvent.scroll(scrollFrame);

    const origin = screen.getByRole("button", {
      name: "Inspect endpoint-alpha",
    });
    await user.click(origin);
    await screen.findByRole("dialog", { name: "Endpoint endpoint-alpha" });
    await user.click(
      screen.getByRole("button", {
        name: "Close Endpoint endpoint-alpha",
      }),
    );

    expect(
      screen.queryByRole("dialog", { name: "Endpoint endpoint-alpha" }),
    ).not.toBeInTheDocument();
    expect(origin).toHaveFocus();
    expect(search).toHaveValue("prod");
    expect(compliance).toHaveValue("compliant");
    expect(screen.getByText("1 of 2 Endpoints")).toBeVisible();
    expect(
      screen.getByRole("columnheader", { name: "Fleet" }),
    ).toHaveAttribute("aria-sort", "ascending");
    expect(scrollFrame.scrollLeft).toBe(275);

    await user.click(origin);
    await screen.findByRole("dialog", { name: "Endpoint endpoint-alpha" });
    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("dialog", { name: "Endpoint endpoint-alpha" }),
    ).not.toBeInTheDocument();
    expect(origin).toHaveFocus();
    expect(search).toHaveValue("prod");
    expect(scrollFrame.scrollLeft).toBe(275);
  });

  it("ignores obsolete evidence when selection changes during loading", async () => {
    const user = userEvent.setup();
    let resolveAlpha!: (detail: DetailView) => void;
    let resolveBeta!: (detail: DetailView) => void;
    const alphaRequest = new Promise<DetailView>((resolve) => {
      resolveAlpha = resolve;
    });
    const betaRequest = new Promise<DetailView>((resolve) => {
      resolveBeta = resolve;
    });
    const loadEndpointDetail = vi.fn<DetailLoader>((endpointId) =>
      endpointId === "endpoint-alpha" ? alphaRequest : betaRequest,
    );
    renderApp(loadEndpointDetail);
    await openInventory(user);

    await user.click(
      screen.getByRole("button", { name: "Inspect endpoint-alpha" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Inspect endpoint-beta" }),
    );
    await act(async () => resolveBeta(endpointDetail("endpoint-beta")));

    const dialog = await screen.findByRole("dialog", {
      name: "Endpoint endpoint-beta",
    });
    await user.click(within(dialog).getByRole("tab", { name: "State" }));
    expect(within(dialog).getByText("jq installed")).toBeVisible();

    await act(async () => resolveAlpha(endpointDetail("endpoint-alpha")));
    expect(
      screen.queryByRole("dialog", { name: "Endpoint endpoint-alpha" }),
    ).not.toBeInTheDocument();
    expect(within(dialog).getByText("jq installed")).toBeVisible();
    expect(within(dialog).queryByText("curl installed")).not.toBeInTheDocument();
    expect(loadEndpointDetail.mock.calls).toEqual([
      ["endpoint-alpha"],
      ["endpoint-beta"],
    ]);
  });
});
