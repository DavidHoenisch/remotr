// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AIIntegrationPage } from "./AIIntegrationPage";

afterEach(cleanup);

const integrations = [
  {
    agent: "claude",
    bundleVersion: "1.0.0",
    displayName: "Claude Code",
    guidance: "",
    installed: true,
    runtimeAvailable: true,
    runtimeStatus: "available",
    scope: "user",
    source: "embedded",
    sourceVersion: "desktop-test",
  },
  {
    agent: "cursor",
    bundleVersion: "",
    displayName: "Cursor",
    guidance: "Install Cursor, then return here. The skill can be prepared before the runtime is available.",
    installed: false,
    runtimeAvailable: false,
    runtimeStatus: "not_found",
    scope: "user",
    source: "",
    sourceVersion: "",
  },
  {
    agent: "pi",
    bundleVersion: "",
    displayName: "Pi",
    guidance: "",
    installed: false,
    runtimeAvailable: true,
    runtimeStatus: "available",
    scope: "user",
    source: "",
    sourceVersion: "",
  },
];

describe("AI integration flow", () => {
  it("uses explicit scope, version, and replacement controls with missing-runtime recovery", async () => {
    const user = userEvent.setup();
    const listIntegrations = vi.fn().mockResolvedValue(integrations);
    const setupIntegration = vi.fn().mockResolvedValue({
      integration: { ...integrations[0], scope: "project" },
      status: "installed",
    });
    const upgradeIntegration = vi.fn().mockResolvedValue({
      integration: { ...integrations[0], bundleVersion: "2.0.0", scope: "project", source: "github", sourceVersion: "v2.0.0" },
      status: "upgraded",
    });

    render(
      <AIIntegrationPage
        chooseProjectRoot={async () => ({ directoryName: "selected-project", id: "opaque-project", status: "selected" })}
        listIntegrations={listIntegrations}
        setupIntegration={setupIntegration}
        upgradeIntegration={upgradeIntegration}
      />,
    );

    const page = screen.getByRole("region", { name: "AI integrations" });
    expect(await within(page).findByText("Claude Code")).toBeVisible();
    expect(within(page).getByText("Runtime not found")).toBeVisible();
    expect(within(page).getByText(/skill can be prepared before the runtime is available/i)).toBeVisible();
    expect(listIntegrations).toHaveBeenCalledWith({ projectRootId: "", scope: "user" });

    await user.selectOptions(within(page).getByRole("combobox", { name: "Installation scope" }), "project");
    await user.click(within(page).getByRole("button", { name: "Choose project directory" }));
    expect(await within(page).findByText("selected-project")).toBeVisible();
    expect(listIntegrations).toHaveBeenLastCalledWith({ projectRootId: "opaque-project", scope: "project" });

    await user.click(within(page).getByRole("button", { name: "Set up Claude Code" }));
    const setupDialog = screen.getByRole("dialog", { name: "Set up AI integration" });
    await user.click(within(setupDialog).getByRole("checkbox", { name: "Replace existing installation" }));
    await user.click(within(setupDialog).getByRole("button", { name: "Install integration" }));
    expect(setupIntegration).toHaveBeenCalledWith({ agent: "claude", projectRootId: "opaque-project", replace: true, scope: "project" });

    await user.click(within(page).getByRole("button", { name: "Upgrade Claude Code" }));
    const upgradeDialog = screen.getByRole("dialog", { name: "Upgrade AI integration" });
    await user.type(within(upgradeDialog).getByRole("textbox", { name: "Release version" }), "v2.0.0");
    await user.click(within(upgradeDialog).getByRole("checkbox", { name: "Replace existing installation" }));
    await user.click(within(upgradeDialog).getByRole("button", { name: "Download and upgrade" }));
    expect(upgradeIntegration).toHaveBeenCalledWith({ agent: "claude", projectRootId: "opaque-project", replace: true, scope: "project", version: "v2.0.0" });
  });
});
