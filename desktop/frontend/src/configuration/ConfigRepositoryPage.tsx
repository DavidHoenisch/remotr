import { CheckCircle2, FileSearch, FolderGit2, PackagePlus, Save } from "lucide-react";
import { type FormEvent, useState } from "react";

import type {
  ConfigFleetDiscoverRequest,
  ConfigFleetDiscoveryView,
  ConfigHubImportRequest,
  ConfigHubImportResult,
  ConfigHubSnippetView,
  ConfigRenderedArtifactView,
  ConfigRenderRequest,
  ConfigRenderSaveRequest,
  ConfigRenderSaveResult,
  ConfigRenderView,
  ConfigRepositoryInitRequest,
  ConfigRepositoryInitResult,
  ConfigValidationView,
  ConfigWorkingTreeView,
} from "./configRepository";
import "./ConfigRepositoryPage.css";

interface ConfigRepositoryPageProps {
  chooseRepository: () => Promise<ConfigWorkingTreeView>;
  discoverFleet: (request: ConfigFleetDiscoverRequest) => Promise<ConfigFleetDiscoveryView>;
  importHubSnippet: (request: ConfigHubImportRequest) => Promise<ConfigHubImportResult>;
  initializeRepository: (request: ConfigRepositoryInitRequest) => Promise<ConfigRepositoryInitResult>;
  listHubSnippets: (workingTreeId: string) => Promise<ConfigHubSnippetView[]>;
  renderRepository: (request: ConfigRenderRequest) => Promise<ConfigRenderView>;
  saveRender: (request: ConfigRenderSaveRequest) => Promise<ConfigRenderSaveResult>;
  validateRepository: (workingTreeId: string) => Promise<ConfigValidationView>;
}

