import { AlertTriangle, CheckCircle2, Clock3, LockKeyhole } from "lucide-react";

import type { ChangeRequestSummaryView } from "./ChangeRequestPage";
import type {
  RefreshClock,
  WorkspaceVisibility,
} from "../refresh/useWorkspaceRefresh";
import { GitDesiredStateBoundary } from "../states/GitDesiredStateBoundary";
import { ChangeControlPanel } from "./ChangeControlPanel";
import type {
  ChangeActionResult,
  ChangeControlBindings,
} from "./changeControl";
import "./ChangeRequestDetail.css";

export interface ChangeResourceEvidence {
  activationTargets: string[];
  address: string;
  authorizationGroup: string;
  baselineEligible: boolean;
  dependsOn: string[];
  desiredHash: string;
  predictedEffects: string[];
  provider: string;
  risk: string;
  rollbackClass: string;
}

export interface ChangeTargetEvidence {
  compatible: boolean;
  endpointId: string;
  preflightReady: boolean;
  preflightReason: string;
}

export interface ChangeApprovalEvidence {
  approvedAt: string;
  justification: string;
  operatorId: string;
}

export interface ChangeOutcomeEvidence {
  endpointId: string;
  reason: string;
  state: string;
}

export interface ChangeHistoryEvidence {
  action: string;
  actorId: string;
  details: string;
  occurredAt: string;
}

export interface ChangeRequestDetailView {
  approvals: ChangeApprovalEvidence[];
  approvalsTruncated: boolean;
  artifactDigest: string;
  authorizationGroup: string;
  history: ChangeHistoryEvidence[];
  historyTruncated: boolean;
  outcomes: ChangeOutcomeEvidence[];
  outcomesTruncated: boolean;
  policyWarning: string;
  readOnly: boolean;
  resources: ChangeResourceEvidence[];
  resourcesTruncated: boolean;
  summary: ChangeRequestSummaryView;
  targets: ChangeTargetEvidence[];
  targetsTruncated: boolean;
}

function reportedList(values: string[]): string {
  return values.length > 0 ? values.join(", ") : "None reported";
}

function booleanLabel(value: boolean, positive: string, negative: string): string {
  return value ? positive : negative;
}

function TruncationNotice({ visible }: { visible: boolean }) {
  return visible ? (
    <p className="change-detail-truncation">
      Additional server evidence was omitted from this bounded response.
    </p>
  ) : null;
}

