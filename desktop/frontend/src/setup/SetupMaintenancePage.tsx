import {
  BookOpen,
  Cable,
  CircleCheck,
  Download,
  KeyRound,
  Plus,
  Stethoscope,
  Wrench,
} from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";

import type {
  ConnectionProfile,
  ConnectionView,
  DesktopDoctorReport,
  DesktopUpdateStatus,
  SetupMaintenanceView,
} from "./setupMaintenance";
import "./SetupMaintenancePage.css";

interface SetupMaintenancePageProps {
  bootstrapProfile: (
    profile: ConnectionProfile,
    token: string,
  ) => Promise<ConnectionView>;
  checkDesktopUpdate: () => Promise<DesktopUpdateStatus>;
  connectProfile: (profile: ConnectionProfile) => Promise<ConnectionView>;
  loadSetup: () => Promise<SetupMaintenanceView>;
  onConnected: (connection: ConnectionView) => void;
  onCreateEnrollmentToken?: () => void;
  openDocumentation: () => Promise<void>;
  runDoctor: (profile: ConnectionProfile) => Promise<DesktopDoctorReport>;
  saveProfile: (profile: ConnectionProfile) => Promise<void>;
}

const emptyProfile: ConnectionProfile = {
  caPath: "",
  defaultFleet: "",
  name: "",
  serverUrl: "",
  stateDir: "",
};

