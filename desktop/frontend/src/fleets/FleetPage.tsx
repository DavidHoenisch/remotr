import { ArrowRight, Server } from "lucide-react";
import { useRef, useState } from "react";

import { DataState } from "../states/DataState";
import {
  EndpointTable,
  type EndpointTableInitialFilters,
  type EndpointTableRow,
} from "../endpoints/EndpointTable";
import "./FleetPage.css";

interface StatusCount {
  count: number;
  status: string;
}

export interface FleetSummaryView {
  agentVersions: StatusCount[];
  compliance: StatusCount[];
  endpointCount: number;
  fleet: string;
  freshness: StatusCount[];
}

interface FleetDetailSection {
  error?: {
    guidance: string;
    kind: string;
    message: string;
  };
  snapshot: {
    failedAt?: string;
    loadedAt?: string;
    observedAt?: string;
  };
  state: string;
}

export interface FleetDetailView {
  empty: boolean;
  emptyMessage: string;
  fleet: string;
  members: EndpointTableRow[];
  sections: {
    members: FleetDetailSection;
    state: FleetDetailSection;
  };
  summary: FleetSummaryView;
}

interface FleetPageProps {
  loadFleetDetail?: (fleet: string) => Promise<FleetDetailView>;
  onOpenEndpoint?: (endpointId: string) => void;
  summaries: FleetSummaryView[];
}

interface FleetLoadFailure {
  fleet: string;
  guidance: string;
  message: string;
}

