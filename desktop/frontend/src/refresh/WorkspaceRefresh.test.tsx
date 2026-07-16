// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

class ManualRefreshClock {
  private intervals = new Map<
    number,
    { callback: () => void; intervalMs: number; nextAt: number }
  >();
  private nextID = 1;
  private nowMs: number;

  constructor(now: string) {
    this.nowMs = Date.parse(now);
  }

  now = () => new Date(this.nowMs);

  setInterval = (callback: () => void, intervalMs: number): number => {
    const id = this.nextID++;
    this.intervals.set(id, {
      callback,
      intervalMs,
      nextAt: this.nowMs + intervalMs,
    });
    return id;
  };

  clearInterval = (id: number) => {
    this.intervals.delete(id);
  };

  advanceBy(milliseconds: number) {
    const target = this.nowMs + milliseconds;
    while (true) {
      const dueAt = Math.min(
        ...[...this.intervals.values()].map((interval) => interval.nextAt),
      );
      if (!Number.isFinite(dueAt) || dueAt > target) {
        break;
      }
      this.nowMs = dueAt;
      for (const [id, interval] of this.intervals) {
        if (interval.nextAt !== dueAt) {
          continue;
        }
        interval.nextAt += interval.intervalMs;
        this.intervals.set(id, interval);
        interval.callback();
      }
    }
    this.nowMs = target;
  }
}

class ManualWorkspaceVisibility {
  private listeners = new Set<(visible: boolean) => void>();

  constructor(private visible: boolean) {}

  isVisible = () => this.visible;

