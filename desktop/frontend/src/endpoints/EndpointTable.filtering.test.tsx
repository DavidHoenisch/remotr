// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { EndpointTable, type EndpointTableRow } from "./EndpointTable";

afterEach(cleanup);

const filteringRows: EndpointTableRow[] = [
  {
    endpointId: "endpoint-delta",
    fleet: "staging",
    usernames: ["dave"],
    compliance: "not_reported",
    freshness: "never_reported",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.1.0",
    releaseRef: "release-43",
    labels: [{ key: "role", value: "database" }],
    evidenceAt: "2032-03-04T05:01:07Z",
  },
  {
    endpointId: "endpoint-beta",
    fleet: "production",
    usernames: ["bob"],
    compliance: "drifted",
    freshness: "stale",
    desiredAgentVersion: "v2.0.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-42",
    labels: [{ key: "environment", value: "prod" }],
    evidenceAt: "2032-03-04T05:02:07Z",
  },
  {
    endpointId: "endpoint-charlie",
    fleet: "staging",
    usernames: ["CAROL"],
    compliance: "compliant",
    freshness: "stale",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-43",
    labels: [{ key: "region", value: "east" }],
    evidenceAt: "2032-03-04T05:03:07Z",
  },
  {
    endpointId: "endpoint-alpha",
    fleet: "production",
    usernames: ["Alice"],
    compliance: "compliant",
    freshness: "recent",
    desiredAgentVersion: "v2.1.0",
    reportedAgentVersion: "v2.0.0",
    releaseRef: "release-42",
    labels: [
      { key: "owner", value: "Platform" },
      { key: "region", value: "WEST" },
    ],
    evidenceAt: "2032-03-04T05:04:07Z",
  },
];

function renderInventory() {
  render(
    <EndpointTable
      endpoints={filteringRows}
      labelColumns={["environment", "region"]}
      onOpenEndpoint={() => {}}
    />,
  );
}

function visibleEndpointIDs(): string[] {
  const rows = within(screen.getByRole("table", { name: "Endpoints" })).getAllByRole(
    "row",
  );
  return rows.slice(1).map((row) => within(row).getAllByRole("cell")[0].textContent ?? "");
}

describe("EndpointTable filtering", () => {
  it("searches every visible field case-insensitively", async () => {
    const user = userEvent.setup();
    renderInventory();
    const search = screen.getByRole("searchbox", { name: "Search Endpoints" });

    const searches: Array<[string, string[]]> = [
      ["ALPHA", ["endpoint-alpha"]],
      ["production", ["endpoint-alpha", "endpoint-beta"]],
      ["alice", ["endpoint-alpha"]],
      ["REGION", ["endpoint-alpha", "endpoint-charlie"]],
      ["database", ["endpoint-delta"]],
    ];
    for (const [query, expected] of searches) {
      await user.clear(search);
      await user.type(search, query);
      expect(visibleEndpointIDs().sort()).toEqual(expected);
    }
  });

  it("intersects explicit filters and clears them atomically", async () => {
    const user = userEvent.setup();
    renderInventory();

    const search = screen.getByRole("searchbox", { name: "Search Endpoints" });
    const fleet = screen.getByRole("combobox", { name: "Fleet filter" });
    const compliance = screen.getByRole("combobox", {
      name: "Compliance filter",
    });
    const freshness = screen.getByRole("combobox", {
      name: "Freshness filter",
    });

    await user.type(search, "PLATFORM");
    await user.selectOptions(fleet, "production");
    await user.selectOptions(compliance, "compliant");
    await user.selectOptions(freshness, "recent");

    expect(visibleEndpointIDs()).toEqual(["endpoint-alpha"]);
    expect(screen.getByText("1 of 4 Endpoints")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Clear all filters" }));
    expect(search).toHaveValue("");
    expect(fleet).toHaveValue("all");
    expect(compliance).toHaveValue("all");
    expect(freshness).toHaveValue("all");
    expect(visibleEndpointIDs()).toHaveLength(4);
    expect(screen.getByText("4 Endpoints")).toBeVisible();
  });

  it("sorts equal Fleet values by stable Endpoint identity", async () => {
    const user = userEvent.setup();
    renderInventory();

    await user.click(screen.getByRole("button", { name: "Sort by Fleet" }));

    expect(visibleEndpointIDs()).toEqual([
      "endpoint-alpha",
      "endpoint-beta",
      "endpoint-charlie",
      "endpoint-delta",
    ]);
  });

  it("focuses search from shortcuts without capturing editor input", async () => {
    const user = userEvent.setup();
    renderInventory();
    const search = screen.getByRole("searchbox", { name: "Search Endpoints" });
    const columnChooser = screen.getByRole("button", { name: "Choose columns" });

    columnChooser.focus();
    await user.keyboard("/");
    expect(search).toHaveFocus();
    expect(search).toHaveValue("");

    await user.keyboard("/");
    expect(search).toHaveValue("/");
    await user.click(screen.getByRole("button", { name: "Clear all filters" }));

    columnChooser.focus();
    await user.keyboard("{Control>}k{/Control}");
    expect(search).toHaveFocus();
    expect(search).toHaveValue("");
  });
});
