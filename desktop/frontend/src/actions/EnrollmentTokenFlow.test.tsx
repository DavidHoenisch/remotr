// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App } from "../App";

afterEach(() => {
  cleanup();
  clearBrowserPersistence();
  vi.restoreAllMocks();
});

const tokenCanary = "enroll-token-canary-never-persist";
const loadedAt = "2032-03-04T05:05:07Z";
const ready = { snapshot: { loadedAt }, state: "ready" };
const workspace = {
  activity: [],
  changeRequests: [],
  endpoints: [
    {
      compliance: "compliant",
      desiredAgentVersion: "v2.1.0",
      endpointId: "endpoint-alpha",
      evidenceAt: loadedAt,
      fleet: "production",
      freshness: "recent",
      labels: [],
      releaseRef: "release-41",
      reportedAgentVersion: "v2.1.0",
      usernames: ["alice"],
    },
  ],
  fleets: [
    {
      agentVersions: [{ count: 1, status: "v2.1.0" }],
      compliance: [{ count: 1, status: "compliant" }],
      endpointCount: 1,
      fleet: "production",
      freshness: [{ count: 1, status: "recent" }],
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
const enrollmentResult = {
  expiresAt: "2032-03-05T05:05:07Z",
  fleet: "production",
  token: tokenCanary,
};

function browserStorage(): Partial<Storage>[] {
  return [globalThis.localStorage, globalThis.sessionStorage].filter(
    (storage): storage is Partial<Storage> => Boolean(storage),
  );
}

function clearBrowserPersistence() {
  for (const storage of browserStorage()) {
    storage.clear?.();
  }
}

function expectCanaryAbsentFromBrowserPersistence() {
  for (const storage of browserStorage()) {
    for (let index = 0; index < (storage.length ?? 0); index += 1) {
      const key = storage.key?.(index) ?? "";
      expect(key).not.toContain(tokenCanary);
      expect(storage.getItem?.(key) ?? "").not.toContain(tokenCanary);
    }
    expect(JSON.stringify(storage)).not.toContain(tokenCanary);
  }
}

async function openEnrollmentTokenForm(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    screen.getByRole("button", { name: "Create enrollment token" }),
  );
  return screen.getByRole("dialog", { name: "Create enrollment token" });
}

async function submitValidEnrollmentToken(
  user: ReturnType<typeof userEvent.setup>,
  dialog: HTMLElement,
) {
  await user.selectOptions(within(dialog).getByRole("combobox", { name: "Fleet" }), "production");
  const lifetime = within(dialog).getByRole("spinbutton", {
    name: "Token lifetime (hours)",
  });
  await user.clear(lifetime);
  await user.type(lifetime, "24");
  await user.click(
    within(dialog).getByRole("button", { name: "Create one-time token" }),
  );
}

describe("Enrollment token user flow", () => {
  it("validates Fleet and TTL, displays and copies once, and clears every secret lifecycle path", async () => {
    const user = userEvent.setup();
    const createEnrollmentToken = vi
      .fn()
      .mockResolvedValueOnce(enrollmentResult)
      .mockRejectedValueOnce({
        debugContext: tokenCanary,
        guidance: "Review the selected Fleet and lifetime, then retry.",
        kind: "validation",
        message: "The enrollment token request was rejected.",
        retryable: false,
      })
      .mockResolvedValueOnce(enrollmentResult)
      .mockResolvedValueOnce(enrollmentResult);
    const copyEnrollmentToken = vi.fn().mockResolvedValue(undefined);
    const clearEnrollmentToken = vi.fn().mockResolvedValue(undefined);
    const connection = {
      operatorId: "operator-a",
      profileName: "Production",
      serverLabel: "remotr.example:8443",
    };
    const view = render(
      <App
        clearEnrollmentToken={clearEnrollmentToken}
        connection={connection}
        copyEnrollmentToken={copyEnrollmentToken}
        createEnrollmentToken={createEnrollmentToken}
        fleetScope="All Fleets"
        workspace={workspace}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Endpoints" }));
    let dialog = await openEnrollmentTokenForm(user);
    const submit = within(dialog).getByRole("button", {
      name: "Create one-time token",
    });
    expect(submit).toBeDisabled();

    await user.selectOptions(
      within(dialog).getByRole("combobox", { name: "Fleet" }),
      "production",
    );
    const lifetime = within(dialog).getByRole("spinbutton", {
      name: "Token lifetime (hours)",
    });
    await user.clear(lifetime);
    await user.type(lifetime, "0");
    expect(submit).toBeDisabled();
    expect(createEnrollmentToken).not.toHaveBeenCalled();

    await user.clear(lifetime);
    await user.type(lifetime, "24");
    expect(submit).toBeEnabled();
    await user.click(submit);

    expect(createEnrollmentToken).toHaveBeenCalledWith({
      fleet: "production",
      ttlSeconds: 24 * 60 * 60,
    });
    const result = await within(dialog).findByRole("status", {
      name: "Enrollment token created",
    });
    expect(within(result).getByText(tokenCanary)).toBeVisible();
    expect(within(result).getByText("production")).toBeVisible();
    expect(
      within(result).getByText("2032-03-05T05:05:07Z"),
    ).toBeVisible();
    expect(result).toHaveTextContent(/one-time sensitive material/i);
    expect(result).toHaveTextContent(
      /clipboard contents are outside Remotr's persistence boundary/i,
    );
    expect(copyEnrollmentToken).not.toHaveBeenCalled();
    expectCanaryAbsentFromBrowserPersistence();

    await user.click(
      within(result).getByRole("button", { name: "Copy token" }),
    );
    expect(copyEnrollmentToken).toHaveBeenCalledOnce();
    expect(copyEnrollmentToken).toHaveBeenCalledWith();
    expectCanaryAbsentFromBrowserPersistence();

    const clearsBeforeClose = clearEnrollmentToken.mock.calls.length;
    await user.click(
      within(result).getByRole("button", { name: "Clear token and close" }),
    );
    await waitFor(() =>
      expect(clearEnrollmentToken.mock.calls.length).toBeGreaterThan(
        clearsBeforeClose,
      ),
    );
    expect(screen.queryByText(tokenCanary)).not.toBeInTheDocument();
    expectCanaryAbsentFromBrowserPersistence();

    dialog = await openEnrollmentTokenForm(user);
    await submitValidEnrollmentToken(user, dialog);
    const failure = await within(dialog).findByRole("alert", {
      name: "Enrollment token creation failed",
    });
    expect(within(failure).getByText("production")).toBeVisible();
    expect(screen.queryByText(tokenCanary)).not.toBeInTheDocument();
    await user.click(within(failure).getByRole("button", { name: "Cancel" }));

    dialog = await openEnrollmentTokenForm(user);
    await submitValidEnrollmentToken(user, dialog);
    expect(
      await within(dialog).findByRole("status", {
        name: "Enrollment token created",
      }),
    ).toBeVisible();
    const clearsBeforeProfileSwitch = clearEnrollmentToken.mock.calls.length;
    view.rerender(
      <App
        clearEnrollmentToken={clearEnrollmentToken}
        connection={{ ...connection, profileName: "Staging" }}
        copyEnrollmentToken={copyEnrollmentToken}
        createEnrollmentToken={createEnrollmentToken}
        fleetScope="All Fleets"
        workspace={workspace}
      />,
    );
    await waitFor(() =>
      expect(clearEnrollmentToken.mock.calls.length).toBeGreaterThan(
        clearsBeforeProfileSwitch,
      ),
    );
    expect(screen.queryByText(tokenCanary)).not.toBeInTheDocument();

    dialog = await openEnrollmentTokenForm(user);
    await submitValidEnrollmentToken(user, dialog);
    expect(
      await within(dialog).findByRole("status", {
        name: "Enrollment token created",
      }),
    ).toBeVisible();
    const clearsBeforeExit = clearEnrollmentToken.mock.calls.length;
    view.unmount();
    expect(clearEnrollmentToken.mock.calls.length).toBeGreaterThan(
      clearsBeforeExit,
    );
    expectCanaryAbsentFromBrowserPersistence();
  });
});
