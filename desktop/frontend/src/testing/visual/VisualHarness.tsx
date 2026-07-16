import { AlertTriangle, Trash2 } from "lucide-react";
import { useState } from "react";

import { App } from "../../App";
import type { EndpointDetailView } from "../../endpoints/EndpointInvestigation";
import { EndpointTable } from "../../endpoints/EndpointTable";
import type { OverviewWorkspace } from "../../overview/Overview";
import { AppShell } from "../../shell/AppShell";
import "./visualHarness.css";

const loadedAt = "2032-03-04T05:05:07Z";
const readySection = { snapshot: { loadedAt }, state: "ready" };

const endpoints = [
  {
    compliance: "compliant",
    desiredAgentVersion: "v2.1.0",
    endpointId: "endpoint-alpha",
    evidenceAt: "2032-03-04T05:04:07Z",
    fleet: "production",
    freshness: "recent",
    labels: [
      { key: "environment", value: "production" },
      { key: "region", value: "us-west" },
    ],
    releaseRef: "release-42",
    reportedAgentVersion: "v2.1.0",
    usernames: ["alice"],
  },
  {
    compliance: "drifted",
    desiredAgentVersion: "v2.1.0",
    endpointId: "endpoint-beta",
    evidenceAt: "2032-03-04T04:58:07Z",
    fleet: "production",
    freshness: "stale",
    labels: [
      { key: "environment", value: "production" },
      { key: "region", value: "us-east" },
    ],
    releaseRef: "release-42",
    reportedAgentVersion: "v2.0.4",
    usernames: ["bob"],
  },
  {
    compliance: "check_failed",
    desiredAgentVersion: "v2.1.0",
    endpointId: "endpoint-gamma",
    evidenceAt: "2032-03-04T05:02:07Z",
    fleet: "staging",
    freshness: "recent",
    labels: [
      { key: "environment", value: "staging" },
      { key: "region", value: "us-west" },
    ],
    releaseRef: "release-41",
    reportedAgentVersion: "v2.1.0",
    usernames: ["carol"],
  },
  {
    compliance: "not_reported",
    desiredAgentVersion: "v2.1.0",
    endpointId: "endpoint-delta",
    fleet: "staging",
    freshness: "never_reported",
    labels: [
      { key: "environment", value: "staging" },
      { key: "region", value: "eu-central" },
    ],
    releaseRef: "release-41",
    reportedAgentVersion: "",
    usernames: [],
  },
];

const workspace: OverviewWorkspace = {
  activity: [
    {
      action: "git_sync",
      actor: "operator-a",
      details: [{ key: "release_ref", value: "release-42" }],
      eventId: "event-101",
      occurredAt: "2032-03-04T05:03:07Z",
      requestId: "request-101",
      resourceId: "primary",
      resourceType: "server",
      status: "accepted",
    },
    {
      action: "endpoint_sync",
      actor: "endpoint-alpha",
      details: [],
      eventId: "event-100",
      occurredAt: "2032-03-04T05:02:07Z",
      requestId: "request-100",
      resourceId: "endpoint-alpha",
      resourceType: "endpoint",
      status: "completed",
    },
  ],
  changeRequests: [
    {
      approvalCount: 1,
      changeRequestId: "change-204",
      fleet: "production",
      lifecycle: "authorized",
      releaseRef: "release-42",
      requiredApprovals: 2,
      risk: "high",
      targetCount: 2,
      updatedAt: "2032-03-04T04:55:07Z",
    },
    {
      approvalCount: 0,
      changeRequestId: "change-205",
      fleet: "staging",
      lifecycle: "pending",
      releaseRef: "release-43",
      requiredApprovals: 1,
      risk: "normal",
      targetCount: 2,
      updatedAt: "2032-03-04T04:50:07Z",
    },
  ],
  endpoints,
  fleets: [
    {
      agentVersions: [
        { count: 1, status: "v2.1.0" },
        { count: 1, status: "v2.0.4" },
      ],
      compliance: [
        { count: 1, status: "compliant" },
        { count: 1, status: "drifted" },
      ],
      endpointCount: 2,
      fleet: "production",
      freshness: [
        { count: 1, status: "recent" },
        { count: 1, status: "stale" },
      ],
    },
    {
      agentVersions: [
        { count: 1, status: "v2.1.0" },
        { count: 1, status: "not_reported" },
      ],
      compliance: [
        { count: 1, status: "check_failed" },
        { count: 1, status: "not_reported" },
      ],
      endpointCount: 2,
      fleet: "staging",
      freshness: [
        { count: 1, status: "recent" },
        { count: 1, status: "never_reported" },
      ],
    },
  ],
  sections: {
    activity: readySection,
    changeRequests: readySection,
    endpoints: readySection,
    fleets: readySection,
    state: readySection,
  },
};

const partialWorkspace: OverviewWorkspace = {
  ...workspace,
  sections: {
    ...workspace.sections,
    state: {
      error: {
        guidance:
          "The last complete State evidence remains visible while two reports are retried.",
        kind: "connection",
        message: "Two Endpoint State reports could not be refreshed.",
      },
      snapshot: {
        failedAt: "2032-03-04T05:05:07Z",
        loadedAt: "2032-03-04T05:04:07Z",
      },
      state: "partial",
    },
  },
};