  subscribe = (listener: (visible: boolean) => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  setVisible(visible: boolean) {
    if (visible === this.visible) {
      return;
    }
    this.visible = visible;
    for (const listener of this.listeners) {
      listener(visible);
    }
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

function workspaceFor(endpointId: string, loadedAt: string) {
  const ready = { snapshot: { loadedAt }, state: "ready" };
  return {
    activity: [],
    activityNextCursor: "",
    changeRequests: [],
    endpoints: [
      {
        compliance: "compliant",
        desiredAgentVersion: "v1.2.4",
        endpointId,
        evidenceAt: loadedAt,
        fleet: "production",
        freshness: "recent",
        labels: [{ key: "region", value: "west" }],
        releaseRef: "release-42",
        reportedAgentVersion: "v1.2.4",
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
}

type Workspace = ReturnType<typeof workspaceFor>;
type WorkspaceLoader = () => Promise<Workspace>;

function renderRefreshApp({
  clock,
  loadWorkspace,
  visibility,
  workspace,
}: {
  clock: ManualRefreshClock;
  loadWorkspace: WorkspaceLoader;
  visibility: ManualWorkspaceVisibility;
  workspace: Workspace;
}) {
  render(
    <App
      connection={{
        operatorId: "operator-refresh",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      loadWorkspace={loadWorkspace}
      refreshClock={clock}
      workspace={workspace}
      workspaceVisibility={visibility}
    />,
  );
}

describe("workspace refresh", () => {
  it("atomically replaces a successful snapshot and retains visibly stale evidence after failure", async () => {
    const clock = new ManualRefreshClock("2032-03-04T05:00:00Z");
    const visibility = new ManualWorkspaceVisibility(true);
    const initial = workspaceFor("endpoint-old", "2032-03-04T05:00:00Z");
    const refreshed = workspaceFor(
      "endpoint-new",
      "2032-03-04T05:00:30Z",
    );
    const first = deferred<Workspace>();
    const second = deferred<Workspace>();
    const loadWorkspace = vi
      .fn<WorkspaceLoader>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const user = userEvent.setup();
    renderRefreshApp({ clock, loadWorkspace, visibility, workspace: initial });
    await user.click(screen.getByRole("button", { name: "Endpoints" }));

    clock.advanceBy(30_000);
    expect(loadWorkspace).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("cell", { name: "endpoint-old" })).toBeVisible();
    expect(
      screen.queryByRole("cell", { name: "endpoint-new" }),
    ).not.toBeInTheDocument();

    clock.advanceBy(30_000);
    expect(loadWorkspace).toHaveBeenCalledTimes(1);
    first.resolve(refreshed);
    await waitFor(() =>
      expect(screen.getByRole("cell", { name: "endpoint-new" })).toBeVisible(),
    );
    expect(
      screen.queryByRole("cell", { name: "endpoint-old" }),
    ).not.toBeInTheDocument();

    clock.advanceBy(30_000);
    expect(loadWorkspace).toHaveBeenCalledTimes(2);
    second.reject({
      guidance: "Check the active server connection.",
      kind: "connection",
      message: "Workspace refresh could not reach the server.",
    });

    const freshness = await screen.findByRole("region", {
      name: "Workspace freshness",
    });
    expect(within(freshness).getByText("Stale workspace evidence")).toBeVisible();
    expect(
      within(freshness).getByText("Data loaded 2032-03-04T05:00:30Z"),
    ).toBeVisible();
    expect(
      within(freshness).getByText(
        "Refresh failed 2032-03-04T05:01:30.000Z",
      ),
    ).toBeVisible();
    expect(
      within(freshness).getByText(
        "Workspace refresh could not reach the server.",
      ),
    ).toBeVisible();
    expect(screen.getByRole("cell", { name: "endpoint-new" })).toBeVisible();
    expect(within(freshness).queryByText("Current")).not.toBeInTheDocument();
  });

  it("pauses polling while hidden and refreshes once immediately on resume", async () => {
    const clock = new ManualRefreshClock("2032-03-04T05:00:00Z");
    const visibility = new ManualWorkspaceVisibility(false);
    const initial = workspaceFor("endpoint-old", "2032-03-04T05:00:00Z");
    const refreshed = workspaceFor(
      "endpoint-resumed",
      "2032-03-04T05:01:30Z",
    );
    const resumed = deferred<Workspace>();
    const loadWorkspace = vi
      .fn<WorkspaceLoader>()
      .mockReturnValueOnce(resumed.promise)
      .mockResolvedValue(refreshed);
    renderRefreshApp({ clock, loadWorkspace, visibility, workspace: initial });

    clock.advanceBy(90_000);
    expect(loadWorkspace).not.toHaveBeenCalled();
    visibility.setVisible(true);
    expect(loadWorkspace).toHaveBeenCalledTimes(1);
    clock.advanceBy(30_000);
    expect(loadWorkspace).toHaveBeenCalledTimes(1);

    resumed.resolve(refreshed);
    await waitFor(() =>
      expect(screen.getByText("endpoint-resumed")).toBeVisible(),
    );
    clock.advanceBy(30_000);
    expect(loadWorkspace).toHaveBeenCalledTimes(2);
  });

  it("provides explicit and shortcut refresh without intercepting an editor", async () => {
    const clock = new ManualRefreshClock("2032-03-04T05:00:00Z");
    const visibility = new ManualWorkspaceVisibility(true);
    const initial = workspaceFor("endpoint-old", "2032-03-04T05:00:00Z");
    const refreshed = workspaceFor(
      "endpoint-refreshed",
      "2032-03-04T05:00:01Z",
    );
    const loadWorkspace = vi
      .fn<WorkspaceLoader>()
      .mockResolvedValue(refreshed);
    const user = userEvent.setup();
    renderRefreshApp({ clock, loadWorkspace, visibility, workspace: initial });

    await user.click(screen.getByRole("button", { name: "Refresh workspace" }));
    await waitFor(() => expect(loadWorkspace).toHaveBeenCalledTimes(1));
    await user.keyboard("{Control>}r{/Control}");
    await waitFor(() => expect(loadWorkspace).toHaveBeenCalledTimes(2));

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    const search = screen.getByRole("searchbox", { name: "Search Endpoints" });
    await user.type(search, "endpoint");
    await user.keyboard("{Control>}r{/Control}");
    expect(loadWorkspace).toHaveBeenCalledTimes(2);
    expect(search).toHaveValue("endpoint");
  });
});
