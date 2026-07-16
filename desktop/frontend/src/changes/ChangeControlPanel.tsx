import {
  Activity,
  CheckCircle2,
  FileJson2,
  Pause,
  Play,
  ShieldCheck,
  Square,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import type {
  RefreshClock,
  WorkspaceVisibility,
} from "../refresh/useWorkspaceRefresh";
import {
  normalizeActionError,
  type ActionErrorEnvelope,
} from "../actions/useActionController";
import type { ChangeRequestDetailView } from "./ChangeRequestDetail";
import type {
  BaselineAdoptionPreview,
  ChangeActionResult,
  ChangeControlBindings,
  ChangeLifecycleRequest,
} from "./changeControl";
import { useChangeRequestWatch } from "./useChangeRequestWatch";
import "./ChangeControlPanel.css";

const weekdays = [
  { label: "Sun", value: 0 },
  { label: "Mon", value: 1 },
  { label: "Tue", value: 2 },
  { label: "Wed", value: 3 },
  { label: "Thu", value: 4 },
  { label: "Fri", value: 5 },
  { label: "Sat", value: 6 },
];

function dateTimeValue(value: string): string {
  if (!value) return "";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? "" : parsed.toISOString();
}

function startMinute(value: string): number {
  const [hours = "0", minutes = "0"] = value.split(":");
  return Number(hours) * 60 + Number(minutes);
}

function actionLabel(result: ChangeActionResult): string {
  switch (result.action) {
    case "approval_recorded":
      return `Approval recorded · ${result.changeRequest.summary.approvalCount} of ${result.changeRequest.summary.requiredApprovals} required`;
    case "rollout_authorized":
      return "Rollout authorized";
    case "paused":
      return "Rollout paused";
    case "resumed":
      return "Rollout resumed";
    case "revoked":
      return "Authorization revoked";
    case "baseline_promoted":
      return "Baseline promoted";
    case "baseline_adoption_created":
      return "Baseline adoption request created";
    default:
      return "Server accepted the Change-control action";
  }
}

function executionWindowLabel(window: {
  durationMinutes: number;
  startMinuteUtc: number;
  weekdays: number[];
}): string {
  const labels = new Map(weekdays.map((weekday) => [weekday.value, weekday.label]));
  const hours = Math.floor(window.startMinuteUtc / 60).toString().padStart(2, "0");
  const minutes = (window.startMinuteUtc % 60).toString().padStart(2, "0");
  return `${window.weekdays.map((weekday) => labels.get(weekday) ?? String(weekday)).join(", ")} · ${hours}:${minutes} UTC · ${window.durationMinutes} minutes`;
}

export function ChangeControlPanel({
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
}: ChangeControlBindings & {
  clock?: RefreshClock;
  detail: ChangeRequestDetailView;
  loadChangeRequestDetail: (changeRequestId: string) => Promise<ChangeRequestDetailView>;
  onChanged: (result: ChangeActionResult) => void;
  onDetailObserved?: (detail: ChangeRequestDetailView) => void;
  refreshActivity: () => Promise<void>;
  visibility?: WorkspaceVisibility;
  watchRandom?: () => number;
}) {
  const [observed, setObserved] = useState(detail);
  const [pendingAction, setPendingAction] = useState("");
  const [error, setError] = useState<ActionErrorEnvelope>();
  const [result, setResult] = useState<ChangeActionResult>();
  const pending = useRef(false);

  const [justification, setJustification] = useState("");
  const [attemptLimit, setAttemptLimit] = useState(1);
  const [maxConcurrency, setMaxConcurrency] = useState(1);
  const [validFrom, setValidFrom] = useState("");
  const [validUntil, setValidUntil] = useState("");
  const [authorizationConfirmation, setAuthorizationConfirmation] = useState("");
  const [windowEnabled, setWindowEnabled] = useState(false);
  const [windowWeekdays, setWindowWeekdays] = useState([1, 2, 3, 4, 5]);
  const [windowStart, setWindowStart] = useState("02:00");
  const [windowDuration, setWindowDuration] = useState(60);

  const [lifecycleConfirmation, setLifecycleConfirmation] = useState("");
  const eligibleResources = useMemo(
    () => observed.resources.filter((resource) => resource.baselineEligible),
    [observed.resources],
  );
  const [resourceAddress, setResourceAddress] = useState(
    eligibleResources[0]?.address ?? "",
  );
  const [resourceConfirmation, setResourceConfirmation] = useState("");
  const [acknowledgeExceptions, setAcknowledgeExceptions] = useState(false);
  const [adoptionPreview, setAdoptionPreview] = useState<BaselineAdoptionPreview>();
  const [adoptionConfirmation, setAdoptionConfirmation] = useState("");

  const changeRequestId = observed.summary.changeRequestId;
  const unresolvedOutcomes = Math.max(
    0,
    observed.summary.targetCount -
      observed.outcomes.filter((outcome) => outcome.state === "verified_successful").length,
  );

  useEffect(() => {
    setObserved(detail);
  }, [detail]);

  useEffect(() => {
    setError(undefined);
    setResult(undefined);
    setJustification("");
    setAuthorizationConfirmation("");
    setLifecycleConfirmation("");
    setResourceAddress(
      detail.resources.find((resource) => resource.baselineEligible)?.address ?? "",
    );
    setResourceConfirmation("");
    setAcknowledgeExceptions(false);
    setAdoptionPreview(undefined);
    setAdoptionConfirmation("");
  }, [detail.summary.changeRequestId]);

  const watch = useChangeRequestWatch({
    changeRequestId,
    clock,
    loadChangeRequestDetail,
    onUpdate(next) {
      setObserved(next);
      onDetailObserved?.(next);
    },
    random: watchRandom,
    visibility,
  });

  const run = async (
    name: string,
    execute: () => Promise<ChangeActionResult>,
  ) => {
    if (pending.current) return;
    pending.current = true;
    setPendingAction(name);
    setError(undefined);
    setResult(undefined);
    try {
      const accepted = await execute();
      setObserved(accepted.changeRequest);
      setResult(accepted);
      onChanged(accepted);
      await refreshActivity();
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    } finally {
      pending.current = false;
      setPendingAction("");
    }
  };

  const submitLifecycle = (action: ChangeLifecycleRequest["action"]) =>
    run(action, () => changeRequestLifecycle({
      action,
      changeRequestId,
      confirmation: lifecycleConfirmation,
    }));

  const choosePlan = async () => {
    if (pending.current) return;
    setError(undefined);
    try {
      const preview = await chooseBaselineAdoptionPlan(observed.summary.fleet);
      setAdoptionPreview(preview.planId ? preview : undefined);
      setAdoptionConfirmation("");
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    }
  };

  return (
    <section aria-label="Change-control actions" className="change-control-panel">
      <header className="change-control-header">
        <div>
          <span className="page-kicker">Safety-reviewed control plane</span>
          <h3>Change control</h3>
          <p>
            Every mutation is scoped to <strong data-mono>{changeRequestId}</strong> and
            remains subject to server RBAC, approval policy, persistence, and audit.
          </p>
        </div>
        <ShieldCheck aria-hidden="true" size={24} strokeWidth={1.7} />
      </header>

      {error ? (
        <div className="change-control-alert" role="alert">
          <TriangleAlert aria-hidden="true" size={18} strokeWidth={1.8} />
          <div><strong>{error.message}</strong><p>{error.guidance}</p></div>
        </div>
      ) : null}
      {result ? (
        <div className="change-control-result" role="status">
          <CheckCircle2 aria-hidden="true" size={18} strokeWidth={1.8} />
          <div>
            <strong>{actionLabel(result)}</strong>
            <p>Change request and server Activity evidence were refreshed.</p>
          </div>
        </div>
      ) : null}
      {result?.authorization ? (
        <section
          aria-label="Accepted rollout authorization"
          className="change-control-accepted"
        >
          <div>
            <span className="page-kicker">Server-accepted rollout</span>
            <strong data-mono>{result.authorization.id}</strong>
          </div>
          <dl>
            <div>
              <dt>Bounds</dt>
              <dd>{result.authorization.attemptLimit} attempts · {result.authorization.maxConcurrency} concurrent</dd>
            </div>
            <div>
              <dt>Validity</dt>
              <dd data-mono>{result.authorization.validFrom} → {result.authorization.validUntil}</dd>
            </div>
            <div>
              <dt>Authorized by</dt>
              <dd data-mono>{result.authorization.authorizedBy} · {result.authorization.authorizedAt}</dd>
            </div>
            <div>
              <dt>Execution windows</dt>
              <dd>
                {result.authorization.executionWindows.length > 0
                  ? result.authorization.executionWindows.map(executionWindowLabel).join("; ")
                  : "No recurring window restriction"}
              </dd>
            </div>
          </dl>
        </section>
      ) : null}

      <div className="change-control-watch">
        <div>
          <Activity aria-hidden="true" size={18} strokeWidth={1.8} />
          <div>
            <strong>Deterministic watch</strong>
            <span>
              {watch.active
                ? `Watching every 2 seconds${watch.lastObservedAt ? ` · observed ${watch.lastObservedAt}` : ""}`
                : watch.timedOut
                  ? "Watch stopped after its 60-second bound."
                  : "Stopped · no background reads"}
            </span>
          </div>
        </div>
        <button onClick={watch.active ? watch.stop : watch.start} type="button">
          {watch.active ? <Square aria-hidden="true" size={14} /> : <Play aria-hidden="true" size={14} />}
          {watch.active ? "Stop watch" : "Start watch"}
        </button>
      </div>
      {watch.failure ? (
        <p className="change-control-watch-failure" role="status">
          Stale evidence retained after {watch.failure.failedAt}: {watch.failure.message} {watch.failure.guidance}
        </p>
      ) : null}

      <div className="change-control-grid">
        <fieldset aria-label="Authorize rollout" className="change-control-card">
          <legend>01 · Approval &amp; rollout bounds</legend>
          <p>
            Records one Operator approval. A multi-Operator policy stays pending
            until the server receives every distinct approval.
          </p>
          <label>
            <span>Justification</span>
            <textarea
              autoComplete="off"
              maxLength={1024}
              onChange={(event) => setJustification(event.target.value)}
              rows={2}
              value={justification}
            />
          </label>
          <div className="change-control-fields">
            <label>
              <span>Attempt limit</span>
              <input min={1} max={100} onChange={(event) => setAttemptLimit(event.target.valueAsNumber)} type="number" value={attemptLimit} />
            </label>
            <label>
              <span>Maximum concurrency</span>
              <input min={1} max={Math.min(100, observed.summary.targetCount)} onChange={(event) => setMaxConcurrency(event.target.valueAsNumber)} type="number" value={maxConcurrency} />
            </label>
          </div>
          <div className="change-control-fields">
            <label>
              <span>Valid from (optional)</span>
              <input onChange={(event) => setValidFrom(event.target.value)} type="datetime-local" value={validFrom} />
            </label>
            <label>
              <span>Valid until (optional)</span>
              <input onChange={(event) => setValidUntil(event.target.value)} type="datetime-local" value={validUntil} />
            </label>
          </div>
          <label className="change-control-check">
            <input checked={windowEnabled} onChange={(event) => setWindowEnabled(event.target.checked)} type="checkbox" />
            Add one recurring UTC execution window
          </label>
          {windowEnabled ? (
            <div className="change-control-window">
              <div aria-label="Execution weekdays" className="change-control-weekdays" role="group">
                {weekdays.map((weekday) => (
                  <label key={weekday.value}>
                    <input
                      checked={windowWeekdays.includes(weekday.value)}
                      onChange={(event) => setWindowWeekdays((current) => event.target.checked
                        ? [...current, weekday.value].toSorted((left, right) => left - right)
                        : current.filter((value) => value !== weekday.value))}
                      type="checkbox"
                    />
                    {weekday.label}
                  </label>
                ))}
              </div>
              <div className="change-control-fields">
                <label><span>UTC start</span><input onChange={(event) => setWindowStart(event.target.value)} type="time" value={windowStart} /></label>
                <label><span>Duration minutes</span><input min={1} max={1440} onChange={(event) => setWindowDuration(event.target.valueAsNumber)} type="number" value={windowDuration} /></label>
              </div>
            </div>
          ) : null}
          <label>
            <span>Confirm Change request ID</span>
            <input autoComplete="off" onChange={(event) => setAuthorizationConfirmation(event.target.value)} value={authorizationConfirmation} />
          </label>
          <button
            disabled={pending.current || authorizationConfirmation !== changeRequestId || justification.trim().length === 0 || !Number.isInteger(attemptLimit) || attemptLimit < 1 || attemptLimit > 100 || !Number.isInteger(maxConcurrency) || maxConcurrency < 1 || maxConcurrency > Math.min(100, observed.summary.targetCount) || (windowEnabled && (windowWeekdays.length === 0 || !Number.isInteger(windowDuration) || windowDuration < 1 || windowDuration > 1440))}
            onClick={() => void run("authorize", () => authorizeChangeRequest({
              attemptLimit,
              changeRequestId,
              confirmation: authorizationConfirmation,
              executionWindows: windowEnabled ? [{ durationMinutes: windowDuration, startMinuteUtc: startMinute(windowStart), weekdays: windowWeekdays }] : [],
              justification,
              maxConcurrency,
              validFrom: dateTimeValue(validFrom),
              validUntil: dateTimeValue(validUntil),
            }))}
            type="button"
          >
            {pendingAction === "authorize" ? "Recording approval" : "Record approval"}
          </button>
        </fieldset>

        <fieldset aria-label="Lifecycle controls" className="change-control-card">
          <legend>02 · Lifecycle</legend>
          <p>Pause new leases, resume an existing authorization, or revoke it. None of these controls claims Endpoint convergence.</p>
          <dl className="change-control-scope">
            <div><dt>Lifecycle</dt><dd>{observed.summary.lifecycle}</dd></div>
            <div><dt>Frozen targets</dt><dd>{observed.summary.targetCount}</dd></div>
          </dl>
          <label>
            <span>Confirm lifecycle Change request ID</span>
            <input autoComplete="off" onChange={(event) => setLifecycleConfirmation(event.target.value)} value={lifecycleConfirmation} />
          </label>
          <div className="change-control-buttons">
            <button disabled={pending.current || lifecycleConfirmation !== changeRequestId} onClick={() => void submitLifecycle("pause")} type="button"><Pause aria-hidden="true" size={14} />Pause rollout</button>
            <button disabled={pending.current || lifecycleConfirmation !== changeRequestId} onClick={() => void submitLifecycle("resume")} type="button"><Play aria-hidden="true" size={14} />Resume rollout</button>
            <button className="change-control-danger" disabled={pending.current || lifecycleConfirmation !== changeRequestId} onClick={() => void submitLifecycle("revoke")} type="button">Revoke authorization</button>
          </div>
        </fieldset>

        <fieldset aria-label="Promote baseline" className="change-control-card">
          <legend>03 · Baseline promotion</legend>
          <p>Promote only a frozen, eligible resource with verified success evidence.</p>
          <label>
            <span>Eligible resource</span>
            <select onChange={(event) => { setResourceAddress(event.target.value); setResourceConfirmation(""); setAcknowledgeExceptions(false); }} value={resourceAddress}>
              {eligibleResources.map((resource) => <option key={resource.address} value={resource.address}>{resource.address}</option>)}
            </select>
          </label>
          {eligibleResources.find((resource) => resource.address === resourceAddress) ? (
            <dl className="change-control-resource">
              <div><dt>Desired hash</dt><dd data-mono>{eligibleResources.find((resource) => resource.address === resourceAddress)?.desiredHash}</dd></div>
              <div><dt>Provider / risk</dt><dd>{eligibleResources.find((resource) => resource.address === resourceAddress)?.provider} · {eligibleResources.find((resource) => resource.address === resourceAddress)?.risk}</dd></div>
            </dl>
          ) : null}
          <label>
            <span>Confirm resource address</span>
            <input autoComplete="off" onChange={(event) => setResourceConfirmation(event.target.value)} value={resourceConfirmation} />
          </label>
          {unresolvedOutcomes > 0 ? (
            <label className="change-control-check">
              <input checked={acknowledgeExceptions} onChange={(event) => setAcknowledgeExceptions(event.target.checked)} type="checkbox" />
              Acknowledge {unresolvedOutcomes} unresolved target {unresolvedOutcomes === 1 ? "outcome" : "outcomes"}
            </label>
          ) : null}
          <button disabled={pending.current || !resourceAddress || resourceConfirmation !== resourceAddress || (unresolvedOutcomes > 0 && !acknowledgeExceptions)} onClick={() => void run("baseline-promote", () => promoteChangeBaseline({ acknowledgeExceptions, changeRequestId, confirmation: resourceConfirmation, resourceAddress }))} type="button">
            {pendingAction === "baseline-promote" ? "Promoting resource" : "Promote exact resource"}
          </button>
        </fieldset>

        <fieldset aria-label="Adopt existing baseline" className="change-control-card">
          <legend>04 · Baseline adoption</legend>
          <p>A native-selected bounded Fleet plan creates a reviewed Change request. It does not edit, stage, commit, push, or apply desired state.</p>
          <button disabled={pending.current} onClick={() => void choosePlan()} type="button"><FileJson2 aria-hidden="true" size={14} />Choose Fleet plan</button>
          {adoptionPreview ? (
            <div className="change-control-plan">
              <strong>{adoptionPreview.releaseRef}</strong>
              <span data-mono>{adoptionPreview.artifactDigest}</span>
              <span>{adoptionPreview.targetCount} targets · {adoptionPreview.resourceCount} resources</span>
              <span data-mono>{adoptionPreview.resourceAddresses.join(", ")}</span>
            </div>
          ) : null}
          <label>
            <span>Confirm adoption Fleet</span>
            <input autoComplete="off" disabled={!adoptionPreview} onChange={(event) => setAdoptionConfirmation(event.target.value)} value={adoptionConfirmation} />
          </label>
          <button disabled={pending.current || !adoptionPreview || adoptionConfirmation !== observed.summary.fleet} onClick={() => adoptionPreview && void run("baseline-adopt", () => createBaselineAdoption({ confirmation: adoptionConfirmation, fleet: observed.summary.fleet, planId: adoptionPreview.planId }))} type="button">
            {pendingAction === "baseline-adopt" ? "Creating adoption request" : "Create adoption request"}
          </button>
        </fieldset>
      </div>
    </section>
  );
}