export function ConfigRepositoryPage({
  chooseRepository,
  discoverFleet,
  importHubSnippet,
  initializeRepository,
  listHubSnippets,
  renderRepository,
  saveRender,
  validateRepository,
}: ConfigRepositoryPageProps) {
  const [workingTree, setWorkingTree] = useState<ConfigWorkingTreeView>();
  const [validation, setValidation] = useState<ConfigValidationView>();
  const [fleet, setFleet] = useState("");
  const [discovery, setDiscovery] = useState<ConfigFleetDiscoveryView>();
  const [renderScope, setRenderScope] = useState<"endpoint" | "fleet">("fleet");
  const [renderTarget, setRenderTarget] = useState("");
  const [rendered, setRendered] = useState<ConfigRenderView>();
  const [snippets, setSnippets] = useState<ConfigHubSnippetView[]>([]);
  const [importEntry, setImportEntry] = useState<ConfigHubSnippetView>();
  const [outPath, setOutPath] = useState("");
  const [imported, setImported] = useState<ConfigHubImportResult>();
  const [showInitialize, setShowInitialize] = useState(false);
  const [initialFleet, setInitialFleet] = useState("");
  const [remediationPolicy, setRemediationPolicy] = useState<"auto" | "report">("auto");
  const [saved, setSaved] = useState<ConfigRenderSaveResult>();
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");

  const run = async (name: string, action: () => Promise<void>) => {
    setPending(name);
    setError("");
    try {
      await action();
    } catch (cause) {
      setError(actionMessage(cause));
    } finally {
      setPending("");
    }
  };

  const selectWorkingTree = async () => {
    await run("choose", async () => {
      const selected = await chooseRepository();
      if (selected.status === "canceled") return;
      setWorkingTree(selected);
      setValidation(undefined);
      setDiscovery(undefined);
      setRendered(undefined);
      setSnippets(await listHubSnippets(selected.id));
    });
  };

  const submitImport = async (event: FormEvent) => {
    event.preventDefault();
    if (!workingTree || !importEntry) return;
    await run("import", async () => {
      setImported(await importHubSnippet({ entryId: importEntry.id, outPath, workingTreeId: workingTree.id }));
      setImportEntry(undefined);
    });
  };

  const submitInitialize = async (event: FormEvent) => {
    event.preventDefault();
    await run("initialize", async () => {
      const result = await initializeRepository({ fleet: initialFleet, remediationPolicy });
      if (result.status === "canceled") return;
      setWorkingTree(result.workingTree);
      setValidation(undefined);
      setDiscovery(undefined);
      setRendered(undefined);
      setSnippets(await listHubSnippets(result.workingTree.id));
      setShowInitialize(false);
    });
  };

  const saveArtifact = async (artifact: ConfigRenderedArtifactView) => {
    if (!workingTree) return;
    await run(`save-${artifact.digest}`, async () => {
      setSaved(await saveRender({
        artifactType: artifact.artifactType,
        digest: artifact.digest,
        targetId: artifact.targetId,
        targetType: artifact.targetType,
        workingTreeId: workingTree.id,
      }));
    });
  };

  return (
    <section aria-label="Configuration repository" className="config-page">
      <header className="config-heading">
        <div>
          <span className="page-kicker">Explicit local working tree</span>
          <h2>Configuration repository</h2>
          <p>Review and prepare repository content locally. This app never stages, commits, pushes, merges, or applies generated content.</p>
        </div>
        <FolderGit2 aria-hidden="true" size={30} strokeWidth={1.5} />
      </header>

      {error ? <p className="config-error" role="alert">{error}</p> : null}

      <div className="config-toolbar">
        <button disabled={Boolean(pending)} onClick={() => void selectWorkingTree()} type="button">Choose working tree</button>
        <button disabled={Boolean(pending)} onClick={() => setShowInitialize(true)} type="button">Initialize repository</button>
        {workingTree ? <strong><CheckCircle2 aria-hidden="true" size={16} />{workingTree.directoryName}</strong> : <span>No local working tree selected.</span>}
      </div>

      {workingTree ? (
        <div className="config-grid">
          <section className="config-card">
            <div className="config-card-heading"><FileSearch aria-hidden="true" size={18} /><h3>Validate &amp; discover</h3></div>
            <button disabled={Boolean(pending)} onClick={() => void run("validate", async () => setValidation(await validateRepository(workingTree.id)))} type="button">Validate repository</button>
            {validation ? (
              <div aria-label="Repository validation" className="config-result" role="status">
                <strong>{validation.valid ? "Valid configuration repository" : "Configuration repository has issues"}</strong>
                {validation.issues.map((issue) => <p key={`${issue.path}-${issue.message}`}><span data-mono>{issue.path}</span> {issue.message}</p>)}
              </div>
            ) : null}
            <label><span>Fleet name</span><input aria-label="Fleet name" value={fleet} onChange={(event) => setFleet(event.target.value)} /></label>
            <button disabled={Boolean(pending) || !fleet} onClick={() => void run("discover", async () => setDiscovery(await discoverFleet({ fleet, workingTreeId: workingTree.id })))} type="button">Discover Fleet files</button>
            {discovery ? (
              <div aria-label="Fleet discovery" className="config-result" role="status">
                <strong>{discovery.fleet}</strong>
                <span data-mono>{discovery.manifest}</span>
                {[...discovery.modules, ...discovery.applications, ...discovery.crons].map((path) => <span data-mono key={path}>{path}</span>)}
              </div>
            ) : null}
          </section>

          <section className="config-card">
            <div className="config-card-heading"><Save aria-hidden="true" size={18} /><h3>Render preview</h3></div>
            <label><span>Render scope</span><select aria-label="Render scope" value={renderScope} onChange={(event) => setRenderScope(event.target.value as "endpoint" | "fleet")}><option value="fleet">Fleet</option><option value="endpoint">Endpoint</option></select></label>
            <label><span>Render target</span><input aria-label="Render target" value={renderTarget} onChange={(event) => setRenderTarget(event.target.value)} /></label>
            <button disabled={Boolean(pending) || !renderTarget} onClick={() => void run("render", async () => setRendered(await renderRepository({ scope: renderScope, targetId: renderTarget, workingTreeId: workingTree.id })))} type="button">Render preview</button>
            {saved ? <p className="config-notice" role="status">Saved {saved.fileName}</p> : null}
          </section>

          <section className="config-card config-hub">
            <div className="config-card-heading"><PackagePlus aria-hidden="true" size={18} /><h3>Hub snippets</h3></div>
            <p>Import reviewed YAML into a repository-relative module path.</p>
            {snippets.length === 0 ? <span>No Hub snippets are available.</span> : snippets.map((snippet) => (
              <article key={snippet.id}>
                <div><strong>{snippet.title}</strong><span>{snippet.description}</span></div>
                <button disabled={Boolean(pending)} onClick={() => { setImportEntry(snippet); setOutPath(`modules/${snippet.id}.yaml`); }} type="button">Import {snippet.title}</button>
              </article>
            ))}
            {imported ? <p className="config-notice" role="status">Imported {imported.outPath}</p> : null}
          </section>
        </div>
      ) : null}

      {rendered ? (
        <section aria-label="Rendered artifacts" className="config-artifacts">
          <div><span className="page-kicker">Bounded local preview</span><h3>Rendered artifacts</h3></div>
          {rendered.artifacts.map((artifact) => (
            <article key={`${artifact.targetType}-${artifact.targetId}-${artifact.artifactType}`}>
              <header><strong>{artifact.targetId} · {artifact.artifactType}</strong><button disabled={Boolean(pending)} onClick={() => void saveArtifact(artifact)} type="button">Save {artifact.targetId} {artifact.artifactType}</button></header>
              <pre>{artifact.content}</pre>
            </article>
          ))}
        </section>
      ) : null}

      {importEntry ? (
        <form aria-label="Import Hub snippet" className="config-dialog" onSubmit={submitImport} role="dialog">
          <span className="page-kicker">Local repository write</span><h3>Import {importEntry.title}</h3>
          <label><span>Repository-relative output path</span><input aria-label="Repository-relative output path" value={outPath} onChange={(event) => setOutPath(event.target.value)} /></label>
          <div><button onClick={() => setImportEntry(undefined)} type="button">Cancel</button><button disabled={Boolean(pending)} type="submit">Import snippet</button></div>
        </form>
      ) : null}

      {showInitialize ? (
        <form aria-label="Initialize Configuration repository" className="config-dialog" onSubmit={submitInitialize} role="dialog">
          <span className="page-kicker">Empty local directory</span><h3>Initialize Configuration repository</h3>
          <p>Creates starter YAML only after you choose an empty directory.</p>
          <label><span>Initial Fleet</span><input aria-label="Initial Fleet" value={initialFleet} onChange={(event) => setInitialFleet(event.target.value)} /></label>
          <label><span>Remediation policy</span><select aria-label="Remediation policy" value={remediationPolicy} onChange={(event) => setRemediationPolicy(event.target.value as "auto" | "report")}><option value="auto">Auto</option><option value="report">Report only</option></select></label>
          <div><button onClick={() => setShowInitialize(false)} type="button">Cancel</button><button disabled={Boolean(pending)} type="submit">Choose empty directory and initialize</button></div>
        </form>
      ) : null}
    </section>
  );
}

function actionMessage(cause: unknown): string {
  return cause && typeof cause === "object" && "message" in cause && typeof cause.message === "string"
    ? cause.message
    : "The local Configuration repository action could not be completed safely.";
}
