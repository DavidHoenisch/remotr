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

const loadedAt = "2032-03-04T05:07:07Z";
const untrustedReference = ["https:/", "/untrusted.example/run"].join("");
const ready = { snapshot: { loadedAt }, state: "ready" };
const unavailable = {
  error: {
    guidance: "Ask an administrator for the auditor role.",
    kind: "authorization",
    message: "The current Operator is not authorized to list Activity.",
  },
  snapshot: { failedAt: loadedAt, loadedAt: "" },
  state: "unavailable",
};

const eventTwo = {
  action: "git_sync",
  actor: "operator-a",
  details: [
    { key: "release_ref", value: "release-42" },
    { key: "note", value: "<script>window.evil=true</script>" },
    { key: "reference", value: untrustedReference },
  ],
  eventId: "event-2",
  occurredAt: "2032-03-04T05:06:07Z",
  requestId: "request-2",
  resourceId: "primary",
  resourceType: "server",
  status: "accepted",
};

const eventThree = {
  action: "git_sync",
  actor: "operator-b",
  details: [],
  eventId: "event-3",
  occurredAt: "2032-03-04T05:03:07Z",
  requestId: "request-3",
  resourceId: "endpoint-a",
  resourceType: "endpoint",
  status: "failed",
};

const eventFour = {
  action: "endpoint_sync",
  actor: "operator-c",
  details: [],
  eventId: "event-4",
  occurredAt: "2032-03-04T05:01:07Z",
  requestId: "request-4",
  resourceId: "endpoint-b",
  resourceType: "endpoint",
  status: "accepted",
};

interface ActivityPageRequest {
  action: string;
  actorType: string;
  cursor: string;
  seenEventIds: string[];
  since: string;
  until: string;
}

interface ActivityPageView {
  events: typeof eventTwo[];
  nextCursor: string;
  section: typeof ready;
}

type ActivityPageLoader = (
  request: ActivityPageRequest,
) => Promise<ActivityPageView>;

function workspace(activitySection = ready) {
  return {
    activity: [],
    activityNextCursor: "",
    changeRequests: [],
    endpoints: [
      {
        compliance: "compliant",
        desiredAgentVersion: "v1.2.4",
        endpointId: "endpoint-permitted",
        evidenceAt: "2032-03-04T05:00:00Z",
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
      activity: activitySection,
      changeRequests: ready,
      endpoints: ready,
      fleets: ready,
      state: ready,
    },
  };
}

function renderApp(
  loadActivityPage?: ActivityPageLoader,
  activitySection = ready,
) {
  render(
    <App
      connection={{
        operatorId: "operator-activity",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      loadActivityPage={loadActivityPage}
      workspace={workspace(activitySection)}
    />,
  );
}

function visibleEventIDs(table: HTMLElement): string[] {
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell")[0].textContent ?? "");
}

describe("ActivityPage", () => {
  it("uses the server cursor with preserved filters, stable deduplication, and inert structured detail", async () => {
    const user = userEvent.setup();
    const loadActivityPage = vi
      .fn<ActivityPageLoader>()
      .mockResolvedValueOnce({
        events: [eventTwo, eventThree],
        nextCursor: "cursor-2",
        section: ready,
      })
      .mockResolvedValueOnce({
        events: [eventThree, eventFour],
        nextCursor: "",
        section: ready,
      });
    renderApp(loadActivityPage);

    await user.click(screen.getByRole("button", { name: "Activity" }));
    await user.type(
      screen.getByRole("textbox", { name: "Activity since" }),
      "2032-03-04T04:00:00Z",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Activity until" }),
      "2032-03-04T06:00:00Z",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Activity action" }),
      "git_sync",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Activity actor type" }),
      "operator",
    );
    await user.click(screen.getByRole("button", { name: "Apply Activity filters" }));

    const firstRequest = {
      action: "git_sync",
      actorType: "operator",
      cursor: "",
      seenEventIds: [],
      since: "2032-03-04T04:00:00Z",
      until: "2032-03-04T06:00:00Z",
    };
    await waitFor(() => expect(loadActivityPage).toHaveBeenCalledWith(firstRequest));

    const table = await screen.findByRole("table", { name: "Activity" });
    expect(visibleEventIDs(table)).toEqual(["event-2", "event-3"]);
    const eventTwoRow = within(
      screen.getByRole("cell", { name: "event-2" }).closest("tr")!,
    );
    for (const value of [
      "2032-03-04T05:06:07Z",
      "operator-a",
      "git_sync",
      "server / primary",
      "accepted",
      "request-2",
    ]) {
      expect(eventTwoRow.getByText(value)).toBeVisible();
    }

    await user.click(screen.getByRole("button", { name: "Inspect event-2" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Activity event event-2",
    });
    const details = within(dialog).getByRole("region", {
      name: "Structured audit details",
    });
    expect(within(details).getByText("release_ref")).toBeVisible();
    expect(within(details).getByText("release-42")).toBeVisible();
    expect(
      within(details).getByText("<script>window.evil=true</script>"),
    ).toBeVisible();
    expect(within(details).getByText(untrustedReference)).toBeVisible();
    expect(within(details).queryByRole("link")).not.toBeInTheDocument();
    expect(within(details).queryByRole("img")).not.toBeInTheDocument();
    expect((window as Window & { evil?: boolean }).evil).toBeUndefined();

    await user.click(
      within(dialog).getByRole("button", { name: "Close Activity event event-2" }),
    );
    await user.click(screen.getByRole("button", { name: "Load more Activity" }));
    await waitFor(() =>
      expect(loadActivityPage).toHaveBeenLastCalledWith({
        ...firstRequest,
        cursor: "cursor-2",
        seenEventIds: ["event-2", "event-3"],
      }),
    );
    await waitFor(() =>
      expect(visibleEventIDs(table)).toEqual(["event-2", "event-3", "event-4"]),
    );
    expect(screen.getAllByRole("cell", { name: "event-3" })).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Load more Activity" }),
    ).not.toBeInTheDocument();
  });

  it("keeps an Activity authorization failure local to the connected shell", async () => {
    const user = userEvent.setup();
    renderApp(undefined, unavailable);

    await user.click(screen.getByRole("button", { name: "Activity" }));
    const activity = screen.getByRole("region", { name: "Activity evidence" });
    expect(within(activity).getByText("Activity unavailable")).toBeVisible();
    expect(
      within(activity).getByText(
        "The current Operator is not authorized to list Activity.",
      ),
    ).toBeVisible();
    expect(
      within(activity).getByText("Ask an administrator for the auditor role."),
    ).toBeVisible();
    expect(screen.getByText("Connected")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    expect(screen.getByRole("table", { name: "Endpoints" })).toBeVisible();
    expect(screen.getByRole("cell", { name: "endpoint-permitted" })).toBeVisible();
  });
});
