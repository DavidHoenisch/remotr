import {
  CheckCircle2,
  Clock3,
  Stethoscope,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useState } from "react";

import type {
  DiagnosticCapabilities,
  DiagnosticCollectionRequest,
  DiagnosticCollectionResult,
} from "./diagnosticCollection";
import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import "./DiagnosticCollectionPage.css";

interface DiagnosticCollectionFailure extends ActionErrorEnvelope {
  existingRequestId?: string;
}

interface FieldErrors {
  collectors?: string;
  endpoint?: string;
  interval?: string;
}

const absoluteTimestamp =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

function validateRequest(
  request: DiagnosticCollectionRequest,
  maxTimeSpanSeconds: number,
): FieldErrors {
  const errors: FieldErrors = {};
  if (!request.endpointId) {
    errors.endpoint = "Select one Endpoint.";
  }
  if (request.collectors.length === 0) {
    errors.collectors = "Select at least one collector.";
  }
  if (
    !absoluteTimestamp.test(request.since) ||
    !absoluteTimestamp.test(request.until)
  ) {
    errors.interval = "Enter absolute RFC 3339 since and until timestamps.";
    return errors;
  }
  const since = Date.parse(request.since);
  const until = Date.parse(request.until);
  if (!Number.isFinite(since) || !Number.isFinite(until)) {
    errors.interval = "Enter valid absolute since and until timestamps.";
  } else if (until <= since) {
    errors.interval = "Until must be after since.";
  } else if (until - since > maxTimeSpanSeconds * 1000) {
    const days = maxTimeSpanSeconds / (24 * 60 * 60);
    errors.interval = `The collection interval must be ${days} days or less.`;
  }
  return errors;
}

function existingRequestId(error: unknown): string | undefined {
  if (typeof error !== "object" || error === null) {
    return undefined;
  }
  const value = (error as { existingRequestId?: unknown }).existingRequestId;
  return typeof value === "string" && value.trim() ? value : undefined;
}

function lifecycleLabel(status: string): string {
  return status ? `${status[0].toUpperCase()}${status.slice(1)}` : "Unknown";
}

