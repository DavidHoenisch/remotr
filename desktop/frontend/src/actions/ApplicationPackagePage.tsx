import { Archive, Box, Check, FolderPlus, PackageOpen, ShieldCheck, Trash2 } from "lucide-react";
import { type FormEvent, useCallback, useEffect, useState } from "react";

import type {
  AppPackageArchiveView,
  AppPackageDeleteRequest,
  AppPackageDeleteResult,
  AppPackagePublishRequest,
  AppPackageView,
  LocalPackageCreateRequest,
  LocalPackageView,
} from "./applicationPackage";
import "./ApplicationPackagePage.css";

interface ApplicationPackagePageProps {
  buildLocalPackage: () => Promise<AppPackageArchiveView>;
  chooseAppPackageArchive: () => Promise<AppPackageArchiveView>;
  chooseLocalPackageSource: () => Promise<LocalPackageView>;
  createLocalPackage: (request: LocalPackageCreateRequest) => Promise<LocalPackageView>;
  deleteAppPackage: (request: AppPackageDeleteRequest) => Promise<AppPackageDeleteResult>;
  listAppPackages: (prefix: string) => Promise<AppPackageView[]>;
  loadAppPackage: (name: string, version: string) => Promise<AppPackageView>;
  publishAppPackage: (request: AppPackagePublishRequest) => Promise<AppPackageView>;
  refreshActivity?: () => Promise<void>;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "The native package action failed.";
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

export function ApplicationPackagePage({
  buildLocalPackage,
  chooseAppPackageArchive,
  chooseLocalPackageSource,
  createLocalPackage,
  deleteAppPackage,
  listAppPackages,
  loadAppPackage,
  publishAppPackage,
  refreshActivity,
}: ApplicationPackagePageProps) {
  const [packages, setPackages] = useState<AppPackageView[]>([]);
  const [detail, setDetail] = useState<AppPackageView>();
  const [archive, setArchive] = useState<AppPackageArchiveView>();
  const [source, setSource] = useState<LocalPackageView>();
  const [localResult, setLocalResult] = useState<LocalPackageView>();
  const [publishConfirmation, setPublishConfirmation] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<AppPackageView>();
  const [deleteObject, setDeleteObject] = useState(false);
  const [deleteConfirmation, setDeleteConfirmation] = useState("");
  const [directoryName, setDirectoryName] = useState("");
  const [packageName, setPackageName] = useState("");
  const [version, setVersion] = useState("0.1.0");
  const [mode, setMode] = useState<LocalPackageCreateRequest["mode"]>("binary");
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const refreshCatalog = useCallback(async () => {
    const loaded = await listAppPackages("");
    setPackages(loaded);
  }, [listAppPackages]);

  useEffect(() => {
    void refreshCatalog().catch((cause: unknown) => setError(errorMessage(cause)));
  }, [refreshCatalog]);

  const run = async (name: string, action: () => Promise<void>) => {
    setPending(name);
    setError("");
    setNotice("");
    try {
      await action();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending("");
    }
  };

  const chooseArchive = () => run("archive", async () => {
    const selected = await chooseAppPackageArchive();
    if (selected.name) {
      setArchive(selected);
      setPublishConfirmation("");
      setNotice(`Validated ${selected.fileName}.`);
    }
  });

  const publish = (event: FormEvent) => {
    event.preventDefault();
    if (!archive) return;
    void run("publish", async () => {
      const published = await publishAppPackage({
        confirmation: publishConfirmation,
        name: archive.name,
        sha256: archive.sha256,
        version: archive.version,
      });
      setDetail(published);
      setNotice(`Published ${published.name}@${published.version}.`);
      await refreshCatalog();
      await refreshActivity?.();
    });
  };

  const createLocal = (event: FormEvent) => {
    event.preventDefault();
    void run("create", async () => {
      const created = await createLocalPackage({ directoryName, mode, name: packageName, version });
      if (created.name) {
        setLocalResult(created);
        setSource(created);
        setNotice(`Created ${created.name}@${created.version} in ${created.locationName}.`);
      }
    });
  };

  const chooseSource = () => run("source", async () => {
    const selected = await chooseLocalPackageSource();
    if (selected.name) {
      setSource(selected);
      setNotice(`Selected ${selected.locationName}.`);
    }
  });

  const build = () => run("build", async () => {
    const built = await buildLocalPackage();
    if (built.name) {
      setArchive(built);
      setPublishConfirmation("");
      setNotice(`Built and validated ${built.fileName}.`);
    }
  });

  const inspect = (item: AppPackageView) => run("inspect", async () => {
    setDetail(await loadAppPackage(item.name, item.version));
  });

  const remove = (event: FormEvent) => {
    event.preventDefault();
    if (!deleteTarget) return;
    void run("delete", async () => {
      const result = await deleteAppPackage({
        confirmation: deleteConfirmation,
        deleteObject,
        name: deleteTarget.name,
        version: deleteTarget.version,
      });
      setDeleteTarget(undefined);
      setDeleteConfirmation("");
      setDeleteObject(false);
      setDetail(undefined);
      setNotice(result.scope === "catalog_and_object" ? "Deleted catalog entry and stored object." : "Deleted catalog entry; stored object retained.");
      await refreshCatalog();
      await refreshActivity?.();
    });
  };

  const deletePhrase = deleteTarget
    ? `${deleteTarget.name}@${deleteTarget.version}${deleteObject ? " DELETE OBJECT" : " CATALOG ONLY"}`
    : "";

  return (
    <section aria-label="Application package management" className="application-packages-page">
      <div className="package-lede">
        <div>
          <span className="page-kicker">Artifact ledger</span>
          <h2>Application packages</h2>
          <p>Build locally, verify every byte, then publish through the connected Remotr server.</p>
        </div>
        <div className="package-trust-mark">
          <ShieldCheck aria-hidden="true" size={20} />
          <span><strong>Native custody</strong>Archive bytes and credentials stay outside the webview.</span>
        </div>
      </div>

      {error ? <div className="package-message" data-kind="error" role="alert">{error}</div> : null}
      {notice ? <div className="package-message" data-kind="success" role="status">{notice}</div> : null}

      <div className="package-layout">
        <section className="package-panel package-catalog" aria-labelledby="package-catalog-heading">
          <header>
            <div><span className="package-index">01</span><h3 id="package-catalog-heading">Published catalog</h3></div>
            <span className="package-count">{packages.length} objects</span>
          </header>
          <div className="package-table-wrap">
            <table>
              <thead><tr><th>Package</th><th>Version</th><th>Integrity</th><th><span className="sr-only">Actions</span></th></tr></thead>
              <tbody>
                {packages.map((item) => (
                  <tr key={`${item.name}@${item.version}`}>
                    <td data-mono>{item.name}</td><td data-mono>{item.version}</td><td data-mono>{item.sha256.slice(0, 12)}…</td>
                    <td className="package-row-actions">
                      <button aria-label={`Inspect ${item.name} ${item.version}`} onClick={() => void inspect(item)} type="button">Inspect</button>
                      <button aria-label={`Delete ${item.name} ${item.version}`} className="danger-link" onClick={() => { setDeleteTarget(item); setDeleteConfirmation(""); setDeleteObject(false); }} type="button"><Trash2 aria-hidden="true" size={14} />Delete</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {detail ? (
            <dl className="package-detail">
              <div><dt>Identity</dt><dd data-mono>{detail.name}@{detail.version}</dd></div>
              <div><dt>Install mode</dt><dd>{detail.installMode}</dd></div>
              <div><dt>Object key</dt><dd data-mono>{detail.objectKey}</dd></div>
              <div><dt>SHA-256</dt><dd data-mono>{detail.sha256}</dd></div>
            </dl>
          ) : null}
        </section>

        <section className="package-panel package-archive" aria-labelledby="package-archive-heading">
          <header><div><span className="package-index">02</span><h3 id="package-archive-heading">Validate & publish</h3></div><Archive aria-hidden="true" size={18} /></header>
          <p>Select a ZIP through the native picker. Remotr validates its manifest and records a full SHA-256 digest.</p>
          <button className="package-secondary" disabled={Boolean(pending)} onClick={() => void chooseArchive()} type="button"><PackageOpen aria-hidden="true" size={16} />Choose package archive</button>
          {archive ? (
            <form className="archive-proof" onSubmit={publish}>
              <div className="archive-identity"><Check aria-hidden="true" size={16} /><span><strong>{archive.name}@{archive.version}</strong><small>{archive.fileName} · {formatBytes(archive.sizeBytes)} · {archive.mode}</small></span></div>
              <code>{archive.sha256}</code>
              <label>Publish confirmation<input aria-label="Publish confirmation" autoComplete="off" onChange={(event) => setPublishConfirmation(event.target.value)} spellCheck={false} value={publishConfirmation} /></label>
              <small>Type <code>{archive.name}@{archive.version}</code> exactly.</small>
              <button className="package-primary" disabled={pending !== "" || publishConfirmation !== `${archive.name}@${archive.version}`} type="submit">Publish archive</button>
            </form>
          ) : <div className="package-empty">No archive selected.</div>}
        </section>

        <section className="package-panel package-local" aria-labelledby="package-local-heading">
          <header><div><span className="package-index">03</span><h3 id="package-local-heading">Local package workshop</h3></div><FolderPlus aria-hidden="true" size={18} /></header>
          <form className="local-package-form" onSubmit={createLocal}>
            <label>Directory name<input aria-label="Package directory name" onChange={(event) => setDirectoryName(event.target.value)} value={directoryName} /></label>
            <label>Catalog name<input aria-label="Local package name" onChange={(event) => setPackageName(event.target.value)} value={packageName} /></label>
            <label>Version<input aria-label="Local package version" onChange={(event) => setVersion(event.target.value)} value={version} /></label>
            <label>Install mode<select aria-label="Local package install mode" onChange={(event) => setMode(event.target.value as LocalPackageCreateRequest["mode"])} value={mode}><option value="binary">Binary</option><option value="script">Script</option><option value="build">Build</option></select></label>
            <button className="package-secondary" disabled={Boolean(pending) || !directoryName || !packageName || !version} type="submit">Choose parent and create</button>
          </form>
          {localResult ? <p className="local-result"><Box aria-hidden="true" size={15} />Created <strong>{localResult.locationName}</strong>; files remain unstaged and unpublished.</p> : null}
          <div className="build-actions">
            <button className="package-secondary" disabled={Boolean(pending)} onClick={() => void chooseSource()} type="button">Choose package source</button>
            <button className="package-primary" disabled={Boolean(pending) || !source} onClick={() => void build()} type="button">Build archive</button>
          </div>
          {source ? <small>Ready: <span data-mono>{source.name}@{source.version}</span> from {source.locationName}</small> : null}
        </section>
      </div>

      {deleteTarget ? (
        <div className="package-delete-layer">
          <form aria-label="Delete application package" className="package-delete-dialog" onSubmit={remove} role="dialog">
            <span className="page-kicker">Destructive catalog action</span>
            <h3>Delete {deleteTarget.name}@{deleteTarget.version}</h3>
            <p>Catalog removal stops future deployments. Object deletion also removes the stored archive and cannot be undone.</p>
            <label className="delete-object-choice"><input aria-label="Also delete stored object" checked={deleteObject} onChange={(event) => { setDeleteObject(event.target.checked); setDeleteConfirmation(""); }} type="checkbox" />Also delete stored object</label>
            <label>Delete confirmation<input aria-label="Delete confirmation" autoComplete="off" onChange={(event) => setDeleteConfirmation(event.target.value)} spellCheck={false} value={deleteConfirmation} /></label>
            <small>Type <code>{deletePhrase}</code> exactly.</small>
            <div className="package-delete-actions">
              <button onClick={() => setDeleteTarget(undefined)} type="button">Cancel</button>
              <button className="package-danger" disabled={deleteConfirmation !== deletePhrase || Boolean(pending)} type="submit">{deleteObject ? "Delete package and object" : "Delete catalog entry"}</button>
            </div>
          </form>
        </div>
      ) : null}
    </section>
  );
}
