// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  BridgeProvider,
  type GeneratedBindings,
  createWailsBridge,
  useDesktopBridge,
} from "./desktopBridge";
import { createBridgeFixture } from "../testing/bridgeFixture";

function ApplicationVersion() {
  const bridge = useDesktopBridge();

  return (
    <button
      type="button"
      onClick={async () => {
        const info = await bridge.getApplicationInfo();
        document.body.dataset.applicationVersion = info.version;
      }}
    >
      Read version
    </button>
  );
}

function WorkspaceSummary() {
  const bridge = useDesktopBridge();
  const [summary, setSummary] = useState("Workspace not loaded");

  return (
    <>
      <button
        type="button"
        onClick={async () => {
          const workspace = await bridge.loadWorkspace();
          setSummary(`Endpoint section: ${workspace.sections.endpoints.state}`);
        }}
      >
        Load workspace
      </button>
      <p role="status">{summary}</p>
    </>
  );
}

describe("desktop bridge", () => {
  it("loads the authenticated startup workspace through the native bridge", async () => {
    const user = userEvent.setup();
    const section = {
      snapshot: { loadedAt: "2032-03-04T05:06:07Z" },
      state: "empty",
    };
    const loadWorkspace = vi.fn().mockResolvedValue({
      activity: [],
      activityNextCursor: "",
      changeRequests: [],
      endpoints: [],
      fleets: [],
      operator: { operatorId: "operator-b8108f", roles: ["operator"] },
      sections: {
        activity: section,
        changeRequests: section,
        endpoints: section,
        fleets: section,
        state: section,
      },
      stateEvidence: [],
    });
    const bridge = createWailsBridge({
      LoadWorkspace: loadWorkspace,
    } as unknown as GeneratedBindings);

    render(
      <BridgeProvider bridge={bridge}>
        <WorkspaceSummary />
      </BridgeProvider>,
    );

    await user.click(screen.getByRole("button", { name: "Load workspace" }));
    expect((await screen.findByRole("status")).textContent).toBe(
      "Endpoint section: empty",
    );
  });

  it("adapts the generated Wails binding to the application interface", async () => {
    const getApplicationInfo = vi.fn().mockResolvedValue({
      name: "Remotr Desktop",
      version: "v1.2.3",
    });
    const requestGitSync = vi.fn().mockResolvedValue({
      acceptedAt: "2032-03-04T05:06:07Z",
      action: "git_sync",
      affectedEvidence: ["release_ref", "activity"],
      profileName: "Production",
      status: "accepted",
      summary: "Server accepted Git sync for the Production profile.",
      target: "config-repo",
    });
    const clearEnrollmentToken = vi.fn().mockResolvedValue(undefined);
    const copyEnrollmentToken = vi.fn().mockResolvedValue(undefined);
    const createEnrollmentToken = vi.fn().mockResolvedValue({
      expiresAt: "2032-03-05T05:05:07Z",
      fleet: "production",
      token: "one-time-token",
    });
    const removeEndpointLabel = vi.fn().mockResolvedValue({
      effect: "removed",
      endpointId: "endpoint-alpha",
      key: "region",
      labels: [{ key: "environment", value: "production" }],
      value: "",
    });
    const removeEndpoint = vi.fn().mockResolvedValue({
      affectedEvidence: ["inventory", "activity"],
      credentialStatus: "not_enrolled",
      endpointId: "endpoint-alpha",
      status: "removed",
    });
    const setEndpointLabel = vi.fn().mockResolvedValue({
      effect: "added",
      endpointId: "endpoint-alpha",
      key: "site",
      labels: [{ key: "site", value: "berlin" }],
      value: "berlin",
    });
    const requestEndpointAgentUpgrade = vi.fn().mockResolvedValue({
      affectedEvidence: [
        "desired_agent_version",
        "reported_agent_version",
        "activity",
      ],
      endpointId: "endpoint-alpha",
      status: "requested",
      version: "v2.2.0",
    });
    const requestFleetAgentUpgrade = vi.fn().mockResolvedValue({
      acceptedEndpoints: 3,
      fleet: "production",
      status: "requested",
      version: "v2.2.0",
    });
    const getDiagnosticCapabilities = vi.fn().mockResolvedValue({
      collectors: ["system_info", "network_state"],
      maxTimeSpanSeconds: 604800,
    });
    const requestDiagnosticCollection = vi.fn().mockResolvedValue({
      collectors: ["system_info"],
      endpointId: "endpoint-alpha",
      requestId: "diagnostic-42",
      since: "2026-03-03T05:05:07Z",
      status: "pending",
      until: "2026-03-04T05:05:07Z",
    });
    const saveDiagnosticBundle = vi.fn().mockResolvedValue({
      path: "/home/operator/Downloads/diagnostic-42.tar.gz",
      sizeBytes: 4096,
      status: "saved",
    });
    const emptySection = {
      snapshot: { loadedAt: "2032-03-04T05:06:07Z" },
      state: "empty",
    };
    const loadAssetInventory = vi.fn().mockResolvedValue({
      omittedEndpointIds: [],
      rows: [],
      section: emptySection,
    });
    const loadAuditExportInfo = vi.fn().mockResolvedValue({
      exportPath: "/v1/admin/audit-export/events",
      pathKey: "siem-v1",
    });
    const loadDiagnosticRequest = vi.fn().mockResolvedValue({
      collectors: ["system_info"],
      completedAt: "",
      createdAt: "2032-03-04T05:06:07Z",
      dispatchedAt: "",
      endpointId: "endpoint-alpha",
      errorMessage: "",
      expiresAt: "2032-03-05T05:06:07Z",
      requestId: "diagnostic-42",
      requestedBy: "operator-a",
      sha256: "",
      since: "2032-03-03T05:06:07Z",
      sizeBytes: 0,
      status: "pending",
      until: "2032-03-04T05:06:07Z",
    });
    const loadFirewallReport = vi.fn().mockResolvedValue({
      audit: [],
      endpointId: "endpoint-alpha",
      live: {
        backend: "firewalld",
        defaultZone: "public",
        ruleset: "",
        rulesetTruncated: false,
        zones: [],
      },
      sections: { audit: emptySection, live: emptySection },
    });
    const loadFleetOperationalReports = vi.fn().mockResolvedValue({
      fleet: "production",
      schedules: [],
      sections: { schedules: emptySection, state: emptySection },
      states: [],
    });
    const saveAssetInventory = vi.fn().mockResolvedValue({
      path: "/chosen/inventory.csv",
      sizeBytes: 64,
      status: "saved",
    });
    const saveFirewallReport = vi.fn().mockResolvedValue({ status: "canceled" });
    const deploymentToken = {
      createdAt: "2032-03-01T05:05:07Z",
      expiresAt: "2032-03-05T05:05:07Z",
      fleet: "production",
      id: "deployment-prod",
      label: "prod",
      lastUsedAt: "",
      revokedAt: "",
      status: "active",
    };
    const clearDeploymentToken = vi.fn().mockResolvedValue(undefined);
    const copyDeploymentToken = vi.fn().mockResolvedValue(undefined);
    const createDeploymentToken = vi.fn().mockResolvedValue({
      metadata: deploymentToken,
      token: "view-once-token",
    });
    const listDeploymentTokens = vi.fn().mockResolvedValue([deploymentToken]);
    const loadDeploymentToken = vi.fn().mockResolvedValue(deploymentToken);
    const revokeDeploymentToken = vi.fn().mockResolvedValue({
      ...deploymentToken,
      revokedAt: "2032-03-04T05:05:07Z",
      status: "revoked",
    });
    const saveDeploymentToken = vi.fn().mockResolvedValue({
      path: "/chosen/prod.token",
      sizeBytes: 32,
      status: "saved",
    });
    const bridge = createWailsBridge({
      ClearDeploymentToken: clearDeploymentToken,
      ClearEnrollmentToken: clearEnrollmentToken,
      CopyDeploymentToken: copyDeploymentToken,
      CopyEnrollmentToken: copyEnrollmentToken,
      CreateDeploymentToken: createDeploymentToken,
      CreateEnrollmentToken: createEnrollmentToken,
      GetApplicationInfo: getApplicationInfo,
      GetDiagnosticCapabilities: getDiagnosticCapabilities,
      ListDeploymentTokens: listDeploymentTokens,
      LoadAssetInventory: loadAssetInventory,
      LoadAuditExportInfo: loadAuditExportInfo,
      LoadDiagnosticRequest: loadDiagnosticRequest,
      LoadDeploymentToken: loadDeploymentToken,
      LoadFirewallReport: loadFirewallReport,
      LoadFleetOperationalReports: loadFleetOperationalReports,
      RemoveEndpoint: removeEndpoint,
      RemoveEndpointLabel: removeEndpointLabel,
      RequestEndpointAgentUpgrade: requestEndpointAgentUpgrade,
      RequestDiagnosticCollection: requestDiagnosticCollection,
      RequestFleetAgentUpgrade: requestFleetAgentUpgrade,
      RequestGitSync: requestGitSync,
      RevokeDeploymentToken: revokeDeploymentToken,
      SaveAssetInventory: saveAssetInventory,
      SaveDiagnosticBundle: saveDiagnosticBundle,
      SaveDeploymentToken: saveDeploymentToken,
      SaveFirewallReport: saveFirewallReport,
      SetEndpointLabel: setEndpointLabel,
    });

    await expect(bridge.getApplicationInfo()).resolves.toEqual({
      name: "Remotr Desktop",
      version: "v1.2.3",
    });
    expect(getApplicationInfo).toHaveBeenCalledOnce();

    await expect(bridge.getDiagnosticCapabilities()).resolves.toEqual({
      collectors: ["system_info", "network_state"],
      maxTimeSpanSeconds: 604800,
    });
    expect(getDiagnosticCapabilities).toHaveBeenCalledOnce();

    await expect(bridge.requestGitSync()).resolves.toEqual({
      acceptedAt: "2032-03-04T05:06:07Z",
      action: "git_sync",
      affectedEvidence: ["release_ref", "activity"],
      summary: "Server accepted Git sync for the Production profile.",
      target: "config-repo",
    });
    expect(requestGitSync).toHaveBeenCalledOnce();

    await expect(
      bridge.createEnrollmentToken({ fleet: "production", ttlSeconds: 3600 }),
    ).resolves.toEqual({
      expiresAt: "2032-03-05T05:05:07Z",
      fleet: "production",
      token: "one-time-token",
    });
    expect(createEnrollmentToken).toHaveBeenCalledWith({
      fleet: "production",
      ttlSeconds: 3600,
    });
    await bridge.copyEnrollmentToken();
    await bridge.clearEnrollmentToken();
    expect(copyEnrollmentToken).toHaveBeenCalledOnce();
    expect(copyEnrollmentToken).toHaveBeenCalledWith();
    expect(clearEnrollmentToken).toHaveBeenCalledOnce();

    await expect(
      bridge.setEndpointLabel({
        endpointId: "endpoint-alpha",
        key: "site",
        value: "berlin",
      }),
    ).resolves.toEqual({
      effect: "added",
      endpointId: "endpoint-alpha",
      key: "site",
      labels: [{ key: "site", value: "berlin" }],
      value: "berlin",
    });
    await expect(
      bridge.removeEndpointLabel({
        endpointId: "endpoint-alpha",
        key: "region",
      }),
    ).resolves.toEqual({
      effect: "removed",
      endpointId: "endpoint-alpha",
      key: "region",
      labels: [{ key: "environment", value: "production" }],
      value: "",
    });
    expect(setEndpointLabel).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      key: "site",
      value: "berlin",
    });
    expect(removeEndpointLabel).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      key: "region",
    });

    await expect(
      bridge.removeEndpoint({
        confirmation: "endpoint-alpha",
        endpointId: "endpoint-alpha",
      }),
    ).resolves.toEqual({
      affectedEvidence: ["inventory", "activity"],
      credentialStatus: "not_enrolled",
      endpointId: "endpoint-alpha",
      status: "removed",
    });
    expect(removeEndpoint).toHaveBeenCalledOnce();
    expect(removeEndpoint).toHaveBeenCalledWith({
      confirmation: "endpoint-alpha",
      endpointId: "endpoint-alpha",
    });

    await expect(
      bridge.requestEndpointAgentUpgrade({
        endpointId: "endpoint-alpha",
        version: "v2.2.0",
      }),
    ).resolves.toEqual({
      affectedEvidence: [
        "desired_agent_version",
        "reported_agent_version",
        "activity",
      ],
      endpointId: "endpoint-alpha",
      status: "requested",
      version: "v2.2.0",
    });
    expect(requestEndpointAgentUpgrade).toHaveBeenCalledWith({
      endpointId: "endpoint-alpha",
      version: "v2.2.0",
    });

    await expect(
      bridge.requestFleetAgentUpgrade({
        fleet: "production",
        version: "v2.2.0",
      }),
    ).resolves.toEqual({
      acceptedEndpoints: 3,
      fleet: "production",
      status: "requested",
      version: "v2.2.0",
    });
    expect(requestFleetAgentUpgrade).toHaveBeenCalledWith({
      fleet: "production",
      version: "v2.2.0",
    });

    await expect(
      bridge.requestDiagnosticCollection({
        collectors: ["system_info"],
        endpointId: "endpoint-alpha",
        since: "2026-03-03T05:05:07Z",
        until: "2026-03-04T05:05:07Z",
      }),
    ).resolves.toEqual({
      collectors: ["system_info"],
      endpointId: "endpoint-alpha",
      requestId: "diagnostic-42",
      since: "2026-03-03T05:05:07Z",
      status: "pending",
      until: "2026-03-04T05:05:07Z",
    });
    expect(requestDiagnosticCollection).toHaveBeenCalledWith({
      collectors: ["system_info"],
      endpointId: "endpoint-alpha",
      since: "2026-03-03T05:05:07Z",
      until: "2026-03-04T05:05:07Z",
    });

    await expect(
      bridge.saveDiagnosticBundle("diagnostic-42"),
    ).resolves.toEqual({
      path: "/home/operator/Downloads/diagnostic-42.tar.gz",
      sizeBytes: 4096,
      status: "saved",
    });
    expect(saveDiagnosticBundle).toHaveBeenCalledOnce();
    expect(saveDiagnosticBundle).toHaveBeenCalledWith("diagnostic-42");
  });

  it("injects a deterministic fixture through the component seam", async () => {
    const bridge = createBridgeFixture({ version: "browser-fixture" });

    render(
      <BridgeProvider bridge={bridge}>
        <ApplicationVersion />
      </BridgeProvider>,
    );
    screen.getByRole("button", { name: "Read version" }).click();

    await expect.poll(() => document.body.dataset.applicationVersion).toBe(
      "browser-fixture",
    );
  });
});
