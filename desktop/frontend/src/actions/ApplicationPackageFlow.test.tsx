// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(cleanup);

const loadedAt = "2032-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" as const };
const workspace = {
  activity: [],
  activityNextCursor: "",
  changeRequests: [],
  endpoints: [],
  fleets: [],
  sections: {
    activity: ready,
    changeRequests: ready,
    endpoints: ready,
    fleets: ready,
    state: ready,
  },
};

const existing = {
  createdAt: loadedAt,
  id: "package-existing",
  installMode: "binary",
  name: "internal/existing",
  objectKey: "app-packages/internal/existing/9.0.0/archive.zip",
  sha256: "a".repeat(64),
  version: "9.0.0",
};

const archive = {
  fileName: "internal_tool-1.2.3.zip",
  mode: "binary",
  name: "internal/tool",
  sha256: "b".repeat(64),
  sizeBytes: 4096,
  source: "selected",
  version: "1.2.3",
};

describe("application package parity", () => {
  it("validates, publishes, inspects, deletes, scaffolds, and builds through typed native actions", async () => {
    const user = userEvent.setup();
    const listAppPackages = vi.fn().mockResolvedValue([existing]);
    const loadAppPackage = vi.fn().mockResolvedValue(existing);
    const chooseAppPackageArchive = vi.fn().mockResolvedValue(archive);
    const publishAppPackage = vi.fn().mockResolvedValue({
      ...existing,
      id: "package-published",
      name: archive.name,
      sha256: archive.sha256,
      version: archive.version,
    });
    const deleteAppPackage = vi.fn().mockResolvedValue({
      name: existing.name,
      scope: "catalog_and_object",
      version: existing.version,
    });
    const createLocalPackage = vi.fn().mockResolvedValue({
      locationName: "tool",
      mode: "binary",
      name: archive.name,
      version: archive.version,
    });
    const chooseLocalPackageSource = vi.fn().mockResolvedValue({
      locationName: "tool",
      mode: "binary",
      name: archive.name,
      version: archive.version,
    });
    const buildLocalPackage = vi.fn().mockResolvedValue({
      ...archive,
      source: "built",
    });
    const loadActivityPage = vi.fn().mockResolvedValue({
      events: [],
      nextCursor: "",
      section: ready,
    });

    render(
      <App
        buildLocalPackage={buildLocalPackage}
        chooseAppPackageArchive={chooseAppPackageArchive}
        chooseLocalPackageSource={chooseLocalPackageSource}
        createLocalPackage={createLocalPackage}
        deleteAppPackage={deleteAppPackage}
        listAppPackages={listAppPackages}
        loadActivityPage={loadActivityPage}
        loadAppPackage={loadAppPackage}
        publishAppPackage={publishAppPackage}
        workspace={workspace}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Application packages" }),
    );
    const page = await screen.findByRole("region", {
      name: "Application package management",
    });
    expect(within(page).getByText("internal/existing")).toBeVisible();

    await user.click(
      within(page).getByRole("button", { name: "Inspect internal/existing 9.0.0" }),
    );
    expect(loadAppPackage).toHaveBeenCalledWith("internal/existing", "9.0.0");
    expect(await within(page).findByText(existing.objectKey)).toBeVisible();

    await user.click(
      within(page).getByRole("button", { name: "Choose package archive" }),
    );
    expect(await within(page).findByText(archive.sha256)).toBeVisible();
    await user.type(
      within(page).getByRole("textbox", { name: "Publish confirmation" }),
      "internal/tool@1.2.3",
    );
    await user.click(within(page).getByRole("button", { name: "Publish archive" }));
    expect(publishAppPackage).toHaveBeenCalledWith({
      confirmation: "internal/tool@1.2.3",
      name: archive.name,
      sha256: archive.sha256,
      version: archive.version,
    });

    await user.type(
      within(page).getByRole("textbox", { name: "Package directory name" }),
      "tool",
    );
    await user.type(
      within(page).getByRole("textbox", { name: "Local package name" }),
      "internal/tool",
    );
    const version = within(page).getByRole("textbox", {
      name: "Local package version",
    });
    await user.clear(version);
    await user.type(version, "1.2.3");
    await user.click(
      within(page).getByRole("button", { name: "Choose parent and create" }),
    );
    expect(createLocalPackage).toHaveBeenCalledWith({
      directoryName: "tool",
      mode: "binary",
      name: "internal/tool",
      version: "1.2.3",
    });

    await user.click(
      within(page).getByRole("button", { name: "Choose package source" }),
    );
    await user.click(within(page).getByRole("button", { name: "Build archive" }));
    expect(chooseLocalPackageSource).toHaveBeenCalledOnce();
    expect(buildLocalPackage).toHaveBeenCalledOnce();

    await user.click(
      within(page).getByRole("button", { name: "Delete internal/existing 9.0.0" }),
    );
    const dialog = screen.getByRole("dialog", { name: "Delete application package" });
    await user.click(
      within(dialog).getByRole("checkbox", { name: "Also delete stored object" }),
    );
    await user.type(
      within(dialog).getByRole("textbox", { name: "Delete confirmation" }),
      "internal/existing@9.0.0 DELETE OBJECT",
    );
    await user.click(within(dialog).getByRole("button", { name: "Delete package and object" }));
    expect(deleteAppPackage).toHaveBeenCalledWith({
      confirmation: "internal/existing@9.0.0 DELETE OBJECT",
      deleteObject: true,
      name: existing.name,
      version: existing.version,
    });

    expect(listAppPackages).toHaveBeenCalledTimes(3);
    expect(loadActivityPage).toHaveBeenCalledTimes(2);
  });
});