const endpointDetail: EndpointDetailView = {
  firewall: [
    {
      action: "accept",
      backend: "nftables",
      enforced: true,
      ports: [443],
      protocol: "tcp",
      ruleName: "allow-remotr",
      sources: ["10.42.0.0/16"],
      timestamp: loadedAt,
      wouldHave: "Allow fleet control traffic",
    },
  ],
  firewallTruncated: false,
  header: endpoints[0],
  schedules: [
    {
      applicable: true,
      lastCompletedAt: "2032-03-04T04:45:07Z",
      lastMessage: "Completed successfully",
      lastScheduledFor: "2032-03-04T04:45:00Z",
      lastStatus: "completed",
      name: "state-report",
      schedule: "*/15 * * * *",
    },
  ],
  schedulesTruncated: false,
  sections: {
    firewall: readySection,
    overview: readySection,
    schedules: readySection,
    state: readySection,
    system: readySection,
  },
  state: {
    digest: "sha256:2ed842f6",
    endpointId: "endpoint-alpha",
    items: [
      {
        address: "base/packages",
        description: "Required system packages",
        desiredSummary: "curl and jq installed",
        name: "Packages",
        observedSummary: "curl 8.7.1, jq 1.7",
        provider: "packages",
        reasonCode: "matches",
        status: "compliant",
        subresults: [],
        subresultsTruncated: false,
      },
    ],
    releaseRef: "release-42",
    reportedAt: loadedAt,
    status: "compliant",
  },
  stateTruncated: false,
  system: {
    cpu: "AMD EPYC 7B13",
    cpuCores: "4",
    digest: "sha256:402c861d",
    hostname: "alpha.example",
    kernel: "6.8.0-31-generic",
    memory: "8 GiB",
    os: "Debian GNU/Linux 13",
    reportedAt: loadedAt,
  },
};

const connection = {
  operatorId: "operator-a",
  profileName: "Production",
  serverLabel: "remotr.example:8443",
};

function DestructiveConfirmationFixture() {
  const [confirmation, setConfirmation] = useState("");
  const target = "endpoint-alpha";

  return (
    <AppShell
      activePage="endpoints"
      connection={connection}
      fleetScope="All Fleets"
      onPageChange={() => undefined}
      overlay={{
        content: (
          <section
            aria-label="Endpoint removal confirmation"
            className="visual-destructive-confirmation"
          >
            <div className="visual-danger-notice">
              <AlertTriangle aria-hidden="true" size={20} strokeWidth={1.8} />
              <div>
                <strong>This action cannot be undone.</strong>
                <p>
                  Removing this Endpoint revokes its enrollment. It must be
                  enrolled again before it can report evidence.
                </p>
              </div>
            </div>
            <dl>
              <div>
                <dt>Endpoint</dt>
                <dd data-mono>{target}</dd>
              </div>
              <div>
                <dt>Fleet</dt>
                <dd data-mono>production</dd>
              </div>
              <div>
                <dt>Last evidence</dt>
                <dd data-mono>2032-03-04 05:04 UTC</dd>
              </div>
            </dl>
            <label htmlFor="visual-removal-confirmation">
              Type <code>{target}</code> to confirm
            </label>
            <input
              aria-label={`Type ${target} to confirm`}
              autoComplete="off"
              id="visual-removal-confirmation"
              onChange={(event) => setConfirmation(event.target.value)}
              spellCheck="false"
              value={confirmation}
            />
            <footer>
              <button className="visual-cancel-action" type="button">
                Cancel
              </button>
              <button
                className="visual-destructive-action"
                disabled={confirmation !== target}
                type="button"
              >
                <Trash2 aria-hidden="true" size={15} strokeWidth={1.8} />
                Remove Endpoint
              </button>
            </footer>
          </section>
        ),
        onClose: () => undefined,
        title: `Remove Endpoint ${target}`,
      }}
      renderPage={() => (
        <EndpointTable
          endpoints={endpoints}
          labelColumns={["environment", "region"]}
        />
      )}
    />
  );
}

export function VisualHarness() {
  const fixture = new URLSearchParams(window.location.search).get("state");

  if (fixture === "connection-failure") {
    return (
      <App
        connection={{ ...connection, connected: false }}
        fleetScope="All Fleets"
        onChooseProfile={() => undefined}
        onRetryConnection={() => undefined}
        workspaceFailure={{
          guidance:
            "Verify the server address, network path, and profile credentials before retrying.",
          kind: "connection",
          message: "The selected server could not be reached securely.",
        }}
      />
    );
  }

  if (fixture === "destructive-confirmation") {
    return <DestructiveConfirmationFixture />;
  }

  return (
    <App
      connection={connection}
      fleetScope="All Fleets"
      loadEndpointDetail={async () => endpointDetail}
      workspace={fixture === "partial-overview" ? partialWorkspace : workspace}
    />
  );
}
