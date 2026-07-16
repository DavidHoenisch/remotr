// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2032-03-04T05:00:00Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const configuredServerURL = ["https:", "/", "/remotr-b8108f.fly.dev:8443"].join("");

describe("workspace orientation", () => {
  it("connects the sole standard Operator profile on startup", async () => {
    const user = userEvent.setup();
    const profile = {
      caPath: "/home/operator/.config/remotr/remotr-b8108f/ca.crt",
      defaultFleet: "engineering",
      name: "Default",
      serverUrl: configuredServerURL,
      stateDir: "/home/operator/.config/remotr/remotr-b8108f",
    };
    const loadSetupMaintenance = vi.fn().mockResolvedValue({
      application: {
        architecture: "amd64",
        name: "Remotr Desktop",
        platform: "linux",
        version: "dev",
      },
      desktopProfilesPath: "/home/operator/.config/remotr/desktop-profiles.json",
      profiles: [profile],
      standardConfigPath: "/home/operator/.config/remotr/config.yaml",
    });
    const connectProfile = vi.fn().mockResolvedValue({
      operatorId: "operator-b8108f",
      profileName: "Default",
      roles: ["operator"],
      serverUrl: configuredServerURL,
    });
    const loadWorkspace = vi.fn().mockResolvedValue({
      activity: [],
      activityNextCursor: "",
      changeRequests: [],
      endpoints: [],
      fleets: [],
      sections: {
        activity: ready,
        changeRequests: ready,
        endpoints: { snapshot: { loadedAt }, state: "empty" },
        fleets: ready,
        state: ready,
      },
    });

    render(
      <App
        connectProfile={connectProfile}
        loadSetupMaintenance={loadSetupMaintenance}
        loadWorkspace={loadWorkspace}
      />,
    );

    const connection = within(screen.getByLabelText("Active connection"));
    await waitFor(() => expect(connection.getByText("Connected")).toBeVisible());
    expect(connection.getByText("Default")).toBeVisible();
    expect(
      connection.getByText(configuredServerURL),
    ).toBeVisible();
    expect(screen.getByText("Fleet: engineering")).toBeVisible();
    expect(connectProfile).toHaveBeenCalledWith(profile);
    await waitFor(() => expect(loadWorkspace).toHaveBeenCalledOnce());
    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    expect(screen.getByText("No Endpoints enrolled")).toBeVisible();
  });

  it("retains the standard profile when automatic connection fails", async () => {
    const user = userEvent.setup();
    const profile = {
      caPath: "/home/operator/.config/remotr/remotr-b8108f/ca.crt",
      defaultFleet: "engineering",
      name: "Default",
      serverUrl: configuredServerURL,
      stateDir: "/home/operator/.config/remotr/remotr-b8108f",
    };
    const loadSetupMaintenance = vi.fn().mockResolvedValue({
      application: {
        architecture: "amd64",
        name: "Remotr Desktop",
        platform: "linux",
        version: "dev",
      },
      desktopProfilesPath: "/home/operator/.config/remotr/desktop-profiles.json",
      profiles: [profile],
      standardConfigPath: "/home/operator/.config/remotr/config.yaml",
    });
    const connectProfile = vi.fn().mockRejectedValue({
      guidance: "Check the selected Operator credential directory.",
      kind: "credentials",
      message: "Operator credentials are unavailable.",
    });

    render(
      <App
        connectProfile={connectProfile}
        loadSetupMaintenance={loadSetupMaintenance}
      />,
    );

    const recovery = await screen.findByRole("region", {
      name: "Connection recovery",
    });
    expect(within(recovery).getByText("Default connection failed")).toBeVisible();
    expect(
      within(recovery).getByText("Operator credentials are unavailable."),
    ).toBeVisible();
    const connection = within(screen.getByLabelText("Active connection"));
    expect(connection.getByText("Not connected")).toBeVisible();
    expect(connection.getByText("Default")).toBeVisible();
    expect(
      connection.getByText(configuredServerURL),
    ).toBeVisible();
    await user.click(
      within(recovery).getByRole("button", { name: "Retry connection" }),
    );
    await waitFor(() => expect(connectProfile).toHaveBeenCalledTimes(2));
  });

  it("keeps the authenticated connection when the initial workspace load fails", async () => {
    const user = userEvent.setup();
    const profile = {
      caPath: "/home/operator/.config/remotr/ca.crt",
      defaultFleet: "engineering",
      name: "Default",
      serverUrl: configuredServerURL,
      stateDir: "/home/operator/.config/remotr",
    };
    const loadSetupMaintenance = vi.fn().mockResolvedValue({
      application: {
        architecture: "amd64",
        name: "Remotr Desktop",
        platform: "linux",
        version: "dev",
      },
      desktopProfilesPath: "/home/operator/.config/remotr/desktop-profiles.json",
      profiles: [profile],
      standardConfigPath: "/home/operator/.config/remotr/config.yaml",
    });
    const connectProfile = vi.fn().mockResolvedValue({
      operatorId: "operator-b8108f",
      profileName: "Default",
      roles: ["operator"],
      serverUrl: configuredServerURL,
    });
    const loadWorkspace = vi.fn().mockRejectedValue({
      guidance: "Retry the workspace load.",
      kind: "workspace",
      message: "Fleet sections are temporarily unavailable.",
    });

    render(
      <App
        connectProfile={connectProfile}
        loadSetupMaintenance={loadSetupMaintenance}
        loadWorkspace={loadWorkspace}
      />,
    );

    const connection = within(screen.getByLabelText("Active connection"));
    await waitFor(() => expect(connection.getByText("Connected")).toBeVisible());
    expect(connection.getByText("Default")).toBeVisible();
    const recovery = await screen.findByRole("region", {
      name: "Workspace recovery",
    });
    expect(within(recovery).getByText("Workspace load failed")).toBeVisible();
    expect(
      within(recovery).getByText("Fleet sections are temporarily unavailable."),
    ).toBeVisible();
    expect(
      within(recovery).queryByText("Default connection failed"),
    ).not.toBeInTheDocument();

    await user.click(
      within(recovery).getByRole("button", { name: "Retry workspace" }),
    );
    await waitFor(() => expect(loadWorkspace).toHaveBeenCalledTimes(2));
    expect(connectProfile).toHaveBeenCalledOnce();
  });

  it("distinguishes an authenticated zero-Endpoint inventory and offers enrollment", async () => {
    const user = userEvent.setup();
    const onCreateEnrollmentToken = vi.fn();
    render(
      <App
        connection={{
          operatorId: "operator-empty",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        onCreateEnrollmentToken={onCreateEnrollmentToken}
        workspace={{
          activity: [],
          activityNextCursor: "",
          changeRequests: [],
          endpoints: [],
          fleets: [],
          sections: {
            activity: ready,
            changeRequests: ready,
            endpoints: { snapshot: { loadedAt }, state: "empty" },
            fleets: ready,
            state: ready,
          },
        }}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    const empty = screen.getByRole("status");
    expect(within(empty).getByText("No Endpoints enrolled")).toBeVisible();
    expect(
      within(empty).getByText(
        "Connected successfully. No Endpoints are enrolled in this Fleet scope.",
      ),
    ).toBeVisible();
    expect(screen.getByText("Connected")).toBeVisible();
    expect(screen.queryByRole("table", { name: "Endpoints" })).not.toBeInTheDocument();

    await user.click(
      within(empty).getByRole("button", { name: "Create enrollment token" }),
    );
    expect(onCreateEnrollmentToken).toHaveBeenCalledOnce();
  });

  it("keeps selected-profile context and classified recovery on initial connection failure", async () => {
    const user = userEvent.setup();
    const onChooseProfile = vi.fn();
    const onRetryConnection = vi.fn();
    render(
      <App
        connection={{
          connected: false,
          operatorId: "No operator",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        onChooseProfile={onChooseProfile}
        onRetryConnection={onRetryConnection}
        workspaceFailure={{
          guidance: "Verify the selected profile and server certificate.",
          kind: "connection",
          message: "The selected profile could not reach the Remotr server.",
        }}
      />,
    );

    const recovery = screen.getByRole("region", { name: "Connection recovery" });
    expect(within(recovery).getByText("Production connection failed")).toBeVisible();
    expect(
      within(recovery).getByText(
        "The selected profile could not reach the Remotr server.",
      ),
    ).toBeVisible();
    expect(
      within(recovery).getByText(
        "Verify the selected profile and server certificate.",
      ),
    ).toBeVisible();
    const connection = within(screen.getByLabelText("Active connection"));
    expect(connection.getByText("Not connected")).toBeVisible();
    expect(connection.getByText("Production")).toBeVisible();
    expect(connection.getByText("remotr.example:8443")).toBeVisible();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(screen.queryByText(/demo|sample Endpoint/i)).not.toBeInTheDocument();

    await user.click(
      within(recovery).getByRole("button", { name: "Retry connection" }),
    );
    await user.click(
      within(recovery).getByRole("button", { name: "Choose another profile" }),
    );
    expect(onRetryConnection).toHaveBeenCalledOnce();
    expect(onChooseProfile).toHaveBeenCalledOnce();
  });
});
