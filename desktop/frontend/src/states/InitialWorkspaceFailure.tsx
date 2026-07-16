import { RefreshCw, Server, WifiOff } from "lucide-react";

import "./InitialWorkspaceFailure.css";

export interface InitialWorkspaceFailureView {
  guidance: string;
  kind: string;
  message: string;
}

export function InitialWorkspaceFailure({
  authenticated = false,
  failure,
  onChooseProfile,
  onRetry,
  profileName,
  serverLabel,
}: {
  authenticated?: boolean;
  failure: InitialWorkspaceFailureView;
  onChooseProfile?: () => void;
  onRetry?: () => void;
  profileName: string;
  serverLabel: string;
}) {
  return (
    <section
      aria-label={authenticated ? "Workspace recovery" : "Connection recovery"}
      className="initial-workspace-failure"
      data-kind={failure.kind}
    >
      <div className="initial-failure-icon" aria-hidden="true">
        <WifiOff size={22} strokeWidth={1.8} />
      </div>
      <div className="initial-failure-copy">
        <span className="page-kicker">
          {authenticated
            ? "Authenticated workspace unavailable"
            : "Selected profile unavailable"}
        </span>
        <h2>
          {authenticated
            ? "Workspace load failed"
            : `${profileName} connection failed`}
        </h2>
        <p>{failure.message}</p>
        <p className="initial-failure-guidance">{failure.guidance}</p>
        <dl>
          <div>
            <dt>Profile</dt>
            <dd>{profileName}</dd>
          </div>
          <div>
            <dt>Server</dt>
            <dd>{serverLabel}</dd>
          </div>
        </dl>
        <div className="initial-failure-actions">
          {onRetry ? (
            <button onClick={onRetry} type="button">
              <RefreshCw aria-hidden="true" size={14} strokeWidth={1.8} />
              {authenticated ? "Retry workspace" : "Retry connection"}
            </button>
          ) : null}
          {onChooseProfile ? (
            <button onClick={onChooseProfile} type="button">
              <Server aria-hidden="true" size={14} strokeWidth={1.8} />
              Choose another profile
            </button>
          ) : null}
        </div>
      </div>
    </section>
  );
}
