// @vitest-environment jsdom

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { App } from "../App";

afterEach(cleanup);

const loadedAt = "2026-03-04T05:05:07Z";
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

const forbiddenAuthority =
  /(?:edit|write|scaffold|import|render|validate).*(?:desired state|configuration|repository|artifact)|(?:desired state|configuration|repository|artifact).*(?:edit|write|scaffold|import|render|validate)|authori[sz]e.*change|change.*authori[sz]e|\bsecret\b|publish.*package|upload.*package|\brbac\b|(?:create|delete|edit).*\brole\b|issue.*operator|operator.*credential/i;

function visibleInteractiveNames(): string[] {
  return ["button", "link", "textbox", "combobox"]
    .flatMap((role) => screen.queryAllByRole(role))
    .map(
      (element) =>
        element.getAttribute("aria-label") ?? element.textContent?.trim() ?? "",
    );
}

describe("first-release desktop authority boundary", () => {
  it("exposes only Fleet operations and no deferred authority surface", async () => {
    const user = userEvent.setup();
    render(
      <App
        clearEnrollmentToken={async () => undefined}
        connection={{
          operatorId: "operator-a",
          profileName: "Production",
          serverLabel: "remotr.example:8443",
        }}
        copyEnrollmentToken={async () => undefined}
        createEnrollmentToken={async () => ({
          expiresAt: loadedAt,
          fleet: "production",
          token: "one-time-token",
        })}
        diagnosticCapabilities={{
          collectors: ["system_info"],
          maxTimeSpanSeconds: 604800,
        }}
        removeEndpoint={async () => ({
          affectedEvidence: ["inventory", "activity"],
          credentialStatus: "not_enrolled",
          endpointId: "endpoint-alpha",
          status: "removed",
        })}
        removeEndpointLabel={async () => ({
          effect: "removed",
          endpointId: "endpoint-alpha",
          key: "site",
          labels: [],
          value: "",
        })}
        requestDiagnosticCollection={async () => ({
          collectors: ["system_info"],
          endpointId: "endpoint-alpha",
          requestId: "diagnostic-1",
          since: loadedAt,
          status: "pending",
          until: loadedAt,
        })}
        requestEndpointAgentUpgrade={async () => ({
          affectedEvidence: ["desired_agent_version", "activity"],
          endpointId: "endpoint-alpha",
          status: "requested",
          version: "v2.2.0",
        })}
        requestFleetAgentUpgrade={async () => ({
          acceptedEndpoints: 1,
          fleet: "production",
          status: "requested",
          version: "v2.2.0",
        })}
        requestGitSync={async () => ({
          acceptedAt: loadedAt,
          action: "git_sync",
          affectedEvidence: ["release_ref", "activity"],
          requestId: "request-1",
          summary: "Accepted",
          target: "config-repo",
        })}
        setEndpointLabel={async () => ({
          effect: "added",
          endpointId: "endpoint-alpha",
          key: "site",
          labels: [{ key: "site", value: "berlin" }],
          value: "berlin",
        })}
        workspace={workspace}
      />,
    );

    const navigation = screen.getByRole("navigation", {
      name: "Primary navigation",
    });
    const navigationNames = within(navigation)
      .getAllByRole("button")
      .map((button) => button.textContent?.trim());
    expect(navigationNames).toEqual([
      "Overview",
      "Endpoints",
      "Fleets",
      "Change requests",
      "Diagnostics",
      "Activity",
    ]);

    for (const page of navigationNames) {
      await user.click(within(navigation).getByRole("button", { name: page }));
      expect(visibleInteractiveNames()).not.toEqual(
        expect.arrayContaining([expect.stringMatching(forbiddenAuthority)]),
      );
    }
  });
});