export function DiagnosticCollectionPage({
  capabilities: suppliedCapabilities,
  endpoints,
  loadCapabilities,
  onInspectDiagnosticRequest,
  requestDiagnosticCollection,
}: {
  capabilities?: DiagnosticCapabilities;
  endpoints: Array<{ endpointId: string; fleet: string }>;
  loadCapabilities?: () => Promise<DiagnosticCapabilities>;
  onInspectDiagnosticRequest?: (requestId: string) => void;
  requestDiagnosticCollection: (
    request: DiagnosticCollectionRequest,
  ) => Promise<DiagnosticCollectionResult>;
}) {
  const [loadedCapabilities, setLoadedCapabilities] =
    useState<DiagnosticCapabilities>();
  const [capabilityError, setCapabilityError] = useState(false);
  const capabilities = suppliedCapabilities ?? loadedCapabilities;
  const [endpointId, setEndpointId] = useState("");
  const [collectors, setCollectors] = useState<string[]>([]);
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [reviewing, setReviewing] = useState(false);
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<DiagnosticCollectionFailure>();
  const [result, setResult] = useState<DiagnosticCollectionResult>();

  useEffect(() => {
    if (suppliedCapabilities || !loadCapabilities) {
      return;
    }
    let current = true;
    void loadCapabilities()
      .then((loaded) => {
        if (current) {
          setLoadedCapabilities(loaded);
          setCapabilityError(false);
        }
      })
      .catch(() => {
        if (current) {
          setCapabilityError(true);
        }
      });
    return () => {
      current = false;
    };
  }, [loadCapabilities, suppliedCapabilities]);

  const request = { collectors, endpointId, since, until };
  const review = () => {
    if (!capabilities) {
      return;
    }
    const errors = validateRequest(request, capabilities.maxTimeSpanSeconds);
    setFieldErrors(errors);
    if (Object.keys(errors).length === 0) {
      setFailure(undefined);
      setReviewing(true);
    }
  };
  const submit = async () => {
    if (pending || !reviewing) {
      return;
    }
    setPending(true);
    setFailure(undefined);
    try {
      setResult(await requestDiagnosticCollection(request));
    } catch (error: unknown) {
      setFailure({
        ...normalizeActionError(error),
        ...(existingRequestId(error)
          ? { existingRequestId: existingRequestId(error) }
          : {}),
      });
    } finally {
      setPending(false);
    }
  };

  if (capabilityError) {
    return (
      <section
        aria-label="Diagnostic collection"
        className="diagnostic-collection-page"
      >
        <div className="diagnostic-capability-error" role="alert">
          <TriangleAlert aria-hidden="true" size={18} strokeWidth={1.8} />
          <div>
            <strong>Diagnostic limits could not be loaded.</strong>
            <p>Refresh the current connection before preparing a request.</p>
          </div>
        </div>
      </section>
    );
  }

  if (!capabilities) {
    return (
      <section
        aria-label="Diagnostic collection"
        className="diagnostic-collection-page"
      >
        <div className="diagnostic-loading" role="status">
          Loading server-supported diagnostic collectors and limits…
        </div>
      </section>
    );
  }

  return (
    <section
      aria-label="Diagnostic collection"
      className="diagnostic-collection-page"
    >
      <header className="diagnostic-collection-intro">
        <Stethoscope aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Endpoint evidence request</span>
          <h2>Collect diagnostics</h2>
          <p>
            Select an exact Endpoint, supported collectors, and an absolute UTC
            interval. The server returns a trackable request; collection may
            continue after this acknowledgement.
          </p>
        </div>
      </header>

      {result ? (
        <section
          aria-label="Diagnostic collection requested"
          className="diagnostic-result"
          role="status"
        >
          <CheckCircle2 aria-hidden="true" size={22} strokeWidth={1.8} />
          <div>
            <span className="page-kicker">Server acknowledgement</span>
            <strong>Diagnostic collection requested</strong>
            <p>
              Track this request as the server dispatches collectors and
              prepares the bundle. This acknowledgement proves only that the
              server accepted the request.
            </p>
            <dl>
              <div>
                <dt>Request</dt>
                <dd data-mono>{result.requestId}</dd>
              </div>
              <div>
                <dt>Endpoint</dt>
                <dd data-mono>{result.endpointId}</dd>
              </div>
              <div>
                <dt>Lifecycle</dt>
                <dd>{lifecycleLabel(result.status)}</dd>
              </div>
            </dl>
          </div>
        </section>
      ) : (
        <>
          {failure ? (
            <div
              aria-label={
                failure.kind === "conflict"
                  ? "Active diagnostic request exists"
                  : "Diagnostic collection request failed"
              }
              className="diagnostic-request-error"
              role="alert"
            >
              <TriangleAlert aria-hidden="true" size={18} strokeWidth={1.8} />
              <div>
                <strong>{failure.message}</strong>
                <p>{failure.guidance}</p>
                {failure.existingRequestId && onInspectDiagnosticRequest ? (
                  <button
                    onClick={() =>
                      onInspectDiagnosticRequest(failure.existingRequestId!)
                    }
                    type="button"
                  >
                    Inspect diagnostic request {failure.existingRequestId}
                  </button>
                ) : null}
              </div>
            </div>
          ) : null}

          {!reviewing ? (
            <form
              className="diagnostic-request-form"
              onSubmit={(event) => {
                event.preventDefault();
                review();
              }}
            >
              <label className="diagnostic-endpoint-field">
                <span>Diagnostic Endpoint</span>
                <select
                  onChange={(event) => {
                    setEndpointId(event.target.value);
                    setFieldErrors((current) => ({
                      ...current,
                      endpoint: undefined,
                    }));
                  }}
                  value={endpointId}
                >
                  <option value="">Select an Endpoint</option>
                  {endpoints.map((endpoint) => (
                    <option key={endpoint.endpointId} value={endpoint.endpointId}>
                      {endpoint.endpointId} · {endpoint.fleet}
                    </option>
                  ))}
                </select>
                {fieldErrors.endpoint ? (
                  <span className="diagnostic-field-error" role="alert">
                    {fieldErrors.endpoint}
                  </span>
                ) : null}
              </label>

              <fieldset aria-label="Collectors" className="diagnostic-collectors">
                <legend>Collectors</legend>
                <p>Server-supported v1 sources</p>
                <div className="diagnostic-collector-grid">
                  {capabilities.collectors.map((collector) => (
                    <label key={collector}>
                      <input
                        checked={collectors.includes(collector)}
                        onChange={(event) => {
                          setCollectors((current) =>
                            event.target.checked
                              ? [...current, collector]
                              : current.filter((name) => name !== collector),
                          );
                          setFieldErrors((current) => ({
                            ...current,
                            collectors: undefined,
                          }));
                        }}
                        type="checkbox"
                      />
                      <code>{collector}</code>
                    </label>
                  ))}
                </div>
                {fieldErrors.collectors ? (
                  <span className="diagnostic-field-error" role="alert">
                    {fieldErrors.collectors}
                  </span>
                ) : null}
              </fieldset>

              <fieldset
                aria-label="Collection interval"
                className="diagnostic-interval"
              >
                <legend>Collection interval</legend>
                <div>
                  <label>
                    <span>Since timestamp</span>
                    <input
                      autoComplete="off"
                      onChange={(event) => {
                        setSince(event.target.value);
                        setFieldErrors((current) => ({
                          ...current,
                          interval: undefined,
                        }));
                      }}
                      placeholder="2026-03-03T05:05:07Z"
                      type="text"
                      value={since}
                    />
                  </label>
                  <label>
                    <span>Until timestamp</span>
                    <input
                      autoComplete="off"
                      onChange={(event) => {
                        setUntil(event.target.value);
                        setFieldErrors((current) => ({
                          ...current,
                          interval: undefined,
                        }));
                      }}
                      placeholder="2026-03-04T05:05:07Z"
                      type="text"
                      value={until}
                    />
                  </label>
                </div>
                <p>
                  <Clock3 aria-hidden="true" size={14} strokeWidth={1.8} />
                  Maximum server-supported interval: {capabilities.maxTimeSpanSeconds / (24 * 60 * 60)} days.
                </p>
                {fieldErrors.interval ? (
                  <span className="diagnostic-field-error" role="alert">
                    {fieldErrors.interval}
                  </span>
                ) : null}
              </fieldset>

              <button className="diagnostic-review" type="submit">
                Review diagnostic collection
              </button>
            </form>
          ) : (
            <div
              aria-label="Confirm diagnostic collection"
              className="diagnostic-confirmation"
              role="group"
            >
              <TriangleAlert aria-hidden="true" size={20} strokeWidth={1.8} />
              <div>
                <span className="page-kicker">Confirm exact request</span>
                <dl>
                  <div>
                    <dt>Endpoint</dt>
                    <dd data-mono>{endpointId}</dd>
                  </div>
                  <div>
                    <dt>Since</dt>
                    <dd data-mono>{since}</dd>
                  </div>
                  <div>
                    <dt>Until</dt>
                    <dd data-mono>{until}</dd>
                  </div>
                </dl>
                <div className="diagnostic-preview-collectors">
                  <span>Collectors</span>
                  {collectors.map((collector) => (
                    <code key={collector}>{collector}</code>
                  ))}
                </div>
                <p>
                  The server will create a lifecycle-tracked request. Collection
                  and bundle readiness are reported separately.
                </p>
                <div className="diagnostic-confirmation-actions">
                  <button
                    disabled={pending}
                    onClick={() => setReviewing(false)}
                    type="button"
                  >
                    Back
                  </button>
                  <button
                    className="diagnostic-submit"
                    disabled={pending}
                    onClick={() => void submit()}
                    type="button"
                  >
                    {pending
                      ? "Requesting diagnostic collection"
                      : "Request diagnostic collection"}
                  </button>
                </div>
              </div>
            </div>
          )}
        </>
      )}
    </section>
  );
}
