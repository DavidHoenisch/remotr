import { Trash2, TriangleAlert } from "lucide-react";
import { useState } from "react";

import type {
  EndpointRemovalRequest,
  EndpointRemovalResult,
} from "./endpointRemoval";
import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import "./EndpointRemovalPanel.css";

export function EndpointRemovalPanel({
  endpointId,
  onRemoved,
  removeEndpoint,
}: {
  endpointId: string;
  onRemoved: (result: EndpointRemovalResult) => Promise<void> | void;
  removeEndpoint: (
    request: EndpointRemovalRequest,
  ) => Promise<EndpointRemovalResult>;
}) {
  const [confirming, setConfirming] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<ActionErrorEnvelope>();

  const submit = async () => {
    if (pending || confirmation !== endpointId) {
      return;
    }
    setPending(true);
    setFailure(undefined);
    try {
      const result = await removeEndpoint({ confirmation, endpointId });
      await onRemoved(result);
    } catch (error: unknown) {
      setConfirmation("");
      setFailure(normalizeActionError(error));
    } finally {
      setPending(false);
    }
  };

  if (!confirming) {
    return (
      <section className="endpoint-removal-entry">
        <div>
          <span className="page-kicker">Enrollment authority</span>
          <strong>Remove this Endpoint</strong>
          <p>
            Removal ends this enrolled identity. Desired state remains in Git
            and does not re-enroll the credential automatically.
          </p>
        </div>
        <button
          aria-label={`Remove Endpoint ${endpointId}`}
          onClick={() => setConfirming(true)}
          type="button"
        >
          <Trash2 aria-hidden="true" size={15} strokeWidth={1.8} />
          Remove Endpoint
        </button>
      </section>
    );
  }

  return (
    <section
      aria-label="Confirm Endpoint removal"
      className="endpoint-removal-confirmation"
      role="group"
    >
      <TriangleAlert aria-hidden="true" size={20} strokeWidth={1.8} />
      <div>
        <span className="page-kicker">Destructive action</span>
        <strong>Remove <code>{endpointId}</code> from enrollment?</strong>
        <p>
          Type the exact case-sensitive Endpoint ID. The backend checks the
          same identity again before sending the DELETE request.
        </p>
        {failure ? (
          <div
            aria-label="Endpoint removal failed"
            className="endpoint-removal-error"
            role="alert"
          >
            <strong>{failure.message}</strong>
            <p>{failure.guidance}</p>
          </div>
        ) : null}
        <label>
          <span>{`Type ${endpointId} to confirm`}</span>
          <input
            aria-label={`Type ${endpointId} to confirm`}
            autoComplete="off"
            onChange={(event) => setConfirmation(event.target.value)}
            spellCheck={false}
            type="text"
            value={confirmation}
          />
        </label>
        <div className="endpoint-removal-actions">
          <button
            disabled={pending}
            onClick={() => {
              setConfirmation("");
              setFailure(undefined);
              setConfirming(false);
            }}
            type="button"
          >
            Cancel
          </button>
          <button
            className="endpoint-removal-submit"
            disabled={pending || confirmation !== endpointId}
            onClick={() => void submit()}
            type="button"
          >
            {pending ? "Removing Endpoint" : "Remove Endpoint"}
          </button>
        </div>
      </div>
    </section>
  );
}
