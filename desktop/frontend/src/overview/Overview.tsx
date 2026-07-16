import {
  Activity,
  ArrowRight,
  GitPullRequest,
  Monitor,
  Server,
} from "lucide-react";

import "../styles/theme.css";
import "./Overview.css";

interface OverviewError {
  guidance: string;
  kind: string;
  message: string;
}

interface OverviewSectionResult {
  error?: OverviewError;
  snapshot: {
    failedAt?: string;
    loadedAt: string;
    observedAt?: string;
  };
  state: string;
}

interface OverviewEndpoint {
  compliance: string;
  endpointId: string;
  fleet: string;
  freshness: string;
}

interface OverviewStatusCount {
  count: number;
  status: string;
}

interface OverviewFleet {
  agentVersions: OverviewStatusCount[];
  compliance: OverviewStatusCount[];
  endpointCount: number;
  fleet: string;
  freshness: OverviewStatusCount[];
}

interface OverviewChangeRequest {
  changeRequestId: string;
  fleet: string;
  lifecycle: string;
  releaseRef: string;
  risk: string;
  updatedAt: string;
}

interface OverviewActivityEvent {
  action: string;
  actor: string;
  eventId: string;
  occurredAt: string;
  resourceId: string;
  resourceType: string;
  status: string;
}

export interface OverviewWorkspace {
  activity: OverviewActivityEvent[];
  changeRequests: OverviewChangeRequest[];
  endpoints: OverviewEndpoint[];
  fleets: OverviewFleet[];
  sections: {
    activity: OverviewSectionResult;
    changeRequests: OverviewSectionResult;
    endpoints: OverviewSectionResult;
    fleets: OverviewSectionResult;
    state: OverviewSectionResult;
  };
}

export interface OverviewNavigationTarget {
  filters: {
    compliance?: string[];
    fleet?: string[];
    freshness?: string[];
    lifecycle?: string[];
  };
  page: "change-requests" | "endpoints" | "fleets";
}

interface OverviewProps {
  onNavigate: (target: OverviewNavigationTarget) => void;
  workspace: OverviewWorkspace;
}

interface DistributionStatus {
  label: string;
  status: string;
  tone: string;
}

const complianceStatuses: DistributionStatus[] = [
  { status: "compliant", label: "Compliant", tone: "compliant" },
  { status: "drifted", label: "Drifted", tone: "drifted" },
  { status: "unsupported", label: "Unsupported", tone: "neutral" },
  { status: "check_failed", label: "Check failed", tone: "error" },
  { status: "deferred", label: "Deferred", tone: "info" },
  { status: "apply_failed", label: "Apply failed", tone: "error" },
  { status: "not_reported", label: "Not reported", tone: "neutral" },
];

const freshnessStatuses: DistributionStatus[] = [
  { status: "recent", label: "Recent", tone: "compliant" },
  { status: "stale", label: "Stale", tone: "drifted" },
  {
    status: "never_reported",
    label: "Never reported",
    tone: "neutral",
  },
];

const activeChangeLifecycles = ["pending", "authorized"];

function countBy(
  endpoints: OverviewEndpoint[],
  select: (endpoint: OverviewEndpoint) => string,
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const endpoint of endpoints) {
    const value = select(endpoint);
    counts.set(value, (counts.get(value) ?? 0) + 1);
  }
  return counts;
}

function endpointNoun(count: number): string {
  return count === 1 ? "Endpoint" : "Endpoints";
}

