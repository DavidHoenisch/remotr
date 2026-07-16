import { KeyRound, RefreshCw, ShieldCheck, Upload } from "lucide-react";
import { type FormEvent, useState } from "react";

import type {
  SecretLifecycleRequest,
  SecretUploadRequest,
  SecretVersionView,
} from "./secret";
import "./SecretPage.css";

interface SecretEndpointOption {
  endpointId: string;
  label: string;
}

interface SecretPageProps {
  activateSecretVersion: (
    request: SecretLifecycleRequest,
  ) => Promise<SecretVersionView>;
  endpoints: SecretEndpointOption[];
  fleets: string[];
  listSecretVersions: (name: string) => Promise<SecretVersionView[]>;
  refreshActivity: () => Promise<unknown>;
  revokeSecretVersion: (
    request: SecretLifecycleRequest,
  ) => Promise<SecretVersionView>;
  uploadSecretVersion: (
    request: SecretUploadRequest,
  ) => Promise<SecretVersionView>;
}

type LifecycleAction = "activate" | "revoke";

interface LifecycleSelection {
  action: LifecycleAction;
  version: SecretVersionView;
}

export function SecretPage({
  activateSecretVersion,
  endpoints,
  fleets,
  listSecretVersions,
  refreshActivity,
  revokeSecretVersion,
  uploadSecretVersion,
}: SecretPageProps) {
  const [name, setName] = useState("");
  const [scopeType, setScopeType] = useState<"fleet" | "endpoint">("fleet");
  const [scopeId, setScopeId] = useState("");
  const [versions, setVersions] = useState<SecretVersionView[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [selection, setSelection] = useState<LifecycleSelection>();
  const [confirmation, setConfirmation] = useState("");

  const replaceVersion = (next: SecretVersionView) => {
    setVersions((current) =>
      [...current.filter((item) => item.version !== next.version), next].toSorted(
        (left, right) => Number(left.version) - Number(right.version),
      ),
    );
  };

  const load = async () => {
    if (!name) return;
    setPending(true);
    setError("");
    try {
      setVersions(await listSecretVersions(name));
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending(false);
    }
  };

  const upload = async () => {
    setPending(true);
    setError("");
    try {
      const uploaded = await uploadSecretVersion({ name, scopeId, scopeType });
      setVersions(await listSecretVersions(name));
      replaceVersion(uploaded);
      await refreshActivity();
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending(false);
    }
  };

  const openLifecycle = (action: LifecycleAction, version: SecretVersionView) => {
    setConfirmation("");
    setSelection({ action, version });
  };

  const submitLifecycle = async (event: FormEvent) => {
    event.preventDefault();
    if (!selection) return;
    setPending(true);
    setError("");
    try {
      const request = {
        confirmation,
        name: selection.version.name,
        version: selection.version.version,
      };
      const changed =
        selection.action === "activate"
          ? await activateSecretVersion(request)
          : await revokeSecretVersion(request);
      setVersions(await listSecretVersions(name));
      replaceVersion(changed);
      await refreshActivity();
      setSelection(undefined);
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending(false);
    }
  };

  const scopeOptions =
    scopeType === "fleet"
      ? fleets.map((fleet) => ({ id: fleet, label: fleet }))
      : endpoints.map((endpoint) => ({
          id: endpoint.endpointId,
          label: endpoint.label,
        }));

  return (
    <section aria-label="Encrypted Secret management" className="secret-page">
      <header className="secret-page-heading">
        <div>
          <span className="page-kicker">Encrypted registry</span>
          <h2>Secrets</h2>
          <p>
            Upload protected native input, review version fingerprints, and plan
            lifecycle changes without placing sensitive values in browser state.
          </p>
        </div>
        <ShieldCheck aria-hidden="true" size={28} strokeWidth={1.6} />
      </header>

      {error ? <p className="secret-error" role="alert">{error}</p> : null}

      <div className="secret-controls">
        <label>
          <span>Secret name</span>
          <input
            autoComplete="off"
            onChange={(event) => setName(event.target.value)}
            placeholder="network/office"
            value={name}
          />
        </label>
        <button disabled={pending || !name} onClick={load} type="button">
          <RefreshCw aria-hidden="true" size={15} />
          List versions
        </button>
      </div>

      <div className="secret-upload-panel">
        <div>
          <KeyRound aria-hidden="true" size={20} />
          <strong>Upload a new encrypted version</strong>
        </div>
        <label>
          <span>Scope</span>
          <select
            onChange={(event) => {
              setScopeType(event.target.value as "fleet" | "endpoint");
              setScopeId("");
            }}
            value={scopeType}
          >
            <option value="fleet">Fleet</option>
            <option value="endpoint">Endpoint</option>
          </select>
        </label>
        <label>
          <span>{scopeType === "fleet" ? "Fleet" : "Endpoint"}</span>
          <select onChange={(event) => setScopeId(event.target.value)} value={scopeId}>
            <option value="">Select {scopeType}</option>
            {scopeOptions.map((option) => (
              <option key={option.id} value={option.id}>{option.label}</option>
            ))}
          </select>
        </label>
        <button disabled={pending || !name || !scopeId} onClick={upload} type="button">
          <Upload aria-hidden="true" size={15} />
          Choose protected file and upload
        </button>
      </div>

      <div className="secret-table-wrap">
        <table aria-label="Secret versions">
          <thead>
            <tr>
              <th>Version</th><th>Status</th><th>Scope</th><th>Fingerprint</th><th>Created</th><th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {versions.map((version) => (
              <tr key={version.version}>
                <td data-mono>{version.version}</td>
                <td><span className={`secret-status secret-status-${version.status}`}>{version.status.replace("_", " ")}</span></td>
                <td>{version.scopeType} · <span data-mono>{version.scopeId}</span></td>
                <td data-mono>{version.fingerprint}</td>
                <td>{version.createdAt}</td>
                <td className="secret-actions">
                  <button disabled={pending || version.status === "revoked"} onClick={() => openLifecycle("activate", version)} type="button" aria-label={`Activate ${version.name} ${version.version}`}>Activate</button>
                  <button disabled={pending || version.status === "revoked"} onClick={() => openLifecycle("revoke", version)} type="button" aria-label={`Revoke ${version.name} ${version.version}`}>Revoke</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {versions.flatMap((version) => version.rollouts).map((rollout) => (
          <article className="secret-rollout" key={`${rollout.resourceAddress}-${rollout.changeRequestId}`}>
            <span className="page-kicker">Activation rollout · {rollout.risk}</span>
            <strong>{rollout.resourceAddress}</strong>
            <span>{rollout.purpose} · {rollout.fleet}</span>
            {rollout.changeRequestId ? <span>Change request <b data-mono>{rollout.changeRequestId}</b></span> : null}
          </article>
        ))}
      </div>

      {selection ? (
        <form aria-label={`${titleCase(selection.action)} Secret version`} className="secret-dialog" onSubmit={submitLifecycle} role="dialog">
          <span className="page-kicker">Destructive confirmation</span>
          <h3>{titleCase(selection.action)} {selection.version.name}@{selection.version.version}</h3>
          <p>Type <strong data-mono>{selection.version.name}@{selection.version.version} {selection.action.toUpperCase()}</strong> exactly.</p>
          <label>
            <span>{selection.action === "activate" ? "Activation" : "Revocation"} confirmation</span>
            <input autoComplete="off" onChange={(event) => setConfirmation(event.target.value)} value={confirmation} />
          </label>
          <div>
            <button disabled={pending} onClick={() => setSelection(undefined)} type="button">Cancel</button>
            <button disabled={pending} type="submit">{selection.action === "activate" ? "Plan activation" : "Revoke version"}</button>
          </div>
        </form>
      ) : null}
    </section>
  );
}

function actionMessage(cause: unknown): string {
  if (cause && typeof cause === "object" && "message" in cause && typeof cause.message === "string") {
    return cause.message;
  }
  return "The Secret action could not be completed safely.";
}

function titleCase(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

