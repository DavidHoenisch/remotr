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
    const bridge = createWailsBridge({
      ClearEnrollmentToken: clearEnrollmentToken,
      CopyEnrollmentToken: copyEnrollmentToken,
      CreateEnrollmentToken: createEnrollmentToken,
      GetApplicationInfo: getApplicationInfo,
      RemoveEndpointLabel: removeEndpointLabel,
      RequestEndpointAgentUpgrade: requestEndpointAgentUpgrade,
      RequestGitSync: requestGitSync,
      SetEndpointLabel: setEndpointLabel,
    });

    await expect(bridge.getApplicationInfo()).resolves.toEqual({
      name: "Remotr Desktop",
      version: "v1.2.3",
    });
    expect(getApplicationInfo).toHaveBeenCalledOnce();

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
