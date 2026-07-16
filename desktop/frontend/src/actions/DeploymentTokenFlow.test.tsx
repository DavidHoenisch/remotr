// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(cleanup);

const loadedAt = "2026-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const workspace = {
  activity: [],
  changeRequests: [],
  endpoints: [],
  fleets: [
    {
      agentVersions: [],
      compliance: [],
      endpointCount: 0,
      fleet: "production",
      freshness: [],
    },
  ],
  sections: {
    activity: ready,
    changeRequests: ready,
    endpoints: ready,
    fleets: ready,
    state: ready,
  },
};

const nightly = {
  createdAt: "2032-03-01T05:05:07Z",
  expiresAt: "2032-03-05T05:05:07Z",
  fleet: "production",
  id: "deployment-nightly",
  label: "nightly",
  lastUsedAt: "2032-03-03T05:05:07Z",
  revokedAt: "",
  status: "active",
};
const prodLaptops = {
  ...nightly,
  id: "deployment-prod-laptops",
  label: "prod-laptops",
  lastUsedAt: "",
};

describe("deployment-token parity workspace", () => {
  it("lists, inspects, creates, protects, and exactly revokes reusable tokens", async () => {
    const user = userEvent.setup();
    const listDeploymentTokens = vi
      .fn()
      .mockResolvedValueOnce([nightly])
      .mockResolvedValueOnce([nightly, prodLaptops])
      .mockResolvedValueOnce([
        nightly,
        {
          ...prodLaptops,
          revokedAt: "2032-03-04T05:07:07Z",
          status: "revoked",
        },
      ]);
    const loadDeploymentToken = vi.fn().mockResolvedValue(nightly);
    const createDeploymentToken = vi.fn().mockResolvedValue({
      metadata: {
        createdAt: "",
        expiresAt: "2032-03-05T05:05:07Z",
        fleet: "production",
        id: "",
        label: "prod-laptops",
        lastUsedAt: "",
        revokedAt: "",
        status: "active",
      },
      token: "deployment-token-view-once",
    });
    const copyDeploymentToken = vi.fn().mockResolvedValue(undefined);
    const saveDeploymentToken = vi.fn().mockResolvedValue({
      path: "/chosen/prod-laptops.token",
      sizeBytes: 27,
      status: "saved",
    });
    const clearDeploymentToken = vi.fn().mockResolvedValue(undefined);
    const revokeDeploymentToken = vi.fn().mockResolvedValue({
      ...prodLaptops,
      revokedAt: "2032-03-04T05:07:07Z",
      status: "revoked",
    });
    const loadActivityPage = vi.fn().mockResolvedValue({
      events: [],
      nextCursor: "",
      section: ready,
    });

    render(
      <App
        clearDeploymentToken={clearDeploymentToken}
        connection={{
          operatorId: "operator-a",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        copyDeploymentToken={copyDeploymentToken}
        createDeploymentToken={createDeploymentToken}
        listDeploymentTokens={listDeploymentTokens}
        loadActivityPage={loadActivityPage}
        loadDeploymentToken={loadDeploymentToken}
        revokeDeploymentToken={revokeDeploymentToken}
        saveDeploymentToken={saveDeploymentToken}
        workspace={workspace}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Deployment tokens" }),
    );
    const page = screen.getByRole("region", {
      name: "Deployment token management",
    });
    expect(await within(page).findByText("nightly")).toBeVisible();
    expect(within(page).getByText("active")).toBeVisible();

    await user.click(
      within(page).getByRole("button", { name: "Inspect nightly" }),
    );
    expect(await within(page).findByText("deployment-nightly")).toBeVisible();
    expect(within(page).getByText("2032-03-03T05:05:07Z")).toBeVisible();

    await user.click(
      within(page).getByRole("button", {
        name: "Create deployment token",
      }),
    );
    await user.type(
      within(page).getByRole("textbox", { name: "Deployment token label" }),
      "prod-laptops",
    );
    await user.selectOptions(
      within(page).getByRole("combobox", { name: "Deployment token Fleet" }),
      "production",
    );
    await user.clear(
      within(page).getByRole("spinbutton", {
        name: "Deployment token lifetime in days",
      }),
    );
    await user.type(
      within(page).getByRole("spinbutton", {
        name: "Deployment token lifetime in days",
      }),
      "1",
    );
    await user.click(
      within(page).getByRole("button", { name: "Review token creation" }),
    );
    expect(within(page).getByText("prod-laptops", { selector: "strong" })).toBeVisible();
    await user.click(
      within(page).getByRole("button", { name: "Create reusable token" }),
    );
    expect(createDeploymentToken).toHaveBeenCalledWith({
      fleet: "production",
      label: "prod-laptops",
      ttlSeconds: 86400,
    });

    const secret = await within(page).findByRole("status", {
      name: "One-time deployment token",
    });
    expect(secret).toHaveTextContent("deployment-token-view-once");
    expect(
      within(page).getByRole("button", { name: "Create deployment token" }),
    ).toBeDisabled();
    expect(copyDeploymentToken).not.toHaveBeenCalled();
    expect(saveDeploymentToken).not.toHaveBeenCalled();

    await user.click(within(secret).getByRole("button", { name: "Copy token" }));
    expect(copyDeploymentToken).toHaveBeenCalledOnce();
    await user.click(
      within(secret).getByRole("button", { name: "Save protected token file" }),
    );
    expect(saveDeploymentToken).toHaveBeenCalledWith("prod-laptops");
    expect(
      await within(secret).findByText("/chosen/prod-laptops.token"),
    ).toBeVisible();
    await user.click(within(secret).getByRole("button", { name: "Clear token" }));
    expect(clearDeploymentToken).toHaveBeenCalledOnce();
    expect(
      within(page).queryByRole("status", { name: "One-time deployment token" }),
    ).not.toBeInTheDocument();

    expect(await within(page).findByText("prod-laptops")).toBeVisible();
    await user.click(
      within(page).getByRole("button", { name: "Revoke prod-laptops" }),
    );
    await user.type(
      within(page).getByRole("textbox", {
        name: "Confirm deployment token label",
      }),
      "PROD-LAPTOPS",
    );
    expect(
      within(page).getByRole("button", { name: "Revoke deployment token" }),
    ).toBeDisabled();
    expect(revokeDeploymentToken).not.toHaveBeenCalled();
    await user.clear(
      within(page).getByRole("textbox", {
        name: "Confirm deployment token label",
      }),
    );
    await user.type(
      within(page).getByRole("textbox", {
        name: "Confirm deployment token label",
      }),
      "prod-laptops",
    );
    await user.click(
      within(page).getByRole("button", { name: "Revoke deployment token" }),
    );
    expect(revokeDeploymentToken).toHaveBeenCalledWith({
      confirmation: "prod-laptops",
      label: "prod-laptops",
    });
    expect(
      await within(page).findByText("Revoked at 2032-03-04T05:07:07Z"),
    ).toBeVisible();
    expect(listDeploymentTokens).toHaveBeenCalledTimes(3);
    expect(loadActivityPage).toHaveBeenCalledTimes(2);
  });
});
