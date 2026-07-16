import {
  CheckCircle2,
  Tags,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  type EndpointLabelRemoveRequest,
  type EndpointLabelResult,
  type EndpointLabelSetRequest,
  type EndpointLabelView,
  validEndpointLabelKey,
  validEndpointLabelValue,
} from "./endpointLabel";
import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import "./EndpointLabelPanel.css";

function effectLabel(effect: EndpointLabelResult["effect"]): string {
  return `Label ${effect}`;
}

export function EndpointLabelPanel({
  endpointId,
  labels,
  onClose,
  onPendingChange,
  refreshAffected,
  removeEndpointLabel,
  setEndpointLabel,
}: {
  endpointId: string;
  labels: EndpointLabelView[];
  onClose: () => void;
  onPendingChange: (pending: boolean) => void;
  refreshAffected: (result: EndpointLabelResult) => Promise<void> | void;
  removeEndpointLabel: (
    request: EndpointLabelRemoveRequest,
  ) => Promise<EndpointLabelResult>;
  setEndpointLabel: (
    request: EndpointLabelSetRequest,
  ) => Promise<EndpointLabelResult>;
}) {
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [removingKey, setRemovingKey] = useState<string>();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<ActionErrorEnvelope>();
  const [result, setResult] = useState<EndpointLabelResult>();

  useEffect(() => {
    onPendingChange(pending);
    return () => onPendingChange(false);
  }, [onPendingChange, pending]);

  const validSetRequest =
    validEndpointLabelKey(key) && validEndpointLabelValue(value);

  const runMutation = async (
    mutate: () => Promise<EndpointLabelResult>,
  ) => {
    if (pending) {
      return;
    }
    setPending(true);
    setError(undefined);
    setResult(undefined);
    try {
      const updated = await mutate();
      await refreshAffected(updated);
      setResult(updated);
      setRemovingKey(undefined);
    } catch (failure: unknown) {
      setError(normalizeActionError(failure));
    } finally {
      setPending(false);
    }
  };

  if (result) {
    return (
      <section
        aria-label="Endpoint Label updated"
        className="endpoint-label-result"
        role="status"
      >
        <CheckCircle2 aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Server-confirmed mutation</span>
          <strong>{effectLabel(result.effect)}</strong>
          <dl className="endpoint-label-result-context">
            <div>
              <dt>Endpoint</dt>
              <dd data-mono>{result.endpointId}</dd>
            </div>
            <div>
              <dt>Label key</dt>
              <dd data-mono>{result.key}</dd>
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
    <section className="endpoint-label-editor">
      <div className="endpoint-label-intro">
        <Tags aria-hidden="true" size={20} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Exact Endpoint target</span>
          <strong data-mono>{endpointId}</strong>
          <p>
            Add or replace one Label, or explicitly confirm removal of one
            existing key.
          </p>
        </div>
      </div>

      {error ? (
        <div
          aria-label="Endpoint Label update failed"
          className="endpoint-label-error"
          role="alert"
        >
          <TriangleAlert aria-hidden="true" size={18} strokeWidth={1.8} />
          <div>
            <strong>{error.message}</strong>
            <p>{error.guidance}</p>
          </div>
        </div>
      ) : null}

      <form
        className="endpoint-label-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (!validSetRequest) {
            return;
          }
          void runMutation(() =>
            setEndpointLabel({ endpointId, key, value }),
          );
        }}
      >
        <label>
          <span>Label key</span>
          <input
            aria-describedby="endpoint-label-key-guidance"
            autoComplete="off"
            onChange={(event) => setKey(event.target.value)}
            type="text"
            value={key}
          />
        </label>
        <label>
          <span>Label value</span>
          <input
            aria-describedby="endpoint-label-value-guidance"
            autoComplete="off"
            onChange={(event) => setValue(event.target.value)}
            type="text"
            value={value}
          />
        </label>
        <p id="endpoint-label-key-guidance">
          Keys are 1–64 characters, cannot begin with a dot, and cannot contain
          whitespace or an equals sign.
        </p>
        <p id="endpoint-label-value-guidance">
          Values may be empty and cannot exceed 512 characters.
        </p>
        <button
          className="endpoint-label-submit"
          disabled={pending || !validSetRequest}
          type="submit"
        >
          {pending ? "Updating Label" : "Set Label"}
        </button>
      </form>

      <div className="endpoint-label-list">
        <header>
          <span className="page-kicker">Current Labels</span>
          <strong>{labels.length}</strong>
        </header>
        {labels.length > 0 ? (
          <ul>
            {labels.map((label) => (
              <li key={label.key}>
                <div>
                  <span data-mono>{label.key}</span>
                  <strong data-mono>{label.value || "Empty value"}</strong>
                </div>
                <button
                  aria-label={`Remove ${label.key}`}
                  disabled={pending}
                  onClick={() => setRemovingKey(label.key)}
                  type="button"
                >
                  <Trash2 aria-hidden="true" size={14} strokeWidth={1.8} />
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p>No operator-managed Labels are present.</p>
        )}
      </div>

      {removingKey ? (
        <div
          aria-label={`Remove Label ${removingKey}`}
          className="endpoint-label-removal"
          role="group"
        >
          <TriangleAlert aria-hidden="true" size={18} strokeWidth={1.8} />
          <div>
            <strong>Confirm exact Label removal</strong>
            <p>
              Remove <span data-mono>{removingKey}</span> from{" "}
              <span data-mono>{endpointId}</span>. Other Labels remain intact.
            </p>
            <div>
              <button
                disabled={pending}
                onClick={() => setRemovingKey(undefined)}
                type="button"
              >
                Cancel
              </button>
              <button
                aria-label={`Remove Label ${removingKey}`}
                className="endpoint-label-remove-confirm"
                disabled={pending}
                onClick={() =>
                  void runMutation(() =>
                    removeEndpointLabel({ endpointId, key: removingKey }),
                  )
                }
                type="button"
              >
                {pending ? "Removing Label" : "Remove Label"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}
