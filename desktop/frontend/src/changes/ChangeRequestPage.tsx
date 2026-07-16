import { ArrowRight, GitPullRequest } from "lucide-react";

import { DataState } from "../states/DataState";
import { GitDesiredStateBoundary } from "../states/GitDesiredStateBoundary";
import "./ChangeRequestPage.css";

export interface ChangeRequestSummaryView {
  approvalCount: number;
  changeRequestId: string;
  fleet: string;
  lifecycle: string;
  releaseRef: string;
  requiredApprovals: number;
  risk: string;
  targetCount: number;
  updatedAt: string;
}

interface ChangeRequestPageProps {
  initialLifecycleFilters?: string[];
  onInspect?: (changeRequestId: string) => void;
  summaries: ChangeRequestSummaryView[];
}

function targetCountLabel(count: number): string {
  return `${count} ${count === 1 ? "target" : "targets"}`;
}

export function ChangeRequestPage({
  initialLifecycleFilters = [],
  onInspect,
  summaries,
}: ChangeRequestPageProps) {
  const visibleSummaries =
    initialLifecycleFilters.length === 0
      ? summaries
      : summaries.filter((summary) =>
          initialLifecycleFilters.includes(summary.lifecycle),
        );

  return (
    <section
      aria-label="Change request inventory"
      className="change-request-inventory"
    >
      <header className="change-request-heading">
        <div>
          <span className="page-kicker">Controlled rollout evidence</span>
          <h2>Change request inventory</h2>
        </div>
        <strong data-numeric>
          {visibleSummaries.length} change request
          {visibleSummaries.length === 1 ? "" : "s"}
        </strong>
      </header>

      {visibleSummaries.length === 0 ? (
        <div className="change-request-empty">
          <DataState
            kind="empty"
            message="No server-reported Change requests match this view."
            title="No Change requests"
          />
          <GitDesiredStateBoundary />
        </div>
      ) : (
        <div className="change-request-table-scroll">
          <table aria-label="Change requests">
            <thead>
              <tr>
                <th scope="col">Change request</th>
                <th scope="col">Fleet</th>
                <th scope="col">Release ref</th>
                <th scope="col">Risk</th>
                <th scope="col">Lifecycle</th>
                <th scope="col">Targets</th>
                <th scope="col">Approvals</th>
                <th scope="col">Updated</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {visibleSummaries.map((summary) => (
                <tr key={summary.changeRequestId}>
                  <td data-mono>
                    <span className="change-request-id">
                      <GitPullRequest
                        aria-hidden="true"
                        size={15}
                        strokeWidth={1.8}
                      />
                      {summary.changeRequestId}
                    </span>
                  </td>
                  <td data-mono>{summary.fleet}</td>
                  <td data-mono>{summary.releaseRef}</td>
                  <td>
                    <span
                      className="change-request-token"
                      data-risk={summary.risk}
                    >
                      {summary.risk}
                    </span>
                  </td>
                  <td>
                    <span
                      className="change-request-token"
                      data-lifecycle={summary.lifecycle}
                    >
                      {summary.lifecycle}
                    </span>
                  </td>
                  <td data-numeric>{targetCountLabel(summary.targetCount)}</td>
                  <td data-numeric>
                    {summary.approvalCount} / {summary.requiredApprovals}
                  </td>
                  <td data-mono>{summary.updatedAt}</td>
                  <td className="change-request-actions">
                    <button
                      aria-label={`Inspect ${summary.changeRequestId}`}
                      disabled={!onInspect}
                      onClick={() => onInspect?.(summary.changeRequestId)}
                      type="button"
                    >
                      <ArrowRight
                        aria-hidden="true"
                        size={15}
                        strokeWidth={1.8}
                      />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
