import { Bot, CheckCircle2, FolderOpen, RefreshCw, TriangleAlert } from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";

import type {
  AIIntegrationActionResult,
  AIIntegrationInstallRequest,
  AIIntegrationListRequest,
  AIIntegrationUpgradeRequest,
  AIIntegrationView,
  AIProjectRootView,
} from "./aiIntegration";
import "./AIIntegrationPage.css";

interface AIIntegrationPageProps {
  chooseProjectRoot: () => Promise<AIProjectRootView>;
  listIntegrations: (request: AIIntegrationListRequest) => Promise<AIIntegrationView[]>;
  setupIntegration: (request: AIIntegrationInstallRequest) => Promise<AIIntegrationActionResult>;
  upgradeIntegration: (request: AIIntegrationUpgradeRequest) => Promise<AIIntegrationActionResult>;
}

interface IntegrationEditor {
  integration: AIIntegrationView;
  kind: "setup" | "upgrade";
}

export function AIIntegrationPage({
  chooseProjectRoot,
  listIntegrations,
  setupIntegration,
  upgradeIntegration,
}: AIIntegrationPageProps) {
  const [scope, setScope] = useState<"project" | "user">("user");
  const [projectRoot, setProjectRoot] = useState<AIProjectRootView>();
  const [integrations, setIntegrations] = useState<AIIntegrationView[]>([]);
  const [editor, setEditor] = useState<IntegrationEditor>();
  const [replace, setReplace] = useState(false);
  const [version, setVersion] = useState("");
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const load = async (nextScope: "project" | "user", projectRootId = "") => {
    setIntegrations(await listIntegrations({ projectRootId, scope: nextScope }));
  };

  useEffect(() => {
    let current = true;
    void listIntegrations({ projectRootId: "", scope: "user" })
      .then((items) => current && setIntegrations(items))
      .catch((cause) => current && setError(actionMessage(cause)));
    return () => {
      current = false;
    };
  }, [listIntegrations]);

  const chooseProject = async () => {
    setPending("project");
    setError("");
    try {
      const selected = await chooseProjectRoot();
      if (selected.status === "canceled") return;
      setProjectRoot(selected);
      await load("project", selected.id);
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending("");
    }
  };

  const changeScope = async (nextScope: "project" | "user") => {
    setScope(nextScope);
    setError("");
    setNotice("");
    setPending("scope");
    try {
      if (nextScope === "user") {
        await load("user");
      } else if (projectRoot) {
        await load("project", projectRoot.id);
      } else {
        setIntegrations([]);
      }
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending("");
    }
  };

  const openEditor = (integration: AIIntegrationView, kind: "setup" | "upgrade") => {
    setEditor({ integration, kind });
    setReplace(false);
    setVersion("");
    setError("");
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!editor) return;
    const projectRootId = scope === "project" ? projectRoot?.id ?? "" : "";
    setPending(editor.kind);
    setError("");
    setNotice("");
    try {
      const base = { agent: editor.integration.agent, projectRootId, replace, scope };
      const result = editor.kind === "setup"
        ? await setupIntegration(base)
        : await upgradeIntegration({ ...base, version });
      setNotice(`${result.integration.displayName} ${result.status}.`);
      setEditor(undefined);
      await load(scope, projectRootId);
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending("");
    }
  };

  return (
    <section aria-label="AI integrations" className="ai-page">
      <header className="ai-heading">
        <div>
          <span className="page-kicker">Local agent skills</span>
          <h2>AI integrations</h2>
          <p>Choose user or project scope explicitly. Setup and upgrades write only the selected runtime skill directory and never invoke the Remotr CLI.</p>
        </div>
        <Bot aria-hidden="true" size={30} strokeWidth={1.5} />
      </header>

      {error ? <p className="ai-error" role="alert">{error}</p> : null}
      {notice ? <p className="ai-notice" role="status">{notice}</p> : null}

      <section className="ai-scope" aria-labelledby="ai-scope-heading">
        <div><h3 id="ai-scope-heading">Installation scope</h3><p>The project scope requires a native-selected local directory.</p></div>
        <label><span>Installation scope</span><select aria-label="Installation scope" value={scope} onChange={(event) => void changeScope(event.target.value as "project" | "user")}><option value="user">Current user</option><option value="project">Selected project</option></select></label>
        {scope === "project" ? <button disabled={Boolean(pending)} onClick={() => void chooseProject()} type="button"><FolderOpen aria-hidden="true" size={15} />Choose project directory</button> : null}
        {scope === "project" && projectRoot ? <strong><CheckCircle2 aria-hidden="true" size={15} />{projectRoot.directoryName}</strong> : null}
      </section>

      {scope === "project" && !projectRoot ? (
        <section className="ai-empty"><h3>Choose the project boundary</h3><p>No project files can be changed until you select its root directory.</p></section>
      ) : (
        <div className="ai-grid">
          {integrations.map((integration) => (
            <article className="ai-card" key={integration.agent}>
              <header><div><Bot aria-hidden="true" size={18} /><h3>{integration.displayName}</h3></div><span data-status={integration.installed ? "installed" : "available"}>{integration.installed ? "Installed" : "Not installed"}</span></header>
              <dl>
                <div><dt>Skill bundle</dt><dd>{integration.bundleVersion || "—"}</dd></div>
                <div><dt>Runtime</dt><dd>{integration.runtimeAvailable ? "Available" : "Runtime not found"}</dd></div>
              </dl>
              {!integration.runtimeAvailable ? <p className="ai-runtime-guidance"><TriangleAlert aria-hidden="true" size={15} />{integration.guidance}</p> : null}
              <div className="ai-actions">
                <button disabled={Boolean(pending)} onClick={() => openEditor(integration, "setup")} type="button">Set up {integration.displayName}</button>
                <button disabled={Boolean(pending)} onClick={() => openEditor(integration, "upgrade")} type="button"><RefreshCw aria-hidden="true" size={14} />Upgrade {integration.displayName}</button>
              </div>
            </article>
          ))}
        </div>
      )}

      {editor ? (
        <form aria-label={editor.kind === "setup" ? "Set up AI integration" : "Upgrade AI integration"} className="ai-dialog" onSubmit={submit} role="dialog">
          <span className="page-kicker">{scope === "user" ? "Current user scope" : `Project scope · ${projectRoot?.directoryName ?? "not selected"}`}</span>
          <h3>{editor.kind === "setup" ? "Set up" : "Upgrade"} {editor.integration.displayName}</h3>
          {editor.kind === "upgrade" ? <label><span>Release version</span><input aria-label="Release version" placeholder="Latest stable" value={version} onChange={(event) => setVersion(event.target.value)} /></label> : null}
          <label className="ai-checkbox"><input aria-label="Replace existing installation" checked={replace} onChange={(event) => setReplace(event.target.checked)} type="checkbox" /><span>Replace existing installation</span></label>
          <p>Replacement occurs only after this explicit confirmation. A failed download leaves the current installation available.</p>
          <div><button onClick={() => setEditor(undefined)} type="button">Cancel</button><button disabled={Boolean(pending)} type="submit">{editor.kind === "setup" ? "Install integration" : "Download and upgrade"}</button></div>
        </form>
      ) : null}
    </section>
  );
}

function actionMessage(cause: unknown): string {
  return cause && typeof cause === "object" && "message" in cause && typeof cause.message === "string"
    ? cause.message
    : "The AI integration action could not be completed safely.";
}
