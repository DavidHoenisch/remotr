// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Overview } from "./Overview";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2032-03-04T05:05:07Z";

function readySection() {
  return {
    state: "ready",
    snapshot: { loadedAt },
  };
}

function endpoint(
  endpointId: string,
  fleet: string,
  compliance: string,
  freshness: string,
) {
  return {
    endpointId,
    fleet,
    usernames: [`operator-${endpointId}`],
    compliance,
    freshness,
    desiredAgentVersion: "v2.0.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-42",
    labels: [],
    evidenceAt: loadedAt,
  };
}

function statusCount(status: string, count: number) {
  return { status, count };
}

function buildWorkspace(activityForbidden = false) {
  const endpoints = [
    endpoint("endpoint-a", "production", "compliant", "recent"),
    endpoint("endpoint-b", "production", "drifted", "stale"),
    endpoint("endpoint-c", "staging", "compliant", "never_reported"),
    endpoint("endpoint-d", "staging", "apply_failed", "recent"),
  ];

  return {
    operator: {
      operatorId: "operator-overview",
      roles: ["read_only"],
    },
    sections: {
      fleets: readySection(),
      endpoints: readySection(),
      state: readySection(),
      changeRequests: readySection(),
      activity: activityForbidden
        ? {
            state: "unavailable",
            snapshot: { loadedAt, failedAt: loadedAt },
            error: {
              kind: "authorization",
              message: "Activity is unavailable for this Operator.",
              guidance: "Ask an administrator for the audit-reader role.",
            },
          }
        : readySection(),
    },
    endpoints,
    fleets: [
      {
        fleet: "production",
        endpointCount: 2,
        compliance: [
          statusCount("compliant", 1),
          statusCount("drifted", 1),
        ],
        freshness: [
          statusCount("recent", 1),
          statusCount("stale", 1),
        ],
        agentVersions: [statusCount("v2.0.0", 2)],
      },
      {
        fleet: "staging",
        endpointCount: 2,
        compliance: [
          statusCount("compliant", 1),
          statusCount("apply_failed", 1),
        ],
        freshness: [
          statusCount("recent", 1),
          statusCount("never_reported", 1),
        ],
        agentVersions: [statusCount("v2.0.0", 2)],
      },
    ],
    stateEvidence: endpoints.map((row) => ({
      endpointId: row.endpointId,
      releaseRef: row.releaseRef,
      digest: `digest-${row.endpointId}`,
      status: row.compliance,
      reportedAt: loadedAt,
      items: [],
    })),
    changeRequests: [
      {
        changeRequestId: "change-pending",
        fleet: "production",
        releaseRef: "release-42",
        risk: "standard",
        lifecycle: "pending",
        targetCount: 2,
        requiredApprovals: 1,
        approvalCount: 0,
        updatedAt: loadedAt,
      },
      {
        changeRequestId: "change-authorized",
        fleet: "staging",
        releaseRef: "release-43",
        risk: "elevated",
        lifecycle: "authorized",
        targetCount: 2,
        requiredApprovals: 2,
        approvalCount: 2,
        updatedAt: loadedAt,
      },
      {
        changeRequestId: "change-revoked",
        fleet: "production",
        releaseRef: "release-40",
        risk: "standard",
        lifecycle: "revoked",
        targetCount: 1,
        requiredApprovals: 1,
        approvalCount: 1,
        updatedAt: loadedAt,
      },
    ],
    activity: activityForbidden
      ? []
      : [
          {
            eventId: "event-2",
            occurredAt: "2032-03-04T05:04:07Z",
            actor: "operator-overview",
            action: "endpoint_enrolled",
            resourceType: "endpoint",
            resourceId: "endpoint-d",
            status: "201",
            requestId: "request-2",
            details: [],
          },
          {
            eventId: "event-1",
            occurredAt: "2032-03-04T05:03:07Z",
            actor: "operator-overview",
            action: "git_sync",
            resourceType: "server",
            resourceId: "primary",
            status: "200",
            requestId: "request-1",
            details: [],
          },
        ],
    activityNextCursor: "",
  };
}

describe("Overview", () => {
  it("renders internally consistent counts without double-counting state evidence", () => {
    render(<Overview onNavigate={() => {}} workspace={buildWorkspace()} />);

    expect(
      screen.getByRole("button", { name: "4 total Endpoints" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "2 compliant Endpoints" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "1 drifted Endpoint" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "1 apply failed Endpoint" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "2 recent Endpoints" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "1 stale Endpoint" }),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "1 never reported Endpoint" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "2 Fleets" })).toBeVisible();
    expect(
      screen.getByRole("button", {
        name: "2 pending or active Change requests",
      }),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "8 total Endpoints" }),
    ).not.toBeInTheDocument();

    const activity = screen.getByRole("region", { name: "Recent activity" });
    expect(within(activity).getAllByRole("listitem")).toHaveLength(2);
    expect(within(activity).getByText("endpoint-d")).toBeVisible();
    expect(within(activity).getByText("primary")).toBeVisible();
  });

  it("keeps fleet evidence visible when Activity is forbidden", () => {
    render(
      <Overview onNavigate={() => {}} workspace={buildWorkspace(true)} />,
    );

    expect(
      screen.getByRole("button", { name: "4 total Endpoints" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "2 Fleets" })).toBeVisible();

    const activity = screen.getByRole("region", { name: "Recent activity" });
    expect(within(activity).getByText("Activity unavailable")).toBeVisible();
    expect(
      within(activity).getByText(
        "Ask an administrator for the audit-reader role.",
      ),
    ).toBeVisible();
    expect(screen.queryByText("Connection failed")).not.toBeInTheDocument();
  });

  it("opens source pages with equivalent summary filters", async () => {
    const user = userEvent.setup();
    const onNavigate = vi.fn();
    render(<Overview onNavigate={onNavigate} workspace={buildWorkspace()} />);

    await user.click(
      screen.getByRole("button", { name: "2 compliant Endpoints" }),
    );
    await user.click(
      screen.getByRole("button", { name: "1 stale Endpoint" }),
    );
    await user.click(screen.getByRole("button", { name: "2 Fleets" }));
    await user.click(
      screen.getByRole("button", {
        name: "2 pending or active Change requests",
      }),
    );

    expect(onNavigate.mock.calls).toEqual([
      [
        {
          page: "endpoints",
          filters: { compliance: ["compliant"] },
        },
      ],
      [
        {
          page: "endpoints",
          filters: { freshness: ["stale"] },
        },
      ],
      [{ page: "fleets", filters: {} }],
      [
        {
          page: "change-requests",
          filters: { lifecycle: ["pending", "authorized"] },
        },
      ],
    ]);
  });
});
