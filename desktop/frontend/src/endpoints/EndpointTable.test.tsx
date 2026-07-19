// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { EndpointTable } from "./EndpointTable";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const canonicalRows = [
  {
    endpointId: "endpoint-compliant",
    fleet: "production",
    usernames: ["alice"],
    compliance: "compliant",
    freshness: "stale",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-42",
    labels: [
      { key: "owner", value: "platform" },
      { key: "region", value: "us-west" },
      { key: "environment", value: "prod" },
    ],
    evidenceAt: "2032-03-04T05:04:07Z",
  },
  {
    endpointId: "endpoint-drifted",
    fleet: "production",
    usernames: ["bob"],
    compliance: "drifted",
    freshness: "recent",
    desiredAgentVersion: "v2.0.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-42",
    labels: [{ key: "region", value: "us-east" }],
    evidenceAt: "2032-03-04T05:03:07Z",
  },
  {
    endpointId: "endpoint-unsupported",
    fleet: "legacy",
    usernames: [],
    compliance: "unsupported",
    freshness: "never_reported",
    desiredAgentVersion: "v2.0.0",
    reportedAgentVersion: "v1.4.0",
    releaseRef: "release-39",
    labels: [{ key: "environment", value: "legacy" }],
  },
  {
    endpointId: "endpoint-check-failed",
    fleet: "production",
    usernames: ["carol"],
    compliance: "check_failed",
    freshness: "recent",
    desiredAgentVersion: "v2.0.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-42",
    labels: [],
    evidenceAt: "2032-03-04T05:02:07Z",
  },
  {
    endpointId: "endpoint-deferred",
    fleet: "staging",
    usernames: ["dana"],
    compliance: "deferred",
    freshness: "recent",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-43",
    labels: [{ key: "environment", value: "staging" }],
    evidenceAt: "2032-03-04T05:01:07Z",
  },
  {
    endpointId: "endpoint-apply-failed",
    fleet: "staging",
    usernames: ["erin"],
    compliance: "apply_failed",
    freshness: "stale",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-43",
    labels: [{ key: "region", value: "eu-central" }],
    evidenceAt: "2032-03-04T04:41:07Z",
  },
  {
    endpointId: "endpoint-no-report",
    fleet: "new-fleet",
    usernames: ["frank"],
    compliance: "not_reported",
    freshness: "recent",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.1.0",
    releaseRef: "release-44",
    labels: [
      { key: "region", value: "ap-south" },
      { key: "environment", value: "prod" },
    ],
    evidenceAt: "2032-03-04T05:00:07Z",
  },
];

function rowFor(endpointId: string): HTMLElement {
  const identityCell = screen.getByRole("cell", { name: endpointId });
  const row = identityCell.closest("tr");
  if (!row) {
    throw new Error(`Endpoint ${endpointId} is not rendered in a table row`);
  }
  return row;
}

describe("EndpointTable", () => {
  it("renders every canonical status and independent inventory evidence", async () => {
    const user = userEvent.setup();
    const onOpenEndpoint = vi.fn();

    render(
      <EndpointTable
        endpoints={canonicalRows}
        labelColumns={["environment", "region"]}
        onOpenEndpoint={onOpenEndpoint}
      />,
    );

    const table = screen.getByRole("table", { name: "Endpoints" });
    expect(table).toBeVisible();
    expect(screen.getByText("7 Endpoints")).toBeVisible();
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((header) => header.textContent?.trim()),
    ).toEqual([
      "Endpoint",
      "Fleet",
      "Compliance",
      "Check-in freshness",
      "Reported agent",
      "Desired agent",
      "Active Release",
      "Environment",
      "Region",
      "Last evidence",
      "Actions",
    ]);

    await user.click(
      screen.getByRole("button", { name: "Choose columns" }),
    );
    const regionColumn = screen.getByRole("checkbox", { name: "Region" });
    expect(regionColumn).toBeChecked();
    await user.click(regionColumn);
    expect(
      within(table).queryByRole("columnheader", { name: "Region" }),
    ).not.toBeInTheDocument();
    expect(within(table).queryByText("us-west")).not.toBeInTheDocument();
    await user.click(regionColumn);
    expect(
      within(table).getByRole("columnheader", { name: "Region" }),
    ).toBeVisible();

    const expectedStatuses: Array<[string, string, string]> = [
      ["endpoint-compliant", "Compliant", "Stale"],
      ["endpoint-drifted", "Drifted", "Recent"],
      ["endpoint-unsupported", "Unsupported", "Never reported"],
      ["endpoint-check-failed", "Check failed", "Recent"],
      ["endpoint-deferred", "Deferred", "Recent"],
      ["endpoint-apply-failed", "Apply failed", "Stale"],
      ["endpoint-no-report", "Not reported", "Recent"],
    ];
    for (const [endpointId, compliance, freshness] of expectedStatuses) {
      const row = within(rowFor(endpointId));
      expect(row.getByText(compliance)).toBeVisible();
      expect(row.getByText(freshness)).toBeVisible();
    }

    const compliant = within(rowFor("endpoint-compliant"));
    expect(compliant.getByText("v2.0.0")).toBeVisible();
    expect(compliant.getByText("v2.1.0")).toBeVisible();
    expect(compliant.getByText("release-42")).toBeVisible();
    expect(compliant.getByText("prod")).toBeVisible();
    expect(compliant.getByText("us-west")).toBeVisible();
    expect(compliant.queryByText("platform")).not.toBeInTheDocument();
    expect(compliant.getByText("2032-03-04 05:04 UTC")).toBeVisible();

    const noReport = within(rowFor("endpoint-no-report"));
    expect(noReport.getByText("Not reported")).toBeVisible();
    expect(noReport.getByText("Recent")).toBeVisible();
    expect(noReport.queryByText("Healthy")).not.toBeInTheDocument();
    expect(noReport.queryByText("Offline")).not.toBeInTheDocument();

    await user.click(
      compliant.getByRole("button", {
        name: "Inspect endpoint-compliant",
      }),
    );
    expect(onOpenEndpoint).toHaveBeenCalledOnce();
    expect(onOpenEndpoint).toHaveBeenCalledWith("endpoint-compliant");
  });

  it("distinguishes an authenticated empty inventory from connection failure", async () => {
    const user = userEvent.setup();
    const onCreateEnrollmentToken = vi.fn();

    render(
      <EndpointTable
        endpoints={[]}
        labelColumns={["environment", "region"]}
        onCreateEnrollmentToken={onCreateEnrollmentToken}
        onOpenEndpoint={() => {}}
      />,
    );

    const empty = screen.getByRole("status", {
      name: "No Endpoints enrolled",
    });
    expect(
      within(empty).getByText(
        "Connected successfully. No Endpoints are enrolled in this Fleet scope.",
      ),
    ).toBeVisible();
    expect(screen.queryByText("Connection failed")).not.toBeInTheDocument();

    await user.click(
      within(empty).getByRole("button", {
        name: "Create enrollment token",
      }),
    );
    expect(onCreateEnrollmentToken).toHaveBeenCalledOnce();
  });
});
