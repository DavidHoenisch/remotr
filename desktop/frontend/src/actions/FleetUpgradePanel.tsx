import {
  CheckCircle2,
  Server,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useState } from "react";

import { validAgentVersion } from "./endpointUpgrade";
import type {
  FleetUpgradeRequest,
  FleetUpgradeResult,
} from "./fleetUpgrade";
import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import "./FleetUpgradePanel.css";

export function FleetUpgradePanel({
  fleet,
  memberCount,
  onClose,
  onPendingChange,
  requestFleetAgentUpgrade,
}: {
  fleet: string;
  memberCount: number;
  onClose: () => void;
  onPendingChange: (pending: boolean) => void;
  requestFleetAgentUpgrade: (
    request: FleetUpgradeRequest,
  ) => Promise<FleetUpgradeResult>;
}) {
  const [version, setVersion] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<ActionErrorEnvelope>();
  const [result, setResult] = useState<FleetUpgradeResult>();

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
      const requested = await requestFleetAgentUpgrade({ fleet, version });
      setResult(requested);
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    } finally {
      setPending(false);
    }
  };

  if (result) {
    return (
      <section
        aria-label="Fleet upgrade requested"
        className="fleet-upgrade-result"
        role="status"
      >
        <CheckCircle2 aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Server acknowledgement</span>
          <strong>Requested</strong>
          <p className="fleet-upgrade-accepted" data-numeric>
            {result.acceptedEndpoints} Endpoints accepted by server
          </p>
          <p>
            Accepted Endpoints receive the desired version on a later Sync.
            This result does not claim installation or convergence.
          </p>
          <dl className="fleet-upgrade-result-context">
            <div>
              <dt>Fleet</dt>
              <dd data-mono>{result.fleet}</dd>
            </div>
            <div>
              <dt>Requested version</dt>
              <dd data-mono>{result.version}</dd>
            </div>
          </dl>
          <button onClick={onClose} type="button">
            Close
          </button>
        </div>
      </section>
    );
  }

  return (
    <section className="fleet-upgrade-panel">
      <div className="fleet-upgrade-intro">
        <Server aria-hidden="true" size={20} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Fleet-scoped request</span>
          <strong data-mono>{fleet}</strong>
          <p>
            Review the current cached membership scope, then explicitly confirm
            the exact version request.
          </p>
        </div>
      </div>

      {error ? (
        <div
          aria-label="Fleet upgrade request failed"
          className="fleet-upgrade-error"
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
          className="fleet-upgrade-form"
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
              aria-describedby="fleet-upgrade-version-guidance"
              autoComplete="off"
              onChange={(event) => setVersion(event.target.value)}
              placeholder="v2.2.0"
              type="text"
              value={version}
            />
          </label>
          <p id="fleet-upgrade-version-guidance">
            Enter an exact semantic version. No “latest” alias is accepted.
          </p>
          <button disabled={!validVersion} type="submit">
            Review Fleet upgrade request
          </button>
        </form>
      ) : (
        <div
          aria-label="Confirm Fleet agent upgrade"
          className="fleet-upgrade-confirmation"
          role="group"
        >
          <TriangleAlert aria-hidden="true" size={19} strokeWidth={1.8} />
          <div>
            <span className="page-kicker">Confirm exact Fleet scope</span>
            <dl>
              <div>
                <dt>Fleet</dt>
                <dd data-mono>{fleet}</dd>
              </div>
              <div>
                <dt>Requested version</dt>
                <dd data-mono>{version}</dd>
              </div>
              <div>
                <dt>Current membership</dt>
                <dd data-numeric>{memberCount} member Endpoints</dd>
              </div>
            </dl>
            <p>
              The server decides the accepted count. The cached member count is
              confirmation context only.
            </p>
            <div className="fleet-upgrade-actions">
              <button
                disabled={pending}
                onClick={() => setReviewing(false)}
                type="button"
              >
                Back
              </button>
              <button
                className="fleet-upgrade-submit"
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
