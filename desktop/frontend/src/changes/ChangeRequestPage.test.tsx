// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";
import { ChangeRequestPage } from "./ChangeRequestPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2032-03-04T05:06:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const summaries = [
  {
    approvalCount: 1,
    changeRequestId: "change-active",
    fleet: "production",
    lifecycle: "authorized",
    releaseRef: "release-41",
    requiredApprovals: 1,
    risk: "destructive",
    targetCount: 1,
    updatedAt: "2032-03-04T05:05:07Z",
  },
  {
    approvalCount: 0,
    changeRequestId: "change-pending",
    fleet: "production",
    lifecycle: "pending",
    releaseRef: "release-42",
    requiredApprovals: 2,
    risk: "connectivity",
    targetCount: 2,
    updatedAt: "2032-03-04T05:03:07Z",
  },
];

const activeDetail = {
  approvalCount: 1,
  approvals: [
    {
      approvedAt: "2032-03-04T05:04:07Z",
      justification: "approved after peer review",
      operatorId: "operator-approver",
    },
  ],
  approvalsTruncated: false,
  artifactDigest: "artifact-41",
  authorizationGroup: "network-transition",
  history: [
    {
      action: "created",
      actorId: "operator-a",
      details: "request created",
      occurredAt: "2032-03-04T05:01:07Z",
    },
    {
      action: "rollout_authorized",
      actorId: "operator-approver",
      details: "approval threshold met",
      occurredAt: "2032-03-04T05:05:07Z",
    },
  ],
  historyTruncated: false,
  outcomes: [
    {
      endpointId: "endpoint-a",
      reason: "observed compliant",
      state: "verified_successful",
    },
  ],
  outcomesTruncated: false,
  policyWarning: "destructive review required",
  readOnly: true,
  resources: [
    {
      activationTargets: ["firewalld"],
      address: "base/firewall",
      authorizationGroup: "network-transition",
      baselineEligible: true,
      dependsOn: [],
      desiredHash: "hash-1",
      predictedEffects: ["reload firewall"],
      provider: "nftables",
      risk: "destructive",
      rollbackClass: "automatic",
    },
  ],
  resourcesTruncated: false,
  summary: summaries[0],
  targets: [
    {
      compatible: true,
      endpointId: "endpoint-a",
      preflightReady: true,
      preflightReason: "all checks passed",
    },
  ],
  targetsTruncated: false,
};

type ChangeDetail = typeof activeDetail;
type ChangeDetailLoader = (changeRequestId: string) => Promise<ChangeDetail>;

