import {
  Clipboard,
  Download,
  Eye,
  KeyRound,
  Plus,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useEffect, useState } from "react";

import {
  type ActionErrorEnvelope,
  normalizeActionError,
} from "./useActionController";
import type {
  DeploymentTokenCreateRequest,
  DeploymentTokenCreateResult,
  DeploymentTokenRevokeRequest,
  DeploymentTokenSaveResult,
  DeploymentTokenView,
} from "./deploymentToken";
import "./DeploymentTokenPage.css";

interface DeploymentTokenPageProps {
  clearDeploymentToken: () => Promise<void>;
  copyDeploymentToken: () => Promise<void>;
  createDeploymentToken: (
    request: DeploymentTokenCreateRequest,
  ) => Promise<DeploymentTokenCreateResult>;
  fleets: string[];
  listDeploymentTokens: () => Promise<DeploymentTokenView[]>;
  loadDeploymentToken: (label: string) => Promise<DeploymentTokenView>;
  refreshActivity: () => Promise<void>;
  revokeDeploymentToken: (
    request: DeploymentTokenRevokeRequest,
  ) => Promise<DeploymentTokenView>;
  saveDeploymentToken: (
    label: string,
  ) => Promise<DeploymentTokenSaveResult>;
}

function FailureNotice({ failure }: { failure?: ActionErrorEnvelope }) {
  if (!failure) return null;
  return (
    <div className="deployment-token-failure" role="alert">
      <strong>{failure.message}</strong>
      <span>{failure.guidance}</span>
    </div>
  );
}

function displayStatus(status: string): string {
  return status.replaceAll("_", " ");
}

