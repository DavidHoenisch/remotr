// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SetupMaintenancePage } from "./SetupMaintenancePage";
import type { ConnectionProfile } from "./setupMaintenance";

afterEach(cleanup);

const productionURL = ["https:", "", "remotr.example:8443"].join("/");
const recoveryURL = ["https:", "", "recovery.example:8443"].join("/");

const production: ConnectionProfile = {
  caPath: "/etc/remotr/ca.crt",
  defaultFleet: "production",
  name: "Production",
  serverUrl: productionURL,
  stateDir: "/var/lib/remotr/operator",
};

describe("setup and maintenance flow", () => {
  it("manages safe profiles, bootstrap, doctor, docs, version, and truthful Linux updates", async () => {
    const user = userEvent.setup();
    const saveProfile = vi.fn().mockResolvedValue(undefined);
    const connectProfile = vi.fn().mockResolvedValue({
      operatorId: "operator-production",
      profileName: "Production",
      roles: ["global_admin"],
      serverUrl: production.serverUrl,
    });
    const bootstrapProfile = vi.fn().mockResolvedValue({
      operatorId: "operator-bootstrap",
      profileName: "Recovery",
      roles: ["global_admin"],
      serverUrl: recoveryURL,
    });
    const runDoctor = vi.fn().mockResolvedValue({
      checks: [
        { detail: production.serverUrl, guidance: "", name: "Server URL", status: "ok" },
        { detail: "Verified Operator identity", guidance: "", name: "Authenticated connection", status: "ok" },
      ],
      healthy: true,
      operatorId: "operator-production",
      profileName: "Production",
      roles: ["global_admin"],
    });
    const openDocumentation = vi.fn().mockResolvedValue(undefined);
    const checkDesktopUpdate = vi.fn().mockResolvedValue({
      currentVersion: "v9.0.0",
      guidance: "Install a verified Linux release package when one is published.",
      installSupported: false,
      latestVersion: "v9.1.0",
      platform: "linux/amd64",
      updateAvailable: true,
    });
    const onConnected = vi.fn();
    const onCreateEnrollmentToken = vi.fn();

    render(
      <SetupMaintenancePage
        bootstrapProfile={bootstrapProfile}
        checkDesktopUpdate={checkDesktopUpdate}
        connectProfile={connectProfile}
        loadSetup={async () => ({
          application: {
            architecture: "amd64",
            name: "Remotr Desktop",
            platform: "linux",
            version: "v9.0.0",
          },
          desktopProfilesPath: "/home/operator/.config/remotr-desktop/profiles.json",
          profiles: [production],
          standardConfigPath: "/home/operator/.config/remotr/config.yaml",
        })}
        onConnected={onConnected}
        onCreateEnrollmentToken={onCreateEnrollmentToken}
        openDocumentation={openDocumentation}
        runDoctor={runDoctor}
        saveProfile={saveProfile}
      />,
    );

    const page = await screen.findByRole("region", { name: "Setup and support" });
    expect(within(page).getByText("Remotr Desktop v9.0.0")).toBeVisible();
    expect(within(page).getByText("linux/amd64")).toBeVisible();
    expect(within(page).getByText("/home/operator/.config/remotr/config.yaml")).toBeVisible();
    expect(within(page).getByText("/home/operator/.config/remotr-desktop/profiles.json")).toBeVisible();

    await user.click(within(page).getByRole("button", { name: "Connect Production" }));
    expect(connectProfile).toHaveBeenCalledWith(production);
    expect(onConnected).toHaveBeenCalledWith(expect.objectContaining({ operatorId: "operator-production" }));

    await user.click(within(page).getByRole("button", { name: "Run doctor for Production" }));
    const doctor = await screen.findByRole("status", { name: "Doctor report" });
    expect(within(doctor).getByText("All setup checks passed")).toBeVisible();
    expect(runDoctor).toHaveBeenCalledWith(production);

    await user.click(within(page).getByRole("button", { name: "Check for desktop updates" }));
    const update = await screen.findByRole("status", { name: "Desktop update status" });
    expect(within(update).getByText(/v9\.1\.0 is available/)).toBeVisible();
    expect(within(update).getByText(/Install a verified Linux release package/)).toBeVisible();
    expect(within(update).queryByRole("button", { name: /install/i })).not.toBeInTheDocument();

    await user.click(within(page).getByRole("button", { name: "Open Remotr documentation" }));
    expect(openDocumentation).toHaveBeenCalledOnce();
    await user.click(within(page).getByRole("button", { name: "Create enrollment token" }));
    expect(onCreateEnrollmentToken).toHaveBeenCalledOnce();

    await user.click(within(page).getByRole("button", { name: "Add profile" }));
    const profileDialog = screen.getByRole("dialog", { name: "Add connection profile" });
    await user.type(within(profileDialog).getByRole("textbox", { name: "Profile name" }), "Recovery");
    await user.type(within(profileDialog).getByRole("textbox", { name: "Server URL" }), recoveryURL);
    await user.type(within(profileDialog).getByRole("textbox", { name: "Operator state directory" }), "/var/lib/remotr/recovery");
    await user.click(within(profileDialog).getByRole("button", { name: "Save profile" }));
    expect(saveProfile).toHaveBeenCalledWith({
      caPath: "",
      defaultFleet: "",
      name: "Recovery",
      serverUrl: recoveryURL,
      stateDir: "/var/lib/remotr/recovery",
    });

    await user.click(within(page).getByRole("button", { name: "Bootstrap profile" }));
    const bootstrapDialog = screen.getByRole("dialog", { name: "Bootstrap first Operator" });
    await user.type(within(bootstrapDialog).getByRole("textbox", { name: "Profile name" }), "Recovery");
    await user.type(within(bootstrapDialog).getByRole("textbox", { name: "Server URL" }), recoveryURL);
    await user.type(within(bootstrapDialog).getByRole("textbox", { name: "Operator state directory" }), "/var/lib/remotr/recovery");
    const token = within(bootstrapDialog).getByLabelText("One-time bootstrap token");
    await user.type(token, "bootstrap-token-canary");
    await user.click(within(bootstrapDialog).getByRole("button", { name: "Bootstrap and connect" }));
    expect(bootstrapProfile).toHaveBeenCalledWith(
      expect.objectContaining({ name: "Recovery" }),
      "bootstrap-token-canary",
    );
    expect(token).toHaveValue("");
    expect(onConnected).toHaveBeenCalledWith(expect.objectContaining({ operatorId: "operator-bootstrap" }));
  });
});
