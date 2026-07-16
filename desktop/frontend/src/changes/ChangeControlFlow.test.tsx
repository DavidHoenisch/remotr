// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { RefreshClock, WorkspaceVisibility } from "../refresh/useWorkspaceRefresh";
import { ChangeRequestDetail, type ChangeRequestDetailView } from "./ChangeRequestDetail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

class ManualChangeClock implements RefreshClock {
  private intervals = new Map<number, { callback: () => void; intervalMs: number; nextAt: number }>();
  private nextID = 1;
  private nowMs = Date.parse("2032-03-04T05:00:00Z");
  readonly scheduledIntervals: number[] = [];

  clearInterval = (id: number) => this.intervals.delete(id);
  now = () => new Date(this.nowMs);
  setInterval = (callback: () => void, intervalMs: number) => {
    this.scheduledIntervals.push(intervalMs);
    const id = this.nextID++;
    this.intervals.set(id, { callback, intervalMs, nextAt: this.nowMs + intervalMs });
    return id;
  };

  advanceBy(milliseconds: number) {
    const target = this.nowMs + milliseconds;
    while (true) {
      const dueAt = Math.min(...[...this.intervals.values()].map((entry) => entry.nextAt));
      if (!Number.isFinite(dueAt) || dueAt > target) break;
      this.nowMs = dueAt;
      for (const entry of this.intervals.values()) {
        if (entry.nextAt === dueAt) {
          entry.nextAt += entry.intervalMs;
          entry.callback();
        }
      }
    }
    this.nowMs = target;
  }
}

class ManualChangeVisibility implements WorkspaceVisibility {
  private listeners = new Set<(visible: boolean) => void>();
  constructor(private visible = true) {}
  isVisible = () => this.visible;
  subscribe = (listener: (visible: boolean) => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };
  setVisible(visible: boolean) {
    this.visible = visible;
    for (const listener of this.listeners) listener(visible);
  }
}

const detail: ChangeRequestDetailView = {
  approvals: [],
  approvalsTruncated: false,
  artifactDigest: "sha256:artifact",
  authorizationGroup: "network",
  history: [{ action: "created", actorId: "operator-seed", details: "", occurredAt: "2032-03-04T05:00:00Z" }],
  historyTruncated: false,
  outcomes: [
    { endpointId: "endpoint-a", reason: "", state: "verified_successful" },
    { endpointId: "endpoint-b", reason: "awaiting check-in", state: "not_seen" },
  ],
  outcomesTruncated: false,
  policyWarning: "Two Operators are required for this risk.",
  readOnly: false,
  resources: [{
    activationTargets: ["endpoint-a", "endpoint-b"],
    address: "base/firewall",
    authorizationGroup: "network",
    baselineEligible: true,
    dependsOn: [],
    desiredHash: "sha256:firewall",
    predictedEffects: ["replace nftables ruleset"],
    provider: "nftables",
    risk: "connectivity",
    rollbackClass: "automatic",
  }],
  resourcesTruncated: false,
  summary: {
    approvalCount: 0,
    changeRequestId: "change-prod",
    fleet: "production",
    lifecycle: "pending",
    releaseRef: "release-42",
    requiredApprovals: 2,
    risk: "connectivity",
    targetCount: 2,
    updatedAt: "2032-03-04T05:00:00Z",
  },
  targets: [
    { compatible: true, endpointId: "endpoint-a", preflightReady: true, preflightReason: "" },
    { compatible: true, endpointId: "endpoint-b", preflightReady: true, preflightReason: "" },
  ],
  targetsTruncated: false,
};

function actionResult(action: string, nextDetail = detail) {
  return {
    action,
    affectedEvidence: ["change_request", "activity"],
    changeRequest: nextDetail,
  };
}