function humanize(value: string): string {
  const words = value.replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function sectionHasEvidence(section: OverviewSectionResult): boolean {
  return ["ready", "empty", "partial", "stale"].includes(section.state);
}

function SectionState({
  emptyMessage,
  label,
  section,
}: {
  emptyMessage: string;
  label: string;
  section: OverviewSectionResult;
}) {
  if (section.state === "ready") {
    return null;
  }

  if (section.state === "empty") {
    return <p className="overview-empty">{emptyMessage}</p>;
  }

  if (section.state === "loading") {
    return (
      <div className="overview-section-state" data-state="loading">
        <span className="overview-state-pulse" aria-hidden="true" />
        <span>Loading {label}</span>
      </div>
    );
  }

  const unavailable = section.state === "unavailable";
  const stateLabel = unavailable
    ? `${label} unavailable`
    : section.state === "partial"
      ? `${label} partially available`
      : section.state === "stale"
        ? `${label} may be stale`
        : `${label} could not be loaded`;

  return (
    <div
      className="overview-section-state"
      data-kind={section.error?.kind ?? "unknown"}
      data-state={section.state}
    >
      <strong>{stateLabel}</strong>
      <span>{section.error?.guidance ?? section.error?.message}</span>
    </div>
  );
}

function SummaryButton({
  accessibleName,
  icon,
  label,
  onClick,
  value,
}: {
  accessibleName: string;
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  value: number;
}) {
  return (
    <button
      aria-label={accessibleName}
      className="overview-summary-button"
      onClick={onClick}
      type="button"
    >
      <span className="overview-summary-icon" aria-hidden="true">
        {icon}
      </span>
      <span className="overview-summary-copy">
        <strong data-numeric>{value}</strong>
        <span>{label}</span>
      </span>
      <ArrowRight aria-hidden="true" size={15} strokeWidth={1.8} />
    </button>
  );
}

function Distribution({
  counts,
  endpoints,
  label,
  onSelect,
  statuses,
}: {
  counts: Map<string, number>;
  endpoints: number;
  label: string;
  onSelect: (status: string) => void;
  statuses: DistributionStatus[];
}) {
  const visible = statuses.filter((status) => (counts.get(status.status) ?? 0) > 0);

  return (
    <div className="overview-distribution">
      <div className="distribution-track" aria-hidden="true">
        {visible.map((status) => {
          const count = counts.get(status.status) ?? 0;
          return (
            <span
              data-tone={status.tone}
              key={status.status}
              style={{ flexGrow: count / Math.max(endpoints, 1) }}
            />
          );
        })}
      </div>
      <ul className="distribution-legend" aria-label={label}>
        {visible.map((status) => {
          const count = counts.get(status.status) ?? 0;
          return (
            <li key={status.status}>
              <button
                aria-label={`${count} ${status.label.toLowerCase()} ${endpointNoun(count)}`}
                onClick={() => onSelect(status.status)}
                type="button"
              >
                <span
                  aria-hidden="true"
                  className="status-mark"
                  data-tone={status.tone}
                />
                <span>{status.label}</span>
                <strong data-numeric>{count}</strong>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export function Overview({ onNavigate, workspace }: OverviewProps) {
  const endpointCount = workspace.endpoints.length;
  const complianceCounts = countBy(
    workspace.endpoints,
    (endpoint) => endpoint.compliance,
  );
  const freshnessCounts = countBy(
    workspace.endpoints,
    (endpoint) => endpoint.freshness,
  );
  const activeChanges = workspace.changeRequests.filter((change) =>
    activeChangeLifecycles.includes(change.lifecycle),
  );
  const recentActivity = workspace.activity.slice(0, 6);

  return (
    <div className="overview-page">
      <section className="overview-summary-strip" aria-label="Workspace summary">
        <SummaryButton
          accessibleName={`${endpointCount} total Endpoints`}
          icon={<Monitor size={17} strokeWidth={1.8} />}
          label="Managed Endpoints"
          onClick={() => onNavigate({ page: "endpoints", filters: {} })}
          value={endpointCount}
        />
        <SummaryButton
          accessibleName={`${workspace.fleets.length} Fleets`}
          icon={<Server size={17} strokeWidth={1.8} />}
          label="Fleets"
          onClick={() => onNavigate({ page: "fleets", filters: {} })}
          value={workspace.fleets.length}
        />
        <SummaryButton
          accessibleName={`${activeChanges.length} pending or active Change requests`}
          icon={<GitPullRequest size={17} strokeWidth={1.8} />}
          label="Open changes"
          onClick={() =>
            onNavigate({
              page: "change-requests",
              filters: { lifecycle: activeChangeLifecycles },
            })
          }
          value={activeChanges.length}
        />
        <div className="overview-summary-readout">
          <span className="overview-summary-icon" aria-hidden="true">
            <Activity size={17} strokeWidth={1.8} />
          </span>
          <span className="overview-summary-copy">
            <strong data-numeric>{workspace.activity.length}</strong>
            <span>Recent events</span>
          </span>
        </div>
      </section>

      <div className="overview-grid">
        <section className="overview-panel overview-posture" aria-labelledby="posture-heading">
          <header className="overview-panel-header">
            <div>
              <span className="page-kicker">Endpoint evidence</span>
              <h2 id="posture-heading">Operational posture</h2>
            </div>
            <span className="overview-panel-total" data-numeric>
              {endpointCount} total
            </span>
          </header>

          <SectionState
            emptyMessage="No Endpoints are enrolled in the current scope."
            label="Endpoint inventory"
            section={workspace.sections.endpoints}
          />

          {sectionHasEvidence(workspace.sections.endpoints) && endpointCount > 0 ? (
            <div className="posture-distributions">
              <section aria-labelledby="compliance-heading">
                <div className="distribution-heading">
                  <h3 id="compliance-heading">Compliance</h3>
                  <span>Latest State report</span>
                </div>
                <SectionState
                  emptyMessage="No State reports are available."
                  label="Compliance evidence"
                  section={workspace.sections.state}
                />
                <Distribution
                  counts={complianceCounts}
                  endpoints={endpointCount}
                  label="Compliance distribution"
                  onSelect={(compliance) =>
                    onNavigate({
                      page: "endpoints",
                      filters: { compliance: [compliance] },
                    })
                  }
                  statuses={complianceStatuses}
                />
              </section>

              <section aria-labelledby="freshness-heading">
                <div className="distribution-heading">
                  <h3 id="freshness-heading">Check-in freshness</h3>
                  <span>Independent of compliance</span>
                </div>
                <Distribution
                  counts={freshnessCounts}
                  endpoints={endpointCount}
                  label="Freshness distribution"
                  onSelect={(freshness) =>
                    onNavigate({
                      page: "endpoints",
                      filters: { freshness: [freshness] },
                    })
                  }
                  statuses={freshnessStatuses}
                />
              </section>
            </div>
          ) : null}
        </section>

        <section
          className="overview-panel overview-fleets"
          aria-labelledby="fleets-heading"
        >
          <header className="overview-panel-header">
            <div>
              <span className="page-kicker">Scope</span>
              <h2 id="fleets-heading">Fleets</h2>
            </div>
            <button
              className="panel-link"
              onClick={() => onNavigate({ page: "fleets", filters: {} })}
              type="button"
            >
              View all <ArrowRight aria-hidden="true" size={14} />
            </button>
          </header>
          <SectionState
            emptyMessage="No Fleets exist in the current scope."
            label="Fleets"
            section={workspace.sections.fleets}
          />
          {sectionHasEvidence(workspace.sections.fleets) && workspace.fleets.length > 0 ? (
            <ul className="overview-rows fleet-rows">
              {workspace.fleets.slice(0, 5).map((fleet) => (
                <li key={fleet.fleet}>
                  <button
                    onClick={() =>
                      onNavigate({
                        page: "fleets",
                        filters: { fleet: [fleet.fleet] },
                      })
                    }
                    type="button"
                  >
                    <span>
                      <strong data-mono>{fleet.fleet}</strong>
                      <small>{fleet.agentVersions.length} reported version groups</small>
                    </span>
                    <span className="row-metric">
                      <strong data-numeric>{fleet.endpointCount}</strong>
                      <small>{endpointNoun(fleet.endpointCount)}</small>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          ) : null}
        </section>

        <section
          className="overview-panel overview-changes"
          aria-labelledby="changes-heading"
        >
          <header className="overview-panel-header">
            <div>
              <span className="page-kicker">Desired state</span>
              <h2 id="changes-heading">Change requests</h2>
            </div>
            <button
              className="panel-link"
              onClick={() =>
                onNavigate({ page: "change-requests", filters: {} })
              }
              type="button"
            >
              View all <ArrowRight aria-hidden="true" size={14} />
            </button>
          </header>
          <SectionState
            emptyMessage="No Change requests are present."
            label="Change requests"
            section={workspace.sections.changeRequests}
          />
          {sectionHasEvidence(workspace.sections.changeRequests) &&
          workspace.changeRequests.length > 0 ? (
            <ul className="overview-rows change-rows">
              {workspace.changeRequests.slice(0, 5).map((change) => (
                <li key={change.changeRequestId}>
                  <span>
                    <strong data-mono>{change.changeRequestId}</strong>
                    <small>
                      {change.fleet} · {change.releaseRef}
                    </small>
                  </span>
                  <span className="change-state" data-state={change.lifecycle}>
                    {humanize(change.lifecycle)}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </section>

        <section
          aria-label="Recent activity"
          className="overview-panel overview-activity"
        >
          <header className="overview-panel-header">
            <div>
              <span className="page-kicker">Audit trail</span>
              <h2>Recent activity</h2>
            </div>
          </header>
          <SectionState
            emptyMessage="No recent Activity was returned."
            label="Activity"
            section={workspace.sections.activity}
          />
          {sectionHasEvidence(workspace.sections.activity) && recentActivity.length > 0 ? (
            <ul className="overview-rows activity-rows">
              {recentActivity.map((event) => (
                <li key={event.eventId}>
                  <span className="activity-mark" aria-hidden="true" />
                  <span className="activity-copy">
                    <strong>{humanize(event.action)}</strong>
                    <span>
                      <span data-mono>{event.resourceId}</span>
                      <span className="activity-separator" aria-hidden="true">
                        ·
                      </span>
                      <span>{event.actor}</span>
                    </span>
                  </span>
                  <time dateTime={event.occurredAt}>
                    {formatTimestamp(event.occurredAt)}
                  </time>
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      </div>
    </div>
  );
}
