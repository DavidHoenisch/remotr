// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "../shell/AppShell";
import { DataState, type DataStateKind } from "./DataState";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const cases: Array<{
  action?: string;
  kind: DataStateKind;
  message: string;
  role: "alert" | "status";
  title: string;
}> = [
  {
    kind: "loading",
    title: "Loading Endpoints",
    message: "Fetching the current inventory.",
    role: "status",
  },
  {
    kind: "empty",
    title: "No Endpoints enrolled",
    message: "The connection succeeded, but this scope has no Endpoints.",
    role: "status",
    action: "Create enrollment token",
  },
  {
    kind: "partial",
    title: "Endpoint evidence partially available",
    message: "Some sections could not be refreshed.",
    role: "status",
    action: "Retry failed sections",
  },
  {
    kind: "stale",
    title: "Endpoint evidence may be stale",
    message: "The last successful snapshot is still shown.",
    role: "status",
    action: "Refresh now",
  },
  {
    kind: "authorization",
    title: "Activity unavailable",
    message: "This Operator is not authorized to read audit Activity.",
    role: "status",
    action: "Review access requirements",
  },
  {
    kind: "connection",
    title: "Connection failed",
    message: "The selected profile could not reach the Remotr server.",
    role: "alert",
    action: "Reconnect",
  },
  {
    kind: "unexpected",
    title: "Unexpected error",
    message: "The request could not be completed safely.",
    role: "alert",
    action: "Try again",
  },
];

describe("DataState", () => {
  it("labels every state and exposes only relevant recovery controls", async () => {
    const user = userEvent.setup();
    const recover = vi.fn();

    render(
      <div>
        {cases.map((state) => (
          <DataState
            action={
              state.action
                ? {
                    label: state.action,
                    onAction: () => recover(state.kind),
                  }
                : undefined
            }
            key={state.kind}
            kind={state.kind}
            message={state.message}
            title={state.title}
          >
            <p>{state.kind} retained evidence</p>
          </DataState>
        ))}
      </div>,
    );

    for (const state of cases) {
      const surface = screen.getByRole(state.role, { name: state.title });
      expect(surface).toBeVisible();
      expect(within(surface).getByText(state.message)).toBeVisible();

      const retainedEvidence = within(surface).queryByText(
        `${state.kind} retained evidence`,
      );
      if (state.kind === "partial" || state.kind === "stale") {
        expect(retainedEvidence).toBeVisible();
      } else {
        expect(retainedEvidence).not.toBeInTheDocument();
      }

      if (state.action) {
        await user.click(
          within(surface).getByRole("button", { name: state.action }),
        );
      } else {
        expect(within(surface).queryByRole("button")).not.toBeInTheDocument();
      }
    }

    expect(recover.mock.calls).toEqual(
      cases
        .filter((state) => state.action)
        .map((state) => [state.kind]),
    );
  });

  it("keeps connection context while replacing untrusted operational content", async () => {
    const user = userEvent.setup();
    const reconnect = vi.fn();

    render(
      <AppShell
        connection={{
          connected: false,
          operatorId: "No operator",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        fleetScope="All Fleets"
        renderPage={() => (
          <DataState
            action={{ label: "Reconnect", onAction: reconnect }}
            kind="connection"
            message="Verify the server address, network, and profile credentials."
            title="Connection failed"
          >
            <p>fabricated-endpoint-row</p>
          </DataState>
        )}
      />,
    );

    expect(screen.getByText("Production")).toBeVisible();
    expect(screen.getByText("Fleet: All Fleets")).toBeVisible();
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }),
    ).toBeVisible();
    expect(
      screen.getByRole("heading", { level: 1, name: "Overview" }),
    ).toBeVisible();

    const main = screen.getByRole("main");
    const failure = within(main).getByRole("alert", {
      name: "Connection failed",
    });
    expect(within(failure).queryByText("fabricated-endpoint-row")).not.toBeInTheDocument();

    await user.click(within(failure).getByRole("button", { name: "Reconnect" }));
    expect(reconnect).toHaveBeenCalledOnce();
  });
});