export function SetupMaintenancePage({
  bootstrapProfile,
  checkDesktopUpdate,
  connectProfile,
  loadSetup,
  onConnected,
  onCreateEnrollmentToken,
  openDocumentation,
  runDoctor,
  saveProfile,
}: SetupMaintenancePageProps) {
  const [setup, setSetup] = useState<SetupMaintenanceView>();
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");
  const [doctor, setDoctor] = useState<DesktopDoctorReport>();
  const [update, setUpdate] = useState<DesktopUpdateStatus>();
  const [editor, setEditor] = useState<"bootstrap" | "profile">();
  const [draft, setDraft] = useState<ConnectionProfile>(emptyProfile);
  const [bootstrapToken, setBootstrapToken] = useState("");

  useEffect(() => {
    let current = true;
    void loadSetup()
      .then((view) => current && setSetup(view))
      .catch((cause) => current && setError(actionMessage(cause)));
    return () => {
      current = false;
    };
  }, [loadSetup]);

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

  const openEditor = (kind: "bootstrap" | "profile") => {
    setDraft(emptyProfile);
    setBootstrapToken("");
    setEditor(kind);
    setError("");
  };

  const submitProfile = async (event: FormEvent) => {
    event.preventDefault();
    await run("save-profile", async () => {
      await saveProfile({ ...draft });
      setSetup((current) =>
        current
          ? {
              ...current,
              profiles: [
                ...current.profiles.filter((profile) => profile.name !== draft.name),
                { ...draft },
              ].toSorted((left, right) => left.name.localeCompare(right.name)),
            }
          : current,
      );
      setEditor(undefined);
    });
  };

  const submitBootstrap = async (event: FormEvent) => {
    event.preventDefault();
    const oneTimeToken = bootstrapToken;
    setBootstrapToken("");
    await run("bootstrap", async () => {
      const connection = await bootstrapProfile({ ...draft }, oneTimeToken);
      onConnected(connection);
      setEditor(undefined);
    });
  };

  if (!setup) {
    return (
      <section aria-label="Setup and support" className="setup-page">
        <span className="page-kicker">Local application</span>
        <h2>Setup &amp; support</h2>
        {error ? <p className="setup-error" role="alert">{error}</p> : <p>Loading protected connection settings…</p>}
      </section>
    );
  }

  return (
    <section aria-label="Setup and support" className="setup-page">
      <header className="setup-heading">
        <div>
          <span className="page-kicker">Local application</span>
          <h2>Setup &amp; support</h2>
          <p>Manage non-secret connection references, verify Operator access, and inspect this Linux desktop build.</p>
        </div>
        <Wrench aria-hidden="true" size={28} strokeWidth={1.6} />
      </header>
      {error ? <p className="setup-error" role="alert">{error}</p> : null}

      <div className="setup-summary">
        <article>
          <span>Application</span>
          <strong>{setup.application.name} {setup.application.version}</strong>
          <small data-mono>{setup.application.platform}/{setup.application.architecture}</small>
        </article>
        <article>
          <span>Standard Operator configuration</span>
          <strong data-mono>{setup.standardConfigPath}</strong>
        </article>
        <article>
          <span>Protected desktop profile settings</span>
          <strong data-mono>{setup.desktopProfilesPath}</strong>
        </article>
      </div>

      <section className="setup-panel" aria-labelledby="profiles-heading">
        <div className="setup-panel-heading">
          <div><Cable aria-hidden="true" size={18} /><h3 id="profiles-heading">Connection profiles</h3></div>
          <div>
            <button onClick={() => openEditor("profile")} type="button"><Plus aria-hidden="true" size={14} />Add profile</button>
            <button onClick={() => openEditor("bootstrap")} type="button"><KeyRound aria-hidden="true" size={14} />Bootstrap profile</button>
          </div>
        </div>
        {setup.profiles.length === 0 ? <p>No profiles are configured yet.</p> : (
          <div className="setup-profile-list">
            {setup.profiles.map((profile) => (
              <article key={profile.name}>
                <div><strong>{profile.name}</strong><span data-mono>{profile.serverUrl}</span></div>
                <dl><div><dt>State directory</dt><dd data-mono>{profile.stateDir}</dd></div><div><dt>Default Fleet</dt><dd>{profile.defaultFleet || "All Fleets"}</dd></div></dl>
                <div className="setup-profile-actions">
                  <button disabled={Boolean(pending)} onClick={() => void run(`connect-${profile.name}`, async () => onConnected(await connectProfile(profile)))} type="button">Connect {profile.name}</button>
                  <button disabled={Boolean(pending)} onClick={() => void run(`doctor-${profile.name}`, async () => setDoctor(await runDoctor(profile)))} type="button"><Stethoscope aria-hidden="true" size={14} />Run doctor for {profile.name}</button>
                </div>
              </article>
            ))}
          </div>
        )}
      </section>

      <div className="setup-support-grid">
        <section className="setup-panel">
          <div className="setup-panel-heading"><div><BookOpen aria-hidden="true" size={18} /><h3>Documentation</h3></div></div>
          <p>Open the published Remotr operator documentation in your system browser. Remote content never loads inside this app.</p>
          <button disabled={Boolean(pending)} onClick={() => void run("docs", openDocumentation)} type="button">Open Remotr documentation</button>
        </section>
        <section className="setup-panel">
          <div className="setup-panel-heading"><div><Download aria-hidden="true" size={18} /><h3>Desktop updates</h3></div></div>
          <p>Check the stable release channel. Installation is offered only by builds with matching native Linux artifact evidence.</p>
          <button disabled={Boolean(pending)} onClick={() => void run("update", async () => setUpdate(await checkDesktopUpdate()))} type="button">Check for desktop updates</button>
        </section>
        <section className="setup-panel">
          <div className="setup-panel-heading"><div><KeyRound aria-hidden="true" size={18} /><h3>Endpoint enrollment</h3></div></div>
          <p>Create a one-time token for an exact Fleet after connecting an authorized Operator profile.</p>
          <button disabled={!onCreateEnrollmentToken} onClick={onCreateEnrollmentToken} type="button">Create enrollment token</button>
        </section>
      </div>

      {doctor ? (
        <section aria-label="Doctor report" className="setup-result" role="status">
          <h3>{doctor.healthy ? "All setup checks passed" : "Setup needs attention"}</h3>
          <ul>{doctor.checks.map((check) => <li data-status={check.status} key={check.name}><CircleCheck aria-hidden="true" size={15} /><div><strong>{check.name}</strong><span>{check.detail}</span>{check.guidance ? <small>{check.guidance}</small> : null}</div></li>)}</ul>
        </section>
      ) : null}

      {update ? (
        <section aria-label="Desktop update status" className="setup-result" role="status">
          <h3>{update.updateAvailable ? `${update.latestVersion} is available` : `${update.currentVersion} is current`}</h3>
          <p>{update.platform} · {update.guidance}</p>
        </section>
      ) : null}

      {editor ? (
        <form aria-label={editor === "profile" ? "Add connection profile" : "Bootstrap first Operator"} className="setup-dialog" onSubmit={editor === "profile" ? submitProfile : submitBootstrap} role="dialog">
          <span className="page-kicker">{editor === "profile" ? "Non-secret references" : "One-time credential exchange"}</span>
          <h3>{editor === "profile" ? "Add connection profile" : "Bootstrap first Operator"}</h3>
          <label><span>Profile name</span><input aria-label="Profile name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label>
          <label><span>Server URL</span><input aria-label="Server URL" value={draft.serverUrl} onChange={(event) => setDraft({ ...draft, serverUrl: event.target.value })} /></label>
          <label><span>Operator state directory</span><input aria-label="Operator state directory" value={draft.stateDir} onChange={(event) => setDraft({ ...draft, stateDir: event.target.value })} /></label>
          <label><span>CA certificate path</span><input aria-label="CA certificate path" value={draft.caPath} onChange={(event) => setDraft({ ...draft, caPath: event.target.value })} /></label>
          <label><span>Default Fleet</span><input aria-label="Default Fleet" value={draft.defaultFleet} onChange={(event) => setDraft({ ...draft, defaultFleet: event.target.value })} /></label>
          {editor === "bootstrap" ? <label><span>One-time bootstrap token</span><input aria-label="One-time bootstrap token" autoComplete="off" type="password" value={bootstrapToken} onChange={(event) => setBootstrapToken(event.target.value)} /></label> : null}
          <div><button disabled={Boolean(pending)} onClick={() => { setBootstrapToken(""); setEditor(undefined); }} type="button">Cancel</button><button disabled={Boolean(pending)} type="submit">{editor === "profile" ? "Save profile" : "Bootstrap and connect"}</button></div>
        </form>
      ) : null}
    </section>
  );
}

function actionMessage(cause: unknown): string {
  return cause && typeof cause === "object" && "message" in cause && typeof cause.message === "string"
    ? cause.message
    : "The setup action could not be completed safely.";
}
