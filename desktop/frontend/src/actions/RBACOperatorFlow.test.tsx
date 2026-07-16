// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RBACOperatorPage } from "./RBACOperatorPage";

afterEach(cleanup);

const rule = {
  id: "11111111-1111-4111-8111-111111111111",
  method: "GET",
  pathPattern: "/v1/admin/endpoints/*",
  roleName: "ops_team",
};

const roles = [
  { builtIn: true, description: "Full access", name: "global_admin", rules: [] },
  { builtIn: false, description: "Operations", name: "ops_team", rules: [rule] },
  { builtIn: true, description: "Read access", name: "read_only", rules: [] },
  { builtIn: true, description: "Audit access", name: "security_logger", rules: [] },
];

const operator = {
  certFingerprint: "b".repeat(64),
  createdAt: "2032-03-04T05:05:07Z",
  id: "operator-target",
  roles: ["read_only"],
};

describe("RBAC and Operator parity", () => {
  it("manages canonical roles, rules, assignments, and protected credential output", async () => {
    const user = userEvent.setup();
    const listRoles = vi.fn().mockResolvedValue(roles);
    const getRole = vi.fn().mockImplementation(async (name: string) => roles.find((role) => role.name === name));
    const createRole = vi.fn().mockResolvedValue({ builtIn: false, description: "Responders", name: "responders", rules: [] });
    const deleteRole = vi.fn().mockResolvedValue({ name: "ops_team", ruleId: "", status: "deleted" });
    const addRule = vi.fn().mockResolvedValue({ ...rule, id: "22222222-2222-4222-8222-222222222222", method: "POST" });
    const removeRule = vi.fn().mockResolvedValue({ name: "ops_team", ruleId: rule.id, status: "removed" });
    const listOperators = vi.fn().mockResolvedValue([operator]);
    const setOperatorRoles = vi.fn().mockResolvedValue({ ...operator, roles: ["ops_team", "read_only"] });
    const stampCredential = vi.fn().mockResolvedValue({ directoryName: "siem-credentials", label: "siem-collector", operatorId: "operator-issued", roles: ["security_logger"], status: "saved" });
    const refreshActivity = vi.fn().mockResolvedValue(undefined);

    render(
      <RBACOperatorPage
        addRule={addRule}
        createRole={createRole}
        deleteRole={deleteRole}
        getRole={getRole}
        listOperators={listOperators}
        listRoles={listRoles}
        refreshActivity={refreshActivity}
        removeRule={removeRule}
        setOperatorRoles={setOperatorRoles}
        stampCredential={stampCredential}
      />,
    );

    const page = screen.getByRole("region", { name: "RBAC and Operator administration" });
    expect(await within(page).findByRole("button", { name: "Inspect ops_team" })).toBeVisible();
    expect(within(page).getByText("operator-target")).toBeVisible();

    await user.type(within(page).getByRole("textbox", { name: "New role name" }), "responders");
    await user.type(within(page).getByRole("textbox", { name: "Role description" }), "Responders");
    await user.click(within(page).getByRole("button", { name: "Create role" }));
    expect(createRole).toHaveBeenCalledWith({ description: "Responders", name: "responders" });

    await user.click(within(page).getByRole("button", { name: "Inspect ops_team" }));
    await user.selectOptions(within(page).getByRole("combobox", { name: "Rule method" }), "POST");
    await user.type(within(page).getByRole("textbox", { name: "Rule path pattern" }), "/v1/admin/endpoints/*/diagnostics/collect");
    await user.click(within(page).getByRole("button", { name: "Add rule" }));
    expect(addRule).toHaveBeenCalledWith({ method: "POST", pathPattern: "/v1/admin/endpoints/*/diagnostics/collect", roleName: "ops_team" });

    await user.click(within(page).getByRole("button", { name: `Remove rule ${rule.id}` }));
    let dialog = screen.getByRole("dialog", { name: "Remove RBAC rule" });
    await user.type(within(dialog).getByRole("textbox", { name: "Rule removal confirmation" }), `ops_team/${rule.id} REMOVE RULE`);
    await user.click(within(dialog).getByRole("button", { name: "Remove rule" }));
    expect(removeRule).toHaveBeenCalledWith({ confirmation: `ops_team/${rule.id} REMOVE RULE`, roleName: "ops_team", ruleId: rule.id });

    await user.click(within(page).getByRole("button", { name: "Manage roles for operator-target" }));
    dialog = screen.getByRole("dialog", { name: "Set Operator roles" });
    await user.click(within(dialog).getByRole("checkbox", { name: "ops_team" }));
    await user.type(within(dialog).getByRole("textbox", { name: "Operator role confirmation" }), "operator-target SET ROLES");
    await user.click(within(dialog).getByRole("button", { name: "Set roles" }));
    expect(setOperatorRoles).toHaveBeenCalledWith({ confirmation: "operator-target SET ROLES", operatorId: "operator-target", roles: ["ops_team", "read_only"] });

    await user.click(within(page).getByRole("button", { name: "Delete ops_team" }));
    dialog = screen.getByRole("dialog", { name: "Delete RBAC role" });
    await user.type(within(dialog).getByRole("textbox", { name: "Role deletion confirmation" }), "ops_team DELETE ROLE");
    await user.click(within(dialog).getByRole("button", { name: "Delete role" }));
    expect(deleteRole).toHaveBeenCalledWith({ confirmation: "ops_team DELETE ROLE", name: "ops_team" });

    await user.type(within(page).getByRole("textbox", { name: "Credential label" }), "siem-collector");
    await user.click(within(page).getByRole("checkbox", { name: "Credential role security_logger" }));
    await user.type(within(page).getByRole("textbox", { name: "Credential confirmation" }), "siem-collector ISSUE CREDENTIAL");
    await user.click(within(page).getByRole("button", { name: "Choose directory and issue credential" }));
    expect(stampCredential).toHaveBeenCalledWith({ confirmation: "siem-collector ISSUE CREDENTIAL", label: "siem-collector", roles: ["security_logger"] });
    expect(await within(page).findByText("siem-credentials")).toBeVisible();

    expect(refreshActivity).toHaveBeenCalledTimes(6);
    expect(page).not.toHaveTextContent(/private key|certificate body|BEGIN PRIVATE KEY/i);
  });
});