describe("Change-control parity", () => {
  it("preserves bounded approval, lifecycle, baseline, adoption, Activity, and deterministic watch controls", async () => {
    const user = userEvent.setup();
    const clock = new ManualChangeClock();
    const visibility = new ManualChangeVisibility();
    const pendingApproval = {
      ...detail,
      approvals: [{ approvedAt: "2032-03-04T05:05:00Z", justification: "CHG-404", operatorId: "operator-a" }],
      summary: { ...detail.summary, approvalCount: 1 },
    };
    const authorizeChangeRequest = vi.fn().mockResolvedValue(actionResult("approval_recorded", pendingApproval));
    const changeRequestLifecycle = vi.fn().mockImplementation(async (request: { action: string }) => actionResult(`${request.action}d`, {
      ...pendingApproval,
      summary: { ...pendingApproval.summary, lifecycle: request.action === "pause" ? "paused" : request.action === "resume" ? "authorized" : "revoked" },
    }));
    const promoteChangeBaseline = vi.fn().mockResolvedValue({
      ...actionResult("baseline_promoted", pendingApproval),
      baseline: {
        authorizedAt: "2032-03-04T05:06:00Z",
        authorizedBy: "operator-a",
        changeRequestId: "change-prod",
        desiredHash: "sha256:firewall",
        fleet: "production",
        id: "baseline-prod",
        provider: "nftables",
        resourceAddress: "base/firewall",
        risk: "connectivity",
      },
    });
    const chooseBaselineAdoptionPlan = vi.fn().mockResolvedValue({
      artifactDigest: "sha256:adopt",
      fleet: "production",
      planId: "opaque-plan",
      releaseRef: "release-adopt",
      resourceAddresses: ["base/sudo"],
      resourceCount: 1,
      targetCount: 2,
    });
    const createBaselineAdoption = vi.fn().mockResolvedValue(actionResult("baseline_adoption_created", {
      ...detail,
      summary: { ...detail.summary, changeRequestId: "adoption-prod" },
    }));
    const loadChangeRequestDetail = vi.fn().mockResolvedValue(detail);
    const refreshActivity = vi.fn().mockResolvedValue(undefined);
    const onChanged = vi.fn();

    render(
      <ChangeRequestDetail
        authorizeChangeRequest={authorizeChangeRequest}
        changeRequestLifecycle={changeRequestLifecycle}
        chooseBaselineAdoptionPlan={chooseBaselineAdoptionPlan}
        clock={clock}
        createBaselineAdoption={createBaselineAdoption}
        detail={detail}
        loadChangeRequestDetail={loadChangeRequestDetail}
        onChanged={onChanged}
        promoteChangeBaseline={promoteChangeBaseline}
        refreshActivity={refreshActivity}
        visibility={visibility}
        watchRandom={() => 0.5}
      />,
    );

    expect(screen.queryByText("Read-only evidence")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Change-control actions" })).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Start watch" }));
    await waitFor(() => expect(loadChangeRequestDetail).toHaveBeenCalledTimes(1));
    visibility.setVisible(false);
    clock.advanceBy(6_000);
    expect(loadChangeRequestDetail).toHaveBeenCalledTimes(1);
    visibility.setVisible(true);
    await waitFor(() => expect(loadChangeRequestDetail).toHaveBeenCalledTimes(2));
    clock.advanceBy(2_000);
    await waitFor(() => expect(loadChangeRequestDetail).toHaveBeenCalledTimes(3));
    await user.click(screen.getByRole("button", { name: "Stop watch" }));

    const authorize = screen.getByRole("group", { name: "Authorize rollout" });
    await user.type(within(authorize).getByLabelText("Justification"), "CHG-404");
    await user.clear(within(authorize).getByLabelText("Maximum concurrency"));
    await user.type(within(authorize).getByLabelText("Maximum concurrency"), "1");
    await user.type(within(authorize).getByLabelText("Confirm Change request ID"), "change-prod");
    await user.click(within(authorize).getByRole("button", { name: "Record approval" }));
    await waitFor(() => expect(authorizeChangeRequest).toHaveBeenCalledWith(expect.objectContaining({
      attemptLimit: 1,
      changeRequestId: "change-prod",
      confirmation: "change-prod",
      justification: "CHG-404",
      maxConcurrency: 1,
    })));
    expect(await screen.findByText("Approval recorded · 1 of 2 required")).toBeVisible();
    expect(screen.queryByText("Rollout authorized")).not.toBeInTheDocument();

    const lifecycle = screen.getByRole("group", { name: "Lifecycle controls" });
    await user.type(within(lifecycle).getByLabelText("Confirm lifecycle Change request ID"), "change-prod");
    await user.click(within(lifecycle).getByRole("button", { name: "Pause rollout" }));
    await waitFor(() => expect(changeRequestLifecycle).toHaveBeenCalledWith({ action: "pause", changeRequestId: "change-prod", confirmation: "change-prod" }));

    const promotion = screen.getByRole("group", { name: "Promote baseline" });
    await user.type(within(promotion).getByLabelText("Confirm resource address"), "base/firewall");
    await user.click(within(promotion).getByLabelText("Acknowledge 1 unresolved target outcome"));
    await user.click(within(promotion).getByRole("button", { name: "Promote exact resource" }));
    await waitFor(() => expect(promoteChangeBaseline).toHaveBeenCalledWith({
      acknowledgeExceptions: true,
      changeRequestId: "change-prod",
      confirmation: "base/firewall",
      resourceAddress: "base/firewall",
    }));

    const adoption = screen.getByRole("group", { name: "Adopt existing baseline" });
    await user.click(within(adoption).getByRole("button", { name: "Choose Fleet plan" }));
    expect(await within(adoption).findByText("release-adopt")).toBeVisible();
    await user.type(within(adoption).getByLabelText("Confirm adoption Fleet"), "production");
    await user.click(within(adoption).getByRole("button", { name: "Create adoption request" }));
    await waitFor(() => expect(createBaselineAdoption).toHaveBeenCalledWith({ confirmation: "production", fleet: "production", planId: "opaque-plan" }));

    expect(refreshActivity).toHaveBeenCalledTimes(4);
    expect(onChanged).toHaveBeenCalled();
  });

  it("stops at the deterministic timeout even when the prior read is pending", async () => {
    const user = userEvent.setup();
    const clock = new ManualChangeClock();
    const visibility = new ManualChangeVisibility();
    const loadChangeRequestDetail = vi.fn().mockImplementation(
      () => new Promise<ChangeRequestDetailView>(() => undefined),
    );
    const unusedAction = vi.fn().mockRejectedValue(new Error("unused"));

    render(
      <ChangeRequestDetail
        authorizeChangeRequest={unusedAction}
        changeRequestLifecycle={unusedAction}
        chooseBaselineAdoptionPlan={unusedAction}
        clock={clock}
        createBaselineAdoption={unusedAction}
        detail={detail}
        loadChangeRequestDetail={loadChangeRequestDetail}
        onChanged={() => undefined}
        promoteChangeBaseline={unusedAction}
        refreshActivity={async () => undefined}
        visibility={visibility}
        watchRandom={() => 1}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Start watch" }));
    expect(loadChangeRequestDetail).toHaveBeenCalledOnce();
    expect(clock.scheduledIntervals).toContain(2_200);
    act(() => clock.advanceBy(60_000));

    expect(
      screen.getByText("Watch stopped after its 60-second bound."),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Start watch" })).toBeEnabled();
    expect(loadChangeRequestDetail).toHaveBeenCalledOnce();
  });

  it("renders the exact rollout authorization accepted by the server", async () => {
    const user = userEvent.setup();
    const authorizedDetail = {
      ...detail,
      summary: {
        ...detail.summary,
        approvalCount: 1,
        lifecycle: "authorized",
        requiredApprovals: 1,
      },
    };
    const authorizeChangeRequest = vi.fn().mockResolvedValue({
      ...actionResult("rollout_authorized", authorizedDetail),
      authorization: {
        attemptLimit: 2,
        authorizedAt: "2032-03-04T05:06:00Z",
        authorizedBy: "operator-a",
        changeRequestId: "change-prod",
        executionWindows: [{
          durationMinutes: 45,
          startMinuteUtc: 120,
          weekdays: [1, 3, 5],
        }],
        fleet: "production",
        id: "rollout-prod",
        justification: "CHG-404",
        maxConcurrency: 1,
        validFrom: "2032-03-04T05:00:00Z",
        validUntil: "2032-03-05T05:00:00Z",
      },
    });
    const unusedAction = vi.fn().mockRejectedValue(new Error("unused"));

    render(
      <ChangeRequestDetail
        authorizeChangeRequest={authorizeChangeRequest}
        changeRequestLifecycle={unusedAction}
        chooseBaselineAdoptionPlan={unusedAction}
        createBaselineAdoption={unusedAction}
        detail={detail}
        loadChangeRequestDetail={async () => detail}
        onChanged={() => undefined}
        promoteChangeBaseline={unusedAction}
        refreshActivity={async () => undefined}
        watchRandom={() => 0.5}
      />,
    );

    const authorize = screen.getByRole("group", { name: "Authorize rollout" });
    await user.type(within(authorize).getByLabelText("Justification"), "CHG-404");
    await user.clear(within(authorize).getByLabelText("Attempt limit"));
    await user.type(within(authorize).getByLabelText("Attempt limit"), "2");
    await user.type(within(authorize).getByLabelText("Confirm Change request ID"), "change-prod");
    await user.click(within(authorize).getByRole("button", { name: "Record approval" }));

    const accepted = await screen.findByRole("region", {
      name: "Accepted rollout authorization",
    });
    expect(within(accepted).getByText("rollout-prod")).toBeVisible();
    expect(within(accepted).getByText("2 attempts · 1 concurrent")).toBeVisible();
    expect(within(accepted).getByText("Mon, Wed, Fri · 02:00 UTC · 45 minutes")).toBeVisible();
    expect(within(accepted).getByText("2032-03-04T05:00:00Z → 2032-03-05T05:00:00Z")).toBeVisible();
    expect(within(accepted).getByText("operator-a · 2032-03-04T05:06:00Z")).toBeVisible();
  });
});
