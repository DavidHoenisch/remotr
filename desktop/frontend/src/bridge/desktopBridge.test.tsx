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
    const bridge = createWailsBridge({
      GetApplicationInfo: getApplicationInfo,
      RequestGitSync: requestGitSync,
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
