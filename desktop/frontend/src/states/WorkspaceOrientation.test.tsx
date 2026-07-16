// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const loadedAt = "2032-03-04T05:00:00Z";
const ready = { snapshot: { loadedAt }, state: "ready" };

describe("workspace orientation", () => {
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
