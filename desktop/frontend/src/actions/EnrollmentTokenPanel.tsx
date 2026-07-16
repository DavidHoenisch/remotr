import {
  Check,
  ClipboardCopy,
  KeyRound,
  ShieldAlert,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";

import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "./enrollmentToken";
import "./EnrollmentTokenPanel.css";

interface EnrollmentFailureContext {
  fleet: string;
  ttlHours: string;
}

export function EnrollmentTokenPanel({
  clearEnrollmentToken,
  copyEnrollmentToken,
  createEnrollmentToken,
  fleets,
  onClose,
  onPendingChange,
  refreshAffected,
}: {
  clearEnrollmentToken: () => Promise<void>;
  copyEnrollmentToken: () => Promise<void>;
  createEnrollmentToken: (
    request: EnrollmentTokenRequest,
  ) => Promise<EnrollmentTokenResult>;
  fleets: string[];
  onClose: () => void;
  onPendingChange: (pending: boolean) => void;
  refreshAffected: () => Promise<void> | void;
}) {
  const [fleet, setFleet] = useState("");
  const [ttlHours, setTTLHours] = useState("168");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<ActionErrorEnvelope>();
  const [failureContext, setFailureContext] =
    useState<EnrollmentFailureContext>();
  const [result, setResult] = useState<EnrollmentTokenResult>();
  const [copied, setCopied] = useState(false);
  const pendingRef = useRef(false);

  useEffect(
    () => () => {
      void clearEnrollmentToken();
    },
    [clearEnrollmentToken],
  );

  useEffect(() => {
    onPendingChange(pending);
    return () => onPendingChange(false);
  }, [onPendingChange, pending]);

  const lifetimeHours = Number(ttlHours);
  const ttlSeconds = lifetimeHours * 60 * 60;
  const validLifetime =
    Number.isFinite(lifetimeHours) &&
    lifetimeHours > 0 &&
    Number.isSafeInteger(ttlSeconds);
  const validFleet = fleets.includes(fleet);

  const clearAndClose = async () => {
    if (pendingRef.current) {
      return;
    }
    setResult(undefined);
    setCopied(false);
    try {
      await clearEnrollmentToken();
    } finally {
      onClose();
    }
  };

  const submit = async () => {
    if (pendingRef.current || !validFleet || !validLifetime) {
      return;
    }

    const context = { fleet, ttlHours };
    pendingRef.current = true;
    setPending(true);
    setError(undefined);
    setFailureContext(context);
    setResult(undefined);
    setCopied(false);
    try {
      const created = await createEnrollmentToken({ fleet, ttlSeconds });
      setResult(created);
      await refreshAffected();
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    } finally {
      pendingRef.current = false;
      setPending(false);
    }
  };

  const copy = async () => {
    if (!result) {
      return;
    }
    setCopied(false);
    try {
      await copyEnrollmentToken();
      setCopied(true);
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    }
  };

  if (result) {
    return (
      <section
        aria-label="Enrollment token created"
        className="enrollment-token-result"
        role="status"
      >
        <div className="enrollment-token-result-heading">
          <ShieldAlert aria-hidden="true" size={22} strokeWidth={1.8} />
          <div>
            <span className="page-kicker">One-time sensitive material</span>
            <strong>Enrollment token created</strong>
            <p>
              This token is shown only in this transient result. Clear it when
              you have transferred it to the intended enrollment workflow.
            </p>
          </div>
        </div>
        <dl className="enrollment-token-metadata">
          <div>
            <dt>Fleet</dt>
            <dd data-mono>{result.fleet}</dd>
          </div>
          <div>
            <dt>Expires</dt>
            <dd data-mono>
              <time dateTime={result.expiresAt}>{result.expiresAt}</time>
            </dd>
          </div>
        </dl>
        <div className="enrollment-token-secret">
          <span>One-time token</span>
          <code>{result.token}</code>
        </div>
        <p className="enrollment-token-warning">
          Clipboard contents are outside Remotr&apos;s persistence boundary.
          Copy only when you are ready to transfer the token.
        </p>
        <div className="enrollment-token-actions">
          <button onClick={() => void copy()} type="button">
            {copied ? (
              <Check aria-hidden="true" size={14} strokeWidth={1.8} />
            ) : (
              <ClipboardCopy aria-hidden="true" size={14} strokeWidth={1.8} />
            )}
            {copied ? "Copied" : "Copy token"}
          </button>
          <button
            className="enrollment-token-clear"
            onClick={() => void clearAndClose()}
            type="button"
          >
            Clear token and close
          </button>
        </div>
      </section>
    );
  }

  if (error) {
    return (
      <section
        aria-label="Enrollment token creation failed"
        className="enrollment-token-failure"
        role="alert"
      >
        <TriangleAlert aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Token not created</span>
          <strong>{error.message}</strong>
          <p>{error.guidance}</p>
          <dl className="enrollment-token-failure-context">
            <div>
              <dt>Fleet</dt>
              <dd data-mono>{failureContext?.fleet || "Not selected"}</dd>
            </div>
            <div>
              <dt>Lifetime</dt>
              <dd data-mono>{failureContext?.ttlHours || "Not entered"} hours</dd>
            </div>
          </dl>
          <div className="enrollment-token-actions">
            <button onClick={() => void clearAndClose()} type="button">
              Cancel
            </button>
          </div>
        </div>
      </section>
    );
  }

  return (
    <form
      className="enrollment-token-form"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="enrollment-token-intro">
        <KeyRound aria-hidden="true" size={20} strokeWidth={1.8} />
        <div>
          <strong>Create a one-time enrollment token</strong>
          <p>
            Scope this token to an existing Fleet and choose a positive
            lifetime. The token will be shown once after server creation.
          </p>
        </div>
      </div>
      <div className="enrollment-token-fields">
        <label>
          <span>Fleet</span>
          <select
            aria-label="Fleet"
            onChange={(event) => setFleet(event.target.value)}
            value={fleet}
          >
            <option value="">Select a Fleet</option>
            {fleets.map((availableFleet) => (
              <option key={availableFleet} value={availableFleet}>
                {availableFleet}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>Token lifetime (hours)</span>
          <input
            aria-describedby="enrollment-token-lifetime-guidance"
            aria-label="Token lifetime (hours)"
            min="0.000278"
            onChange={(event) => setTTLHours(event.target.value)}
            step="any"
            type="number"
            value={ttlHours}
          />
        </label>
      </div>
      <p id="enrollment-token-lifetime-guidance">
        The selected Fleet must exist in the current workspace. Lifetime must
        be greater than zero and resolve to whole seconds.
      </p>
      <footer className="enrollment-token-actions">
        <button onClick={() => void clearAndClose()} type="button">
          Cancel
        </button>
        <button
          className="enrollment-token-submit"
          disabled={pending || !validFleet || !validLifetime}
          type="submit"
        >
          <KeyRound aria-hidden="true" size={14} strokeWidth={1.8} />
          {pending ? "Creating one-time token" : "Create one-time token"}
        </button>
      </footer>
    </form>
  );
}