function renderApp(loadChangeRequestDetail: ChangeDetailLoader) {
  render(
    <App
      connection={{
        operatorId: "operator-changes",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      loadChangeRequestDetail={loadChangeRequestDetail}
      workspace={{
        activity: [],
        changeRequests: summaries,
        endpoints: [],
        fleets: [],
        sections: {
          activity: ready,
          changeRequests: ready,
          endpoints: ready,
          fleets: ready,
          state: ready,
        },
      }}
    />,
  );
}

function rowFor(changeRequestId: string): HTMLElement {
  const cell = screen.getByRole("cell", { name: changeRequestId });
  const row = cell.closest("tr");
  if (!row) {
    throw new Error(`Change request ${changeRequestId} is not in a row`);
  }
  return row;
}

describe("ChangeRequestPage", () => {
  it("explains the Git-only desired-state boundary in an empty inventory", () => {
    render(<ChangeRequestPage summaries={[]} />);

    const boundary = screen.getByRole("note", {
      name: "Git desired-state boundary",
    });
    expect(within(boundary).getByText("Desired state stays in Git")).toBeVisible();
    expect(boundary).toHaveTextContent(
      "Git review is required before server sync can advance a Release ref",
    );
    expect(boundary).toHaveTextContent(
      "does not edit, stage, commit, push, merge, or directly apply Configuration content",
    );
  });

  it("renders exact server lifecycle, risk, target, approval, and update evidence", async () => {
    const user = userEvent.setup();
    const loadChangeRequestDetail = vi
      .fn<ChangeDetailLoader>()
      .mockResolvedValue(activeDetail);
    renderApp(loadChangeRequestDetail);

    await user.click(
      screen.getByRole("button", { name: "Change requests" }),
    );
    const table = screen.getByRole("table", { name: "Change requests" });
    const active = within(rowFor("change-active"));
    expect(active.getByText("authorized")).toBeVisible();
    expect(active.getByText("destructive")).toBeVisible();
    expect(active.getByText("1 target")).toBeVisible();
    expect(active.getByText("1 / 1")).toBeVisible();
    expect(active.getByText("2032-03-04T05:05:07Z")).toBeVisible();

    const pending = within(rowFor("change-pending"));
    expect(pending.getByText("pending")).toBeVisible();
    expect(pending.getByText("connectivity")).toBeVisible();
    expect(pending.getByText("2 targets")).toBeVisible();
    expect(pending.getByText("0 / 2")).toBeVisible();
    expect(pending.getByText("2032-03-04T05:03:07Z")).toBeVisible();
    expect(pending.queryByText("approved")).not.toBeInTheDocument();
    expect(pending.queryByText("completed")).not.toBeInTheDocument();
    expect(within(table).getAllByRole("row")).toHaveLength(3);
  });

  it("shows bounded read-only plan through outcome evidence with no mutations", async () => {
    const user = userEvent.setup();
    const loadChangeRequestDetail = vi
      .fn<ChangeDetailLoader>()
      .mockResolvedValue(activeDetail);
    renderApp(loadChangeRequestDetail);
    await user.click(
      screen.getByRole("button", { name: "Change requests" }),
    );

    await user.click(
      screen.getByRole("button", { name: "Inspect change-active" }),
    );
    expect(loadChangeRequestDetail).toHaveBeenCalledWith("change-active");
    const dialog = await screen.findByRole("dialog", {
      name: "Change request change-active",
    });
    expect(within(dialog).getByText("Read-only evidence")).toBeVisible();
    expect(
      within(dialog).getByRole("note", {
        name: "Git desired-state boundary",
      }),
    ).toBeVisible();

    const plan = within(dialog).getByRole("region", { name: "Change plan" });
    expect(within(plan).getByText("artifact-41")).toBeVisible();
    expect(within(plan).getByText("base/firewall")).toBeVisible();
    expect(within(plan).getByText("nftables")).toBeVisible();
    expect(within(plan).getByText("reload firewall")).toBeVisible();
    expect(within(plan).getByText("automatic")).toBeVisible();

    const authorization = within(dialog).getByRole("region", {
      name: "Authorization evidence",
    });
    expect(within(authorization).getByText("authorized")).toBeVisible();
    expect(within(authorization).getByText("network-transition")).toBeVisible();
    expect(within(authorization).getByText("1 of 1 approvals")).toBeVisible();
    expect(within(authorization).getByText("operator-approver")).toBeVisible();
    expect(
      within(authorization).getByText("destructive review required"),
    ).toBeVisible();

    const execution = within(dialog).getByRole("region", {
      name: "Execution evidence",
    });
    expect(within(execution).getByText("endpoint-a")).toBeVisible();
    expect(within(execution).getByText("Compatible")).toBeVisible();
    expect(within(execution).getByText("Preflight ready")).toBeVisible();
    expect(within(execution).getByText("Rollout window")).toBeVisible();
    expect(within(execution).getByText("Not reported")).toBeVisible();
    expect(within(execution).getByText("1 of 1 outcomes reported")).toBeVisible();

    const outcomes = within(dialog).getByRole("region", {
      name: "Outcome evidence",
    });
    expect(within(outcomes).getByText("verified_successful")).toBeVisible();
    expect(within(outcomes).getByText("observed compliant")).toBeVisible();

    const history = within(dialog).getByRole("region", {
      name: "Change history",
    });
    expect(within(history).getByText("created")).toBeVisible();
    expect(within(history).getByText("rollout_authorized")).toBeVisible();

    for (const name of [
      "Authorize",
      "Pause",
      "Resume",
      "Revoke",
      "Promote baseline",
      "Adopt baseline",
    ]) {
      expect(within(dialog).queryByRole("button", { name })).not.toBeInTheDocument();
    }
  });
});
