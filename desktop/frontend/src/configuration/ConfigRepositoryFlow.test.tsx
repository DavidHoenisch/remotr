// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigRepositoryPage } from "./ConfigRepositoryPage";

afterEach(cleanup);

const workingTree = {
  directoryName: "remotr-config",
  id: "opaque-working-tree",
  status: "selected" as const,
};

describe("Configuration repository flow", () => {
  it("selects one local tree and uses explicit shared-package workflows without Git or server apply", async () => {
    const user = userEvent.setup();
    const validateRepository = vi.fn().mockResolvedValue({
      diagnostics: [],
      issues: [],
      ok: ["fleets/production/manifest.yaml"],
      valid: true,
      workingTreeId: workingTree.id,
    });
    const discoverFleet = vi.fn().mockResolvedValue({
      applications: [],
      capabilityRequirements: ["package:apt"],
      crons: [],
      diagnostics: [],
      fleet: "production",
      manifest: "fleets/production/manifest.yaml",
      modules: ["modules/base.yaml"],
      resourceKinds: ["package"],
      workingTreeId: workingTree.id,
    });
    const renderRepository = vi.fn().mockResolvedValue({
      artifacts: [
        {
          artifactType: "desired",
          content: "schemaVersion: 1\nconfigurations:\n  - name: curl\n",
          digest: "a".repeat(64),
          targetId: "production",
          targetType: "fleet",
        },
      ],
      workingTreeId: workingTree.id,
    });
    const saveRender = vi.fn().mockResolvedValue({ fileName: "production-desired.yaml", status: "saved" });
    const importHubSnippet = vi.fn().mockResolvedValue({
      entryId: "ssh-hardening",
      outPath: "modules/ssh-hardening.yaml",
      status: "imported",
    });
    const initializeRepository = vi.fn().mockResolvedValue({
      fleet: "lab",
      status: "initialized",
      workingTree: { directoryName: "lab-config", id: "opaque-lab", status: "selected" },
    });

    render(
      <ConfigRepositoryPage
        chooseRepository={async () => workingTree}
        discoverFleet={discoverFleet}
        importHubSnippet={importHubSnippet}
        initializeRepository={initializeRepository}
        listHubSnippets={async () => [
          {
            author: "Remotr",
            category: "modules",
            description: "Harden SSH defaults",
            distros: ["Debian"],
            featured: true,
            id: "ssh-hardening",
            tags: ["ssh"],
            title: "SSH hardening",
          },
          {
            author: "Remotr",
            category: "crons",
            description: "Run weekly upgrades",
            distros: ["Debian", "Arch"],
            featured: true,
            id: "weekly-upgrade",
            tags: ["maintenance"],
            title: "Weekly upgrade",
          },
        ]}
        renderRepository={renderRepository}
        saveRender={saveRender}
        validateRepository={validateRepository}
      />,
    );

    const page = screen.getByRole("region", { name: "Configuration repository" });
    expect(within(page).getByText(/never stages, commits, pushes, merges, or applies/i)).toBeVisible();
    await user.click(within(page).getByRole("button", { name: "Choose working tree" }));
    expect(await within(page).findByText("remotr-config")).toBeVisible();

    await user.click(within(page).getByRole("button", { name: "Validate repository" }));
    expect(await screen.findByRole("status", { name: "Repository validation" })).toHaveTextContent("Valid configuration repository");
    expect(validateRepository).toHaveBeenCalledWith(workingTree.id);

    await user.type(within(page).getByRole("textbox", { name: "Fleet name" }), "production");
    await user.click(within(page).getByRole("button", { name: "Discover Fleet files" }));
    const discovery = await screen.findByRole("status", { name: "Fleet discovery" });
    expect(within(discovery).getByText("modules/base.yaml")).toBeVisible();
    expect(discoverFleet).toHaveBeenCalledWith({ fleet: "production", workingTreeId: workingTree.id });

    await user.selectOptions(within(page).getByRole("combobox", { name: "Render scope" }), "fleet");
    await user.type(within(page).getByRole("textbox", { name: "Render target" }), "production");
    await user.click(within(page).getByRole("button", { name: "Render preview" }));
    const preview = await screen.findByRole("region", { name: "Rendered artifacts" });
    expect(within(preview).getByText(/name: curl/)).toBeVisible();
    await user.click(within(preview).getByRole("button", { name: "Save production desired" }));
    expect(saveRender).toHaveBeenCalledWith(expect.objectContaining({ digest: "a".repeat(64), workingTreeId: workingTree.id }));

    await user.click(within(page).getByRole("button", { name: "Import SSH hardening" }));
    const importDialog = screen.getByRole("dialog", { name: "Import Hub snippet" });
    const outPath = within(importDialog).getByRole("textbox", { name: "Repository-relative output path" });
    await user.clear(outPath);
    await user.type(outPath, "modules/ssh-hardening.yaml");
    await user.click(within(importDialog).getByRole("button", { name: "Import snippet" }));
    expect(importHubSnippet).toHaveBeenCalledWith({ entryId: "ssh-hardening", outPath: "modules/ssh-hardening.yaml", workingTreeId: workingTree.id });

    await user.click(within(page).getByRole("button", { name: "Import Weekly upgrade" }));
    expect(within(screen.getByRole("dialog", { name: "Import Hub snippet" })).getByRole("textbox", { name: "Repository-relative output path" })).toHaveValue("crons/weekly-upgrade.yaml");

    await user.click(within(page).getByRole("button", { name: "Initialize repository" }));
    const initDialog = screen.getByRole("dialog", { name: "Initialize Configuration repository" });
    await user.type(within(initDialog).getByRole("textbox", { name: "Initial Fleet" }), "lab");
    await user.selectOptions(within(initDialog).getByRole("combobox", { name: "Remediation policy" }), "report");
    await user.click(within(initDialog).getByRole("button", { name: "Choose empty directory and initialize" }));
    expect(initializeRepository).toHaveBeenCalledWith({ fleet: "lab", remediationPolicy: "report" });
    expect(await within(page).findByText("lab-config")).toBeVisible();
  });
});
