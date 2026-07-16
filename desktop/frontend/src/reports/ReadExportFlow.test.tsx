// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(cleanup);

const loadedAt = "2026-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const endpoint = {
  compliance: "compliant",
  desiredAgentVersion: "v2.1.0",
  endpointId: "endpoint-alpha",
  evidenceAt: loadedAt,
  fleet: "production",
  freshness: "recent",
  labels: [],
  releaseRef: "release-42",
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

describe("read and export parity workspace", () => {
  it("renders bounded structured reports and uses native save actions", async () => {
    const user = userEvent.setup();
    const loadAssetInventory = vi.fn().mockResolvedValue({
      omittedEndpointIds: [],
      rows: [
        {
          agentVersion: "v2.1.0",
          cpu: "AMD EPYC (4 cores)",
          diskEncryption: "1/1",
          endpointId: "endpoint-alpha",
          fleet: "production",
          kernel: "6.12.0",
          lastCheckIn: "2026-03-04T05:04:07Z",
          macAddress: "02:00:00:00:00:10",
          os: "Debian 13",
          primaryIp: "192.0.2.10",
          ram: "8 GiB",
          tpm: "present (version 2.0)",
        },
      ],
      section: ready,
    });
    const saveAssetInventory = vi.fn().mockResolvedValue({
      path: "/chosen/remotr-inventory.csv",
      sizeBytes: 256,
      status: "saved",
    });
    const loadFleetOperationalReports = vi.fn().mockResolvedValue({
      fleet: "production",
      schedules: [
        {
          applicable: true,
          endpointId: "endpoint-alpha",
          lastCompletedAt: "2026-03-04T02:00:05Z",
          lastMessage: "completed",
          lastScheduledFor: "2026-03-04T02:00:00Z",
          lastStatus: "success",
          name: "daily-audit",
          schedule: "0 2 * * *",
        },
      ],
      sections: { schedules: ready, state: ready },
      states: [
        {
          digest: "digest-alpha",
          endpointId: "endpoint-alpha",
          items: [
            {
              address: "base/packages",
              description: "Required packages",
              desiredSummary: "curl installed",
              name: "Packages",
              observedSummary: "curl 8.7.1",
              provider: "packages",
              reasonCode: "matches",
              status: "compliant",
              subresults: [],
              subresultsTruncated: false,
            },
          ],
          releaseRef: "release-42",
          reportedAt: loadedAt,
          status: "compliant",
        },
      ],
    });
    const loadFirewallReport = vi.fn().mockResolvedValue({
      audit: [
        {
          action: "allow",
          backend: "nftables",
          enforced: true,
          ports: [22],
          protocol: "tcp",
          ruleName: "allow-ssh",
          sources: ["192.0.2.0/24"],
          timestamp: "2026-03-04T05:03:07Z",
          wouldHave: "accept",
        },
      ],
      endpointId: "endpoint-alpha",
      live: {
        backend: "firewalld",
        defaultZone: "public",
        ruleset: "",
        rulesetTruncated: false,
        zones: [
          {
            name: "public",
            ports: ["8443/tcp"],
            richRules: [],
            services: ["ssh"],
            sources: ["192.0.2.0/24"],
            target: "default",
          },
        ],
      },
      sections: { audit: ready, live: ready },
    });
    const saveFirewallReport = vi.fn().mockResolvedValue({
      path: "/chosen/endpoint-alpha-firewall.csv",
      sizeBytes: 128,
      status: "saved",
    });
    const loadAuditExportInfo = vi.fn().mockResolvedValue({
      exportPath: "/v1/admin/audit-export/events",
      pathKey: "siem-v1",
    });
    const loadDiagnosticRequest = vi.fn().mockResolvedValue({
      collectors: ["system_info"],
      completedAt: "2026-03-04T05:06:08Z",
      createdAt: "2026-03-04T05:05:08Z",
      dispatchedAt: "2026-03-04T05:05:18Z",
      endpointId: "endpoint-alpha",
      errorMessage: "",
      expiresAt: "2026-03-05T05:05:08Z",
      requestId: "diagnostic-42",
      requestedBy: "operator-a",
      sha256: "a".repeat(64),
      since: "2026-03-03T05:05:07Z",
      sizeBytes: 128,
      status: "ready",
      until: "2026-03-04T05:05:07Z",
    });

    render(
      <App
        connection={{
          operatorId: "operator-a",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        loadAssetInventory={loadAssetInventory}
        loadAuditExportInfo={loadAuditExportInfo}
        loadDiagnosticRequest={loadDiagnosticRequest}
        loadFirewallReport={loadFirewallReport}
        loadFleetOperationalReports={loadFleetOperationalReports}
        saveAssetInventory={saveAssetInventory}
        saveFirewallReport={saveFirewallReport}
        workspace={workspace}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Reports" }));
    const reports = screen.getByRole("region", {
      name: "Read and export reports",
    });

    await user.click(
      within(reports).getByRole("button", { name: "Load asset inventory" }),
    );
    const inventory = await within(reports).findByRole("table", {
      name: "Asset inventory",
    });
    for (const value of [
      "endpoint-alpha",
      "Debian 13",
      "AMD EPYC (4 cores)",
      "192.0.2.10",
      "1/1",
    ]) {
      expect(within(inventory).getByText(value)).toBeVisible();
    }
    await user.click(
      within(reports).getByRole("button", { name: "Save inventory as CSV" }),
    );
    expect(saveAssetInventory).toHaveBeenCalledWith("csv");
    expect(await within(reports).findByText("/chosen/remotr-inventory.csv")).toBeVisible();

    await user.selectOptions(
      within(reports).getByRole("combobox", { name: "Fleet report scope" }),
      "production",
    );
    await user.click(
      within(reports).getByRole("button", { name: "Load Fleet reports" }),
    );
    expect(await within(reports).findByText("curl installed")).toBeVisible();
    expect(within(reports).getByText("daily-audit")).toBeVisible();

    await user.selectOptions(
      within(reports).getByRole("combobox", { name: "Firewall Endpoint" }),
      "endpoint-alpha",
    );
    await user.click(
      within(reports).getByRole("button", { name: "Load Firewall report" }),
    );
    expect(await within(reports).findByText("allow-ssh")).toBeVisible();
    expect(within(reports).getByText("firewalld")).toBeVisible();
    expect(within(reports).getByText("8443/tcp")).toBeVisible();
    await user.click(
      within(reports).getByRole("button", { name: "Save Firewall as CSV" }),
    );
    expect(saveFirewallReport).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      format: "csv",
    });

    await user.click(
      within(reports).getByRole("button", { name: "Load audit export info" }),
    );
    expect(await within(reports).findByText("/v1/admin/audit-export/events")).toBeVisible();
    expect(within(reports).getByText("siem-v1")).toBeVisible();

    await user.type(
      within(reports).getByRole("textbox", { name: "Diagnostic request ID" }),
      "diagnostic-42",
    );
    await user.click(
      within(reports).getByRole("button", {
        name: "Load diagnostic lifecycle",
      }),
    );
    const lifecycle = await within(reports).findByRole("status", {
      name: "Diagnostic lifecycle diagnostic-42",
    });
    expect(lifecycle).toHaveTextContent("ready");
    expect(lifecycle).toHaveTextContent("2026-03-04T05:06:08Z");
    expect(lifecycle).toHaveTextContent("128 bytes");
  });
});