export function DeploymentTokenPage({
  clearDeploymentToken,
  copyDeploymentToken,
  createDeploymentToken,
  fleets,
  listDeploymentTokens,
  loadDeploymentToken,
  refreshActivity,
  revokeDeploymentToken,
  saveDeploymentToken,
}: DeploymentTokenPageProps) {
  const [tokens, setTokens] = useState<DeploymentTokenView[]>([]);
  const [listPending, setListPending] = useState(true);
  const [failure, setFailure] = useState<ActionErrorEnvelope>();
  const [detail, setDetail] = useState<DeploymentTokenView>();
  const [createOpen, setCreateOpen] = useState(false);
  const [label, setLabel] = useState("");
  const [fleet, setFleet] = useState(fleets[0] ?? "");
  const [lifetimeDays, setLifetimeDays] = useState("365");
  const [reviewing, setReviewing] = useState(false);
  const [createPending, setCreatePending] = useState(false);
  const [oneTime, setOneTime] = useState<DeploymentTokenCreateResult>();
  const [secretPending, setSecretPending] = useState(false);
  const [saveResult, setSaveResult] = useState<DeploymentTokenSaveResult>();
  const [revokeTarget, setRevokeTarget] = useState<DeploymentTokenView>();
  const [confirmation, setConfirmation] = useState("");
  const [revokePending, setRevokePending] = useState(false);
  const [revokeResult, setRevokeResult] = useState<DeploymentTokenView>();

  const refreshTokens = async () => {
    const loaded = await listDeploymentTokens();
    setTokens(loaded.map((token) => ({ ...token })));
  };

  useEffect(() => {
    let current = true;
    setListPending(true);
    void listDeploymentTokens()
      .then((loaded) => {
        if (current) {
          setTokens(loaded.map((token) => ({ ...token })));
          setFailure(undefined);
        }
      })
      .catch((error: unknown) => {
        if (current) setFailure(normalizeActionError(error));
      })
      .finally(() => {
        if (current) setListPending(false);
      });
    return () => {
      current = false;
    };
  }, [listDeploymentTokens]);

  const inspect = async (token: DeploymentTokenView) => {
    setFailure(undefined);
    try {
      const loaded = await loadDeploymentToken(token.label);
      if (loaded.label !== token.label) {
        throw new Error("The returned token did not match the selected label.");
      }
      setDetail({ ...loaded });
    } catch (error: unknown) {
      setFailure(normalizeActionError(error));
    }
  };

  const reviewCreate = () => {
    const days = Number(lifetimeDays);
    if (
      !label ||
      !/^[A-Za-z0-9_-]+$/.test(label) ||
      !fleet ||
      !Number.isInteger(days) ||
      days <= 0
    ) {
      setFailure(
        normalizeActionError({
          guidance:
            "Enter a safe label, select a Fleet, and use a positive whole-day lifetime.",
          kind: "validation",
          message: "Review the deployment token fields.",
          retryable: false,
        }),
      );
      return;
    }
    setFailure(undefined);
    setReviewing(true);
  };

  const create = async () => {
    if (createPending || !reviewing) return;
    const request = {
      fleet,
      label,
      ttlSeconds: Number(lifetimeDays) * 24 * 60 * 60,
    };
    setCreatePending(true);
    setFailure(undefined);
    try {
      const result = await createDeploymentToken(request);
      if (result.metadata.label !== request.label || !result.token) {
        throw new Error("The server returned incomplete deployment token evidence.");
      }
      setOneTime({ metadata: { ...result.metadata }, token: result.token });
      setSaveResult(undefined);
      setCreateOpen(false);
      setReviewing(false);
      setLabel("");
      await Promise.all([refreshTokens(), refreshActivity()]);
    } catch (error: unknown) {
      setFailure(normalizeActionError(error));
    } finally {
      setCreatePending(false);
    }
  };

  const copy = async () => {
    if (secretPending) return;
    setSecretPending(true);
    setFailure(undefined);
    try {
      await copyDeploymentToken();
    } catch (error: unknown) {
      setFailure(normalizeActionError(error));
    } finally {
      setSecretPending(false);
    }
  };

  const save = async () => {
    if (!oneTime || secretPending) return;
    setSecretPending(true);
    setFailure(undefined);
    try {
      setSaveResult(await saveDeploymentToken(oneTime.metadata.label));
    } catch (error: unknown) {
      setFailure(normalizeActionError(error));
    } finally {
      setSecretPending(false);
    }
  };

  const clear = async () => {
    if (secretPending) return;
    setSecretPending(true);
    setFailure(undefined);
    try {
      await clearDeploymentToken();
      setOneTime(undefined);
      setSaveResult(undefined);
    } catch (error: unknown) {
      setFailure(normalizeActionError(error));
    } finally {
      setSecretPending(false);
    }
  };

  const revoke = async () => {
    if (!revokeTarget || confirmation !== revokeTarget.label || revokePending) {
      return;
    }
    setRevokePending(true);
    setFailure(undefined);
    try {
      const result = await revokeDeploymentToken({
        confirmation,
        label: revokeTarget.label,
      });
      if (result.label !== revokeTarget.label || result.status !== "revoked") {
        throw new Error("The server did not confirm token revocation.");
      }
      setRevokeResult({ ...result });
      setRevokeTarget(undefined);
      setConfirmation("");
      await Promise.all([refreshTokens(), refreshActivity()]);
    } catch (error: unknown) {
      setFailure(normalizeActionError(error));
    } finally {
      setRevokePending(false);
    }
  };

  return (
    <section
      aria-label="Deployment token management"
      className="deployment-token-page"
    >
      <header className="deployment-token-heading">
        <div>
          <span className="page-kicker">Reusable enrollment access</span>
          <h2>Deployment tokens</h2>
          <p>
            Manage long-lived enrollment tokens. Secret values appear once;
            saved files are protected for the current user.
          </p>
        </div>
        <button
          className="deployment-token-primary"
          disabled={Boolean(oneTime)}
          onClick={() => {
            setCreateOpen(true);
            setReviewing(false);
            setFailure(undefined);
          }}
          type="button"
        >
          <Plus aria-hidden="true" size={14} />
          Create deployment token
        </button>
      </header>

      <FailureNotice failure={failure} />

      {oneTime ? (
        <section
          aria-label="One-time deployment token"
          className="deployment-token-secret"
          role="status"
        >
          <div>
            <KeyRound aria-hidden="true" size={20} />
            <div>
              <strong>Save this token now</strong>
              <p>
                <span data-mono>{oneTime.metadata.label}</span> will not reveal
                this value again.
              </p>
            </div>
          </div>
          <code>{oneTime.token}</code>
          <div className="deployment-token-secret-actions">
            <button disabled={secretPending} onClick={() => void copy()} type="button">
              <Clipboard aria-hidden="true" size={14} /> Copy token
            </button>
            <button disabled={secretPending} onClick={() => void save()} type="button">
              <Download aria-hidden="true" size={14} /> Save protected token file
            </button>
            <button disabled={secretPending} onClick={() => void clear()} type="button">
              Clear token
            </button>
          </div>
          {saveResult ? (
            <p className="deployment-token-save-result">
              {saveResult.status === "saved" ? (
                <>Saved to <span data-mono>{saveResult.path}</span></>
              ) : (
                "Save canceled."
              )}
            </p>
          ) : null}
        </section>
      ) : null}

      {createOpen ? (
        <section aria-label="Create deployment token" className="deployment-token-form">
          <header>
            <div>
              <span className="deployment-token-step">New reusable token</span>
              <h3>{reviewing ? "Review token creation" : "Token settings"}</h3>
            </div>
            <button
              disabled={createPending}
              onClick={() => setCreateOpen(false)}
              type="button"
            >
              Cancel
            </button>
          </header>
          {reviewing ? (
            <div className="deployment-token-review">
              <TriangleAlert aria-hidden="true" size={20} />
              <div>
                <p>Create a reusable token named <strong>{label}</strong>?</p>
                <dl>
                  <div><dt>Fleet</dt><dd>{fleet}</dd></div>
                  <div><dt>Lifetime</dt><dd>{lifetimeDays} day(s)</dd></div>
                </dl>
              </div>
              <button
                disabled={createPending}
                onClick={() => void create()}
                type="button"
              >
                {createPending ? "Creating token…" : "Create reusable token"}
              </button>
            </div>
          ) : (
            <div className="deployment-token-fields">
              <label>
                Deployment token label
                <input
                  aria-label="Deployment token label"
                  autoComplete="off"
                  onChange={(event) => setLabel(event.target.value)}
                  value={label}
                />
              </label>
              <label>
                Deployment token Fleet
                <select
                  aria-label="Deployment token Fleet"
                  onChange={(event) => setFleet(event.target.value)}
                  value={fleet}
                >
                  {fleets.map((name) => <option key={name} value={name}>{name}</option>)}
                </select>
              </label>
              <label>
                Deployment token lifetime in days
                <input
                  aria-label="Deployment token lifetime in days"
                  min="1"
                  onChange={(event) => setLifetimeDays(event.target.value)}
                  type="number"
                  value={lifetimeDays}
                />
              </label>
              <button onClick={reviewCreate} type="button">Review token creation</button>
            </div>
          )}
        </section>
      ) : null}

      {revokeTarget ? (
        <section aria-label={`Revoke ${revokeTarget.label}`} className="deployment-token-revoke">
          <TriangleAlert aria-hidden="true" size={20} />
          <div>
            <h3>Revoke {revokeTarget.label}</h3>
            <p>
              New enrollments using this token will stop immediately. Type the
              exact case-sensitive label to continue.
            </p>
            <label>
              Confirm deployment token label
              <input
                aria-label="Confirm deployment token label"
                autoComplete="off"
                onChange={(event) => setConfirmation(event.target.value)}
                value={confirmation}
              />
            </label>
          </div>
          <div className="deployment-token-revoke-actions">
            <button
              disabled={revokePending}
              onClick={() => {
                setRevokeTarget(undefined);
                setConfirmation("");
              }}
              type="button"
            >
              Cancel
            </button>
            <button
              disabled={confirmation !== revokeTarget.label || revokePending}
              onClick={() => void revoke()}
              type="button"
            >
              <Trash2 aria-hidden="true" size={14} />
              {revokePending ? "Revoking token…" : "Revoke deployment token"}
            </button>
          </div>
        </section>
      ) : null}

      {revokeResult ? (
        <p className="deployment-token-result" role="status">
          Revoked at {revokeResult.revokedAt}
        </p>
      ) : null}

      <div className="deployment-token-layout">
        <section aria-label="Deployment token inventory" className="deployment-token-inventory">
          <header>
            <div>
              <h3>Token inventory</h3>
              <p>Metadata only. Existing secret values are never readable.</p>
            </div>
            <span data-numeric>{tokens.length}</span>
          </header>
          {listPending ? <p>Loading deployment tokens…</p> : null}
          {!listPending && tokens.length === 0 ? <p>No deployment tokens.</p> : null}
          {tokens.length > 0 ? (
            <table>
              <thead>
                <tr><th>Label</th><th>Fleet</th><th>Status</th><th>Expires</th><th>Actions</th></tr>
              </thead>
              <tbody>
                {tokens.map((token) => (
                  <tr key={token.id || token.label}>
                    <td><strong data-mono>{token.label}</strong></td>
                    <td>{token.fleet}</td>
                    <td><span className="deployment-token-status" data-status={token.status}>{displayStatus(token.status)}</span></td>
                    <td data-mono>{token.expiresAt}</td>
                    <td>
                      <div className="deployment-token-row-actions">
                        <button aria-label={`Inspect ${token.label}`} onClick={() => void inspect(token)} type="button"><Eye aria-hidden="true" size={14} /> Inspect</button>
                        <button aria-label={`Revoke ${token.label}`} disabled={token.status === "revoked"} onClick={() => { setRevokeTarget(token); setConfirmation(""); setRevokeResult(undefined); }} type="button"><Trash2 aria-hidden="true" size={14} /> Revoke</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
        </section>

        {detail ? (
          <aside aria-label={`Deployment token ${detail.label}`} className="deployment-token-detail">
            <span className="deployment-token-step">Server metadata</span>
            <h3>{detail.label}</h3>
            <dl>
              <div><dt>ID</dt><dd data-mono>{detail.id}</dd></div>
              <div><dt>Fleet</dt><dd>{detail.fleet}</dd></div>
              <div><dt>Status</dt><dd>{displayStatus(detail.status)}</dd></div>
              <div><dt>Created</dt><dd>{detail.createdAt}</dd></div>
              <div><dt>Expires</dt><dd>{detail.expiresAt}</dd></div>
              <div><dt>Last used</dt><dd>{detail.lastUsedAt || "Never"}</dd></div>
            </dl>
          </aside>
        ) : null}
      </div>
    </section>
  );
}