export function ChangeRequestDetail({
  authorizeChangeRequest,
  changeRequestLifecycle,
  chooseBaselineAdoptionPlan,
  clock,
  createBaselineAdoption,
  detail,
  loadChangeRequestDetail,
  onChanged,
  onDetailObserved,
  promoteChangeBaseline,
  refreshActivity,
  visibility,
  watchRandom,
}: Partial<ChangeControlBindings> & {
  clock?: RefreshClock;
  detail: ChangeRequestDetailView;
  loadChangeRequestDetail?: (
    changeRequestId: string,
  ) => Promise<ChangeRequestDetailView>;
  onChanged?: (result: ChangeActionResult) => void;
  onDetailObserved?: (detail: ChangeRequestDetailView) => void;
  refreshActivity?: () => Promise<void>;
  visibility?: WorkspaceVisibility;
  watchRandom?: () => number;
}) {
  const { summary } = detail;
  const changeControlAvailable = Boolean(
    authorizeChangeRequest &&
      changeRequestLifecycle &&
      chooseBaselineAdoptionPlan &&
      createBaselineAdoption &&
      loadChangeRequestDetail &&
      onChanged &&
      promoteChangeBaseline &&
      refreshActivity,
  );

  return (
    <div className="change-detail">
      {!changeControlAvailable ? (
        <div className="change-detail-readonly">
          <LockKeyhole aria-hidden="true" size={15} strokeWidth={1.8} />
          <div>
            <strong>Read-only evidence</strong>
            <span>
              Change-control bindings are unavailable for this connection.
            </span>
          </div>
        </div>
      ) : null}

      <GitDesiredStateBoundary />

      {changeControlAvailable ? (
        <ChangeControlPanel
          authorizeChangeRequest={authorizeChangeRequest!}
          changeRequestLifecycle={changeRequestLifecycle!}
          chooseBaselineAdoptionPlan={chooseBaselineAdoptionPlan!}
          clock={clock}
          createBaselineAdoption={createBaselineAdoption!}
          detail={detail}
          loadChangeRequestDetail={loadChangeRequestDetail!}
          onChanged={onChanged!}
          onDetailObserved={onDetailObserved}
          promoteChangeBaseline={promoteChangeBaseline!}
          refreshActivity={refreshActivity!}
          visibility={visibility}
          watchRandom={watchRandom}
        />
      ) : null}

      <dl className="change-detail-summary">
        <div>
          <dt>Fleet</dt>
          <dd>{summary.fleet}</dd>
        </div>
        <div>
          <dt>Release ref</dt>
          <dd>{summary.releaseRef}</dd>
        </div>
        <div>
          <dt>Risk</dt>
          <dd>{summary.risk}</dd>
        </div>
        <div>
          <dt>Lifecycle</dt>
          <dd>{summary.lifecycle}</dd>
        </div>
        <div>
          <dt>Targets</dt>
          <dd>{summary.targetCount}</dd>
        </div>
        <div>
          <dt>Updated</dt>
          <dd>{summary.updatedAt}</dd>
        </div>
      </dl>

      <section aria-label="Change plan" className="change-detail-section">
        <header>
          <div>
            <span className="change-detail-index">01</span>
            <h3>Change plan</h3>
          </div>
          <span data-mono>{detail.artifactDigest}</span>
        </header>
        <div className="change-detail-card-list">
          {detail.resources.map((resource) => (
            <article className="change-detail-card" key={resource.address}>
              <div className="change-detail-card-title">
                <strong data-mono>{resource.address}</strong>
                <span>{resource.risk}</span>
              </div>
              <dl className="change-detail-grid">
                <div>
                  <dt>Provider</dt>
                  <dd>{resource.provider}</dd>
                </div>
                <div>
                  <dt>Desired hash</dt>
                  <dd>{resource.desiredHash}</dd>
                </div>
                <div>
                  <dt>Predicted effects</dt>
                  <dd>{reportedList(resource.predictedEffects)}</dd>
                </div>
                <div>
                  <dt>Rollback</dt>
                  <dd>{resource.rollbackClass}</dd>
                </div>
                <div>
                  <dt>Activation targets</dt>
                  <dd>{reportedList(resource.activationTargets)}</dd>
                </div>
                <div>
                  <dt>Dependencies</dt>
                  <dd>{reportedList(resource.dependsOn)}</dd>
                </div>
                <div>
                  <dt>Baseline eligible</dt>
                  <dd>{resource.baselineEligible ? "Yes" : "No"}</dd>
                </div>
              </dl>
            </article>
          ))}
        </div>
        <TruncationNotice visible={detail.resourcesTruncated} />
      </section>

      <section
        aria-label="Authorization evidence"
        className="change-detail-section"
      >
        <header>
          <div>
            <span className="change-detail-index">02</span>
            <h3>Authorization evidence</h3>
          </div>
          <span className="change-detail-lifecycle">{summary.lifecycle}</span>
        </header>
        <dl className="change-detail-grid change-detail-grid-wide">
          <div>
            <dt>Authorization group</dt>
            <dd>{detail.authorizationGroup || "Not reported"}</dd>
          </div>
          <div>
            <dt>Approval progress</dt>
            <dd>
              {summary.approvalCount} of {summary.requiredApprovals} approvals
            </dd>
          </div>
        </dl>
        {detail.policyWarning ? (
          <p className="change-detail-warning">
            <AlertTriangle aria-hidden="true" size={15} strokeWidth={1.8} />
            {detail.policyWarning}
          </p>
        ) : null}
        <div className="change-detail-card-list">
          {detail.approvals.map((approval) => (
            <article
              className="change-detail-card change-detail-card-compact"
              key={`${approval.operatorId}-${approval.approvedAt}`}
            >
              <strong data-mono>{approval.operatorId}</strong>
              <span>{approval.approvedAt}</span>
              <p>{approval.justification}</p>
            </article>
          ))}
        </div>
        <TruncationNotice visible={detail.approvalsTruncated} />
      </section>

      <section aria-label="Execution evidence" className="change-detail-section">
        <header>
          <div>
            <span className="change-detail-index">03</span>
            <h3>Execution evidence</h3>
          </div>
          <span>
            {summary.targetCount} planned {summary.targetCount === 1 ? "target" : "targets"}
          </span>
        </header>
        <dl className="change-detail-grid change-detail-grid-wide">
          <div>
            <dt>Rollout window</dt>
            <dd>Not reported</dd>
          </div>
          <div>
            <dt>Progress</dt>
            <dd>
              {detail.outcomes.length} of {summary.targetCount} outcomes reported
            </dd>
          </div>
        </dl>
        <div className="change-detail-card-list">
          {detail.targets.map((target) => (
            <article className="change-detail-card" key={target.endpointId}>
              <div className="change-detail-card-title">
                <strong data-mono>{target.endpointId}</strong>
                <span>{target.preflightReason}</span>
              </div>
              <div className="change-detail-checks">
                <span data-ready={target.compatible}>
                  {target.compatible ? (
                    <CheckCircle2 aria-hidden="true" size={14} strokeWidth={1.8} />
                  ) : (
                    <AlertTriangle aria-hidden="true" size={14} strokeWidth={1.8} />
                  )}
                  {booleanLabel(target.compatible, "Compatible", "Incompatible")}
                </span>
                <span data-ready={target.preflightReady}>
                  {target.preflightReady ? (
                    <CheckCircle2 aria-hidden="true" size={14} strokeWidth={1.8} />
                  ) : (
                    <Clock3 aria-hidden="true" size={14} strokeWidth={1.8} />
                  )}
                  {booleanLabel(
                    target.preflightReady,
                    "Preflight ready",
                    "Preflight not ready",
                  )}
                </span>
              </div>
            </article>
          ))}
        </div>
        <TruncationNotice visible={detail.targetsTruncated} />
      </section>

      <section aria-label="Outcome evidence" className="change-detail-section">
        <header>
          <div>
            <span className="change-detail-index">04</span>
            <h3>Outcome evidence</h3>
          </div>
        </header>
        <div className="change-detail-card-list">
          {detail.outcomes.map((outcome) => (
            <article
              className="change-detail-card change-detail-card-compact"
              key={`${outcome.endpointId}-${outcome.state}`}
            >
              <strong data-mono>{outcome.endpointId}</strong>
              <span className="change-detail-outcome">{outcome.state}</span>
              <p>{outcome.reason}</p>
            </article>
          ))}
        </div>
        <TruncationNotice visible={detail.outcomesTruncated} />
      </section>

      <section aria-label="Change history" className="change-detail-section">
        <header>
          <div>
            <span className="change-detail-index">05</span>
            <h3>Change history</h3>
          </div>
        </header>
        <ol className="change-detail-history">
          {detail.history.map((entry) => (
            <li key={`${entry.occurredAt}-${entry.action}`}>
              <span className="change-detail-history-mark" aria-hidden="true" />
              <div>
                <div>
                  <strong>{entry.action}</strong>
                  <span data-mono>{entry.occurredAt}</span>
                </div>
                <p>{entry.details}</p>
                <span data-mono>{entry.actorId}</span>
              </div>
            </li>
          ))}
        </ol>
        <TruncationNotice visible={detail.historyTruncated} />
      </section>
    </div>
  );
}
