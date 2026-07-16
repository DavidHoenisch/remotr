// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppShell, type AppPage } from "./AppShell";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const pages: Array<{ name: string; page: AppPage }> = [
  { name: "Overview", page: "overview" },
  { name: "Endpoints", page: "endpoints" },
  { name: "Fleets", page: "fleets" },
  { name: "Change requests", page: "change-requests" },
  { name: "Diagnostics", page: "diagnostics" },
  { name: "Activity", page: "activity" },
];

function ShellHarness() {
  const [showDetail, setShowDetail] = useState(true);

  return (
    <AppShell
      connection={{
        operatorId: "operator-1",
        profileName: "Production",
        serverLabel: "remotr.example:8443",
      }}
      fleetScope="production"
      overlay={
        showDetail
          ? {
              title: "Endpoint endpoint-a",
              content: <p>Focused Endpoint evidence</p>,
              onClose: () => setShowDetail(false),
            }
          : undefined
      }
      renderPage={(page) => (
        <section aria-label={`${page} data surface`}>
          <label>
            Filter current page
            <input type="search" />
          </label>
          <button type="button">Row actions</button>
        </section>
      )}
    />
  );
}

describe("AppShell", () => {
  it("preserves connection and Fleet context across grouped page navigation", async () => {
    const user = userEvent.setup();
    const openWindow = vi.spyOn(window, "open");
    render(<ShellHarness />);

    const navigation = screen.getByRole("navigation", {
      name: "Primary navigation",
    });
    expect(
      within(navigation).getByRole("heading", { name: "Fleet management" }),
    ).toBeVisible();
    expect(
      within(navigation).getByRole("heading", { name: "Operations" }),
    ).toBeVisible();

    const profileContext = screen.getByText("Production");
    const fleetContext = screen.getByText("Fleet: production");
    const main = screen.getByRole("main");

    for (const page of pages) {
      await user.click(
        within(navigation).getByRole("button", { name: page.name }),
      );
      expect(
        within(main).getByRole("heading", { level: 1, name: page.name }),
      ).toBeVisible();
      expect(profileContext).toBeVisible();
      expect(fleetContext).toBeVisible();
      expect(screen.getAllByRole("main")).toHaveLength(1);
    }

    expect(openWindow).not.toHaveBeenCalled();
  });

  it("keeps critical controls accessible at the 1100 by 720 minimum", async () => {
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 1100,
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 720,
    });
    window.dispatchEvent(new Event("resize"));

    const user = userEvent.setup();
    render(<ShellHarness />);

    const navigation = screen.getByRole("navigation", {
      name: "Primary navigation",
    });
    for (const page of pages) {
      const control = within(navigation).getByRole("button", {
        name: page.name,
      });
      expect(control).toBeVisible();
      expect(control).toBeEnabled();
    }

    expect(screen.getByRole("searchbox", { name: "Filter current page" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Row actions" })).toBeVisible();
    const dialog = screen.getByRole("dialog", {
      name: "Endpoint endpoint-a",
    });
    const close = within(dialog).getByRole("button", {
      name: "Close Endpoint endpoint-a",
    });
    expect(close).toBeVisible();

    close.focus();
    expect(close).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
