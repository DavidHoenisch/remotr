// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { SecretPage } from "./SecretPage";

afterEach(cleanup);

const inactive = {
  activatedAt: "",
  activatedBy: "",
  activationGeneration: 0,
  createdAt: "2032-03-04T05:05:07Z",
  createdBy: "operator-secrets",
  endpointCopyStatus: "",
  fingerprint: `sha256:${"a".repeat(64)}`,
  name: "wifi/office",
  resolutionBlocked: false,
  revokedAt: "",
  revokedBy: "",
  rollouts: [],
  scopeId: "production",
  scopeType: "fleet" as const,
  status: "inactive" as const,
  version: "2",
};

const planned = {
  ...inactive,
  activatedAt: "2032-03-04T05:06:07Z",
  activatedBy: "operator-secrets",
  activationGeneration: 3,
  rollouts: [
    {
      changeRequestId: "change-secret-2",
      effectiveHash: `sha256:${"c".repeat(64)}`,
      fleet: "production",
      purpose: "network-credential",
      resourceAddress: "office/wifi",
      risk: "connectivity",
    },
  ],
  status: "activation_planned" as const,
};

describe("encrypted Secret parity", () => {
  it("uploads protected input and manages metadata-only versions with rollout planning", async () => {
    const user = userEvent.setup();
    const listSecretVersions = vi.fn().mockResolvedValue([inactive]);
    const uploadSecretVersion = vi.fn().mockResolvedValue(inactive);
    const activateSecretVersion = vi.fn().mockResolvedValue(planned);
    const revokeSecretVersion = vi.fn().mockResolvedValue({
      ...planned,
      endpointCopyStatus: "rotation-or-removal-required",
      resolutionBlocked: true,
      revokedAt: "2032-03-04T05:07:07Z",
      revokedBy: "operator-secrets",
      status: "revoked",
    });
    const refreshActivity = vi.fn().mockResolvedValue(undefined);

    render(
      <SecretPage
        activateSecretVersion={activateSecretVersion}
        endpoints={[{ endpointId: "endpoint-1", label: "endpoint-1" }]}
        fleets={["production"]}
        listSecretVersions={listSecretVersions}
        refreshActivity={refreshActivity}
        revokeSecretVersion={revokeSecretVersion}
        uploadSecretVersion={uploadSecretVersion}
      />,
    );

    const page = screen.getByRole("region", { name: "Encrypted Secret management" });
    await user.type(within(page).getByRole("textbox", { name: "Secret name" }), "wifi/office");
    await user.click(within(page).getByRole("button", { name: "List versions" }));
    expect(await within(page).findByText("sha256:" + "a".repeat(64))).toBeVisible();

    await user.selectOptions(within(page).getByRole("combobox", { name: "Scope" }), "fleet");
    await user.selectOptions(within(page).getByRole("combobox", { name: "Fleet" }), "production");
    await user.click(within(page).getByRole("button", { name: "Choose protected file and upload" }));
    expect(uploadSecretVersion).toHaveBeenCalledWith({
      name: "wifi/office",
      scopeId: "production",
      scopeType: "fleet",
    });

    await user.click(within(page).getByRole("button", { name: "Activate wifi/office 2" }));
    let dialog = screen.getByRole("dialog", { name: "Activate Secret version" });
    await user.type(within(dialog).getByRole("textbox", { name: "Activation confirmation" }), "wifi/office@2 ACTIVATE");
    await user.click(within(dialog).getByRole("button", { name: "Plan activation" }));
    expect(activateSecretVersion).toHaveBeenCalledWith({
      confirmation: "wifi/office@2 ACTIVATE",
      name: "wifi/office",
      version: "2",
    });
    expect(await within(page).findByText("change-secret-2")).toBeVisible();

    await user.click(within(page).getByRole("button", { name: "Revoke wifi/office 2" }));
    dialog = screen.getByRole("dialog", { name: "Revoke Secret version" });
    await user.type(within(dialog).getByRole("textbox", { name: "Revocation confirmation" }), "wifi/office@2 REVOKE");
    await user.click(within(dialog).getByRole("button", { name: "Revoke version" }));
    expect(revokeSecretVersion).toHaveBeenCalledWith({
      confirmation: "wifi/office@2 REVOKE",
      name: "wifi/office",
      version: "2",
    });

    expect(listSecretVersions).toHaveBeenCalledTimes(4);
    expect(refreshActivity).toHaveBeenCalledTimes(3);
    expect(within(page).queryByRole("button", { name: /copy|download|read|reveal/i })).not.toBeInTheDocument();
    expect(page).not.toHaveTextContent("plaintext");
  });
});
