import {
  ArrowUpCircle,
  CheckCircle2,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useState } from "react";

import type {
  EndpointUpgradeEvidence,
  EndpointUpgradeRequest,
  EndpointUpgradeResult,
} from "./endpointUpgrade";
import { validAgentVersion } from "./endpointUpgrade";
import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import "./EndpointUpgradePanel.css";

export function EndpointUpgradePanel({
  endpoint,
  onClose,
  onPendingChange,
  refreshAffected,
  requestEndpointAgentUpgrade,
}: {
  endpoint: {
    desiredAgentVersion: string;
    endpointId: string;
    reportedAgentVersion: string;
  };
  onClose: () => void;
  onPendingChange: (pending: boolean) => void;
  refreshAffected: (
    result: EndpointUpgradeResult,
  ) => Promise<EndpointUpgradeEvidence>;
  requestEndpointAgentUpgrade: (
    request: EndpointUpgradeRequest,
  ) => Promise<EndpointUpgradeResult>;
}) {
  const [version, setVersion] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<ActionErrorEnvelope>();
  const [result, setResult] = useState<EndpointUpgradeResult>();
  const [evidence, setEvidence] = useState<EndpointUpgradeEvidence>();

  useEffect(() => {
    onPendingChange(pending);
    return () => onPendingChange(false);
  }, [onPendingChange, pending]);

  const validVersion = validAgentVersion(version);

  const submit = async () => {
    if (pending || !reviewing || !validVersion) {
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      const requested = await requestEndpointAgentUpgrade({
        endpointId: endpoint.endpointId,
        version,
      });
      const refreshed = await refreshAffected(requested);
      setEvidence(refreshed);
      setResult(requested);
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    } finally {
      setPending(false);
    }
  };

  if (result && evidence) {
    return (
      <section
        aria-label="Endpoint upgrade requested"
        className="endpoint-upgrade-result"
        role="status"
      >
        <CheckCircle2 aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Server acknowledgement</span>
          <strong>Endpoint upgrade requested</strong>
          <p>
            The request applies on a later Sync. Refreshed desired and reported
            versions remain separate evidence and do not prove completion.
          </p>
          <div
            aria-label="Upgrade state evidence"
            className="endpoint-upgrade-state"
            role="group"
          >
            <div aria-label="Request state" role="group">
              <span>Request state</span>
              <strong>Requested</strong>
              <code>{result.version}</code>
            </div>
            <div aria-label="Desired version evidence" role="group">
              <span>Desired version</span>
              <strong data-mono>
                {evidence.desiredAgentVersion || "Not reported"}
              </strong>
            </div>
            <div aria-label="Reported version evidence" role="group">
              <span>Reported version</span>
              <strong data-mono>
                {evidence.reportedAgentVersion || "Not reported"}
              </strong>
            </div>
            <div aria-label="Completion state" role="group">
              <span>Completion state</span>
              <strong>Not completed</strong>
            </div>
          </div>
          <button onClick={onClose} type="button">
            Close
          </button>
        </div>
      </section>
    );
  }

  return (
    <section className="endpoint-upgrade-panel">
      <div className="endpoint-upgrade-intro">
        <ArrowUpCircle aria-hidden="true" size={20} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">In-band upgrade request</span>
          <strong data-mono>{endpoint.endpointId}</strong>
          <p>
            The server records an exact desired version. The Endpoint receives
            it on a later Sync and reports installation evidence separately.
          </p>
        </div>
      </div>

      <dl className="endpoint-upgrade-current">
        <div>
          <dt>Current desired version</dt>
          <dd data-mono>{endpoint.desiredAgentVersion || "Not reported"}</dd>
        </div>
        <div>
          <dt>Current reported version</dt>
          <dd data-mono>{endpoint.reportedAgentVersion || "Not reported"}</dd>
        </div>
      </dl>

      {error ? (
        <div
          aria-label="Endpoint upgrade request failed"
          className="endpoint-upgrade-error"
          role="alert"
        >
          <TriangleAlert aria-hidden="true" size={18} strokeWidth={1.8} />
          <div>
            <strong>{error.message}</strong>
            <p>{error.guidance}</p>
          </div>
        </div>
      ) : null}

      {!reviewing ? (
        <form
          className="endpoint-upgrade-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (validVersion) {
              setReviewing(true);
            }
          }}
        >
          <label>
            <span>Requested agent version</span>
            <input
              aria-describedby="endpoint-upgrade-version-guidance"
              autoComplete="off"
              onChange={(event) => setVersion(event.target.value)}
              placeholder="v2.2.0"
              type="text"
              value={version}
            />
          </label>
          <p id="endpoint-upgrade-version-guidance">
            Enter an exact semantic version. No “latest” alias is accepted.
          </p>
          <button disabled={!validVersion} type="submit">
            Review upgrade request
          </button>
        </form>
      ) : (
        <div
          aria-label="Confirm Endpoint agent upgrade"
          className="endpoint-upgrade-confirmation"
          role="group"
        >
          <TriangleAlert aria-hidden="true" size={19} strokeWidth={1.8} />
          <div>
            <span className="page-kicker">Confirm exact request</span>
            <strong>
              Request <code>{version}</code> for{" "}
              <code>{endpoint.endpointId}</code>
            </strong>
            <p>
              This records a request only. It does not mean the version is
              installed or the upgrade is completed.
            </p>
            <div className="endpoint-upgrade-actions">
              <button
                disabled={pending}
                onClick={() => setReviewing(false)}
                type="button"
              >
                Back
              </button>
              <button
                className="endpoint-upgrade-submit"
                disabled={pending}
                onClick={() => void submit()}
                type="button"
              >
                {pending ? "Requesting upgrade" : "Request upgrade"}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