function statusLabel(status: string): string {
  if (status === "not_reported") {
    return "Not reported";
  }
  if (/^v\d/.test(status)) {
    return status;
  }
  const words = status.replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function countLabel(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

function summaryLine(counts: StatusCount[]): string {
  const nonzero = counts.filter((count) => count.count > 0);
  if (nonzero.length === 0) {
    return "No evidence";
  }
  return nonzero
    .map((count) => `${count.count} ${statusLabel(count.status).toLocaleLowerCase()}`)
    .join(" · ");
}

function Distribution({
  counts,
  dimension,
  onSelect,
  title,
}: {
  counts: StatusCount[];
  dimension?: "compliance" | "freshness";
  onSelect?: (filters: EndpointTableInitialFilters) => void;
  title: string;
}) {
  const nonzero = counts.filter((count) => count.count > 0);
  return (
    <section aria-label={title} className="fleet-distribution">
      <header>
        <h3>{title}</h3>
        <span>{countLabel(nonzero.length, "group")}</span>
      </header>
      {nonzero.length > 0 ? (
        <ul>
          {nonzero.map((count) => {
            const label = statusLabel(count.status);
            return (
              <li key={count.status}>
                {dimension && onSelect ? (
                  <button
                    aria-label={`${count.count} ${label.toLocaleLowerCase()} ${
                      count.count === 1 ? "member" : "members"
                    }`}
                    onClick={() => onSelect({ [dimension]: [count.status] })}
                    type="button"
                  >
                    <span>{label}</span>
                    <strong data-numeric>{count.count}</strong>
                  </button>
                ) : (
                  <div>
                    <span>{label}</span>
                    <strong data-numeric>{count.count}</strong>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      ) : (
        <p>No evidence reported.</p>
      )}
    </section>
  );
}

export function FleetPage({
  loadFleetDetail,
  onOpenEndpoint,
  summaries,
}: FleetPageProps) {
  const [detail, setDetail] = useState<FleetDetailView>();
  const [loadingFleet, setLoadingFleet] = useState<string>();
  const [failure, setFailure] = useState<FleetLoadFailure>();
  const [memberFilters, setMemberFilters] =
    useState<EndpointTableInitialFilters>({});
  const loadGeneration = useRef(0);
  const sortedSummaries = summaries.toSorted((left, right) =>
    left.fleet.localeCompare(right.fleet),
  );

  const openFleet = (fleet: string) => {
    if (!loadFleetDetail) {
      return;
    }
    const generation = ++loadGeneration.current;
    setDetail(undefined);
    setFailure(undefined);
    setLoadingFleet(fleet);
    setMemberFilters({});
    void loadFleetDetail(fleet)
      .then((loaded) => {
        if (generation !== loadGeneration.current) {
          return;
        }
        setLoadingFleet(undefined);
        if (loaded.fleet !== fleet || loaded.summary.fleet !== fleet) {
          setFailure({
            fleet,
            guidance: "Select the Fleet again after refreshing inventory.",
            message: "The returned detail did not match the selected Fleet.",
          });
          return;
        }
        setDetail(loaded);
      })
      .catch((error: unknown) => {
        if (generation !== loadGeneration.current) {
          return;
        }
        setLoadingFleet(undefined);
        const classified =
          typeof error === "object" && error !== null
            ? (error as { guidance?: unknown; message?: unknown })
            : undefined;
        setFailure({
          fleet,
          guidance:
            typeof classified?.guidance === "string"
              ? classified.guidance
              : "Check the connection and select the Fleet again.",
          message:
            typeof classified?.message === "string"
              ? classified.message
              : "Fleet detail could not be loaded safely.",
        });
      });
  };

  return (
    <div className="fleet-page">
      <section aria-label="Fleet inventory" className="fleet-inventory">
        <header className="fleet-section-heading">
          <div>
            <span className="page-kicker">Configured scope</span>
            <h2>Fleet inventory</h2>
          </div>
          <strong data-numeric>{countLabel(summaries.length, "Fleet")}</strong>
        </header>

        <div className="fleet-list-scroll">
          <table aria-label="Fleets">
            <thead>
              <tr>
                <th scope="col">Fleet</th>
                <th scope="col">Endpoints</th>
                <th scope="col">Compliance</th>
                <th scope="col">Freshness</th>
                <th scope="col">Reported agents</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {sortedSummaries.map((summary) => (
                <tr key={summary.fleet}>
                  <td data-mono>
                    <span className="fleet-name">
                      <Server aria-hidden="true" size={15} strokeWidth={1.8} />
                      {summary.fleet}
                    </span>
                  </td>
                  <td data-numeric>
                    {countLabel(summary.endpointCount, "Endpoint")}
                  </td>
                  <td>{summaryLine(summary.compliance)}</td>
                  <td>{summaryLine(summary.freshness)}</td>
                  <td>{summaryLine(summary.agentVersions)}</td>
                  <td className="fleet-actions">
                    <button
                      aria-label={`Open Fleet ${summary.fleet}`}
                      disabled={!loadFleetDetail}
                      onClick={() => openFleet(summary.fleet)}
                      type="button"
                    >
                      <ArrowRight aria-hidden="true" size={15} strokeWidth={1.8} />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {loadingFleet ? (
        <section aria-label={`Fleet ${loadingFleet} detail`} className="fleet-detail">
          <DataState
            kind="loading"
            message="Loading member and State evidence."
            title={`Loading Fleet ${loadingFleet}`}
          />
        </section>
      ) : null}

      {failure ? (
        <section aria-label={`Fleet ${failure.fleet} detail`} className="fleet-detail">
          <DataState
            guidance={failure.guidance}
            kind="unexpected"
            message={failure.message}
            title={`Fleet ${failure.fleet} unavailable`}
          />
        </section>
      ) : null}

      {detail ? (
        <section
          aria-label={`Fleet ${detail.fleet} detail`}
          className="fleet-detail"
        >
          <header className="fleet-section-heading">
            <div>
              <span className="page-kicker">Selected Fleet</span>
              <h2 data-mono>{detail.fleet}</h2>
            </div>
            <strong data-numeric>
              {detail.summary.endpointCount} member Endpoints
            </strong>
          </header>

          {detail.empty ? (
            <DataState
              kind="empty"
              message={detail.emptyMessage}
              title={`No Endpoints enrolled in Fleet ${detail.fleet}`}
            />
          ) : (
            <>
              <div className="fleet-distributions">
                <Distribution
                  counts={detail.summary.compliance}
                  dimension="compliance"
                  onSelect={setMemberFilters}
                  title="Compliance"
                />
                <Distribution
                  counts={detail.summary.freshness}
                  dimension="freshness"
                  onSelect={setMemberFilters}
                  title="Freshness"
                />
                <Distribution
                  counts={detail.summary.agentVersions}
                  title="Agent versions"
                />
              </div>
              <div className="fleet-members">
                <EndpointTable
                  endpoints={detail.members}
                  initialFilters={memberFilters}
                  labelColumns={["environment", "region"]}
                  onOpenEndpoint={onOpenEndpoint}
                />
              </div>
            </>
          )}
        </section>
      ) : null}
    </div>
  );
}
