// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  BridgeProvider,
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

describe("desktop bridge", () => {
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
    const bridge = createWailsBridge({
      ClearEnrollmentToken: clearEnrollmentToken,
      CopyEnrollmentToken: copyEnrollmentToken,
      CreateEnrollmentToken: createEnrollmentToken,
      GetApplicationInfo: getApplicationInfo,
      GetDiagnosticCapabilities: getDiagnosticCapabilities,
      RemoveEndpoint: removeEndpoint,
      RemoveEndpointLabel: removeEndpointLabel,
      RequestEndpointAgentUpgrade: requestEndpointAgentUpgrade,
      RequestDiagnosticCollection: requestDiagnosticCollection,
      RequestFleetAgentUpgrade: requestFleetAgentUpgrade,
      RequestGitSync: requestGitSync,
      SaveDiagnosticBundle: saveDiagnosticBundle,
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
