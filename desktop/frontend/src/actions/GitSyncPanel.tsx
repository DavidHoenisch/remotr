import {
  CheckCircle2,
  GitPullRequest,
  RefreshCw,
  TriangleAlert,
} from "lucide-react";
import { useEffect } from "react";

import {
  type ActionAcknowledgement,
  useActionController,
} from "./useActionController";
import "./GitSyncPanel.css";

interface GitSyncContext {
  evidenceObservedAt: string[];
  profileName: string;
  releaseRefs: string;
  serverLabel: string;
}

function GitSyncContextView({ context }: { context: GitSyncContext }) {
  return (
    <dl className="git-sync-context">
      <div>
        <dt>Profile</dt>
        <dd>{context.profileName}</dd>
      </div>
      <div>
        <dt>Server</dt>
        <dd data-mono>{context.serverLabel}</dd>
      </div>
      <div>
        <dt>Observed Release refs</dt>
        <dd data-mono>{context.releaseRefs}</dd>
      </div>
      <div>
        <dt>Release evidence observed</dt>
        <dd>
          <ul className="git-sync-evidence-times">
            {context.evidenceObservedAt.map((observedAt) => (
              <li data-mono key={observedAt}>
                <time dateTime={observedAt}>{observedAt}</time>
              </li>
            ))}
          </ul>
        </dd>
      </div>
    </dl>
  );
}

export function GitSyncPanel({
  onCancel,
  onPendingChange,
  evidenceObservedAt,
  profileName,
  refreshAffected,
  releaseRefs,
  requestGitSync,
  serverLabel,
}: {
  onCancel: () => void;
  onPendingChange: (pending: boolean) => void;
  evidenceObservedAt: string[];
  profileName: string;
  refreshAffected: (result: ActionAcknowledgement) => Promise<void> | void;
  releaseRefs: string[];
  requestGitSync: () => Promise<ActionAcknowledgement>;
  serverLabel: string;
}) {
  const context: GitSyncContext = {
    evidenceObservedAt:
      evidenceObservedAt.length > 0 ? [...evidenceObservedAt] : ["Not reported"],
    profileName,
    releaseRefs: releaseRefs.join(", ") || "Not reported",
    serverLabel,
  };
  const action = useActionController({
    execute: requestGitSync,
    refreshAffected,
    safeContext: () => context,
  });

  useEffect(() => {
    onPendingChange(action.pending);
    return () => onPendingChange(false);
  }, [action.pending, onPendingChange]);

  const cancel = () => {
    if (action.pending) {
      return;
    }
    action.reset();
    onCancel();
  };

  if (action.result) {
    return (
      <section
        aria-label="Git sync accepted"
        className="git-sync-result"
        role="status"
      >
        <CheckCircle2 aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Server acknowledgement</span>
          <strong>{action.result.summary}</strong>
          {action.result.requestId ? (
            <p data-mono>{`Request ${action.result.requestId}`}</p>
          ) : null}
          <p data-mono>{`Accepted ${action.result.acceptedAt}`}</p>
          <p>
            Awaiting refreshed server evidence. Endpoint convergence is reported
            separately by later State evidence.
          </p>
          <button onClick={cancel} type="button">
            Close
          </button>
        </div>
      </section>
    );
  }

  if (action.error) {
    const safeContext = action.safeContext ?? context;
    return (
      <section
        aria-label="Git sync failed"
        className="git-sync-failure"
        role="alert"
      >
        <TriangleAlert aria-hidden="true" size={22} strokeWidth={1.8} />
        <div>
          <span className="page-kicker">Request not accepted</span>
          <strong>{action.error.message}</strong>
          <p>{action.error.guidance}</p>
          <GitSyncContextView context={safeContext} />
          <div className="git-sync-actions">
            {action.error.retryable ? (
              <button
                onClick={() => void action.submit(undefined)}
                type="button"
              >
                <RefreshCw aria-hidden="true" size={14} strokeWidth={1.8} />
                Retry Git sync
              </button>
            ) : null}
            <button onClick={cancel} type="button">
              Cancel
            </button>
          </div>
        </div>
      </section>
    );
  }

  return (
    <section
      aria-label="Git sync confirmation"
      className="git-sync-confirmation"
    >
      <div className="git-sync-intro">
        <GitPullRequest aria-hidden="true" size={20} strokeWidth={1.8} />
        <div>
          <strong>Request server Git sync</strong>
          <p>
            The connected Remotr server will fetch its configured repository.
            This desktop app will not edit or commit local Configuration files.
          </p>
        </div>
      </div>
      <GitSyncContextView context={context} />
      <p className="git-sync-note">
        Acceptance means the server started the operation. Release ref changes
        appear only after refreshed server evidence.
      </p>
      <footer className="git-sync-actions">
        <button disabled={action.pending} onClick={cancel} type="button">
          Cancel
        </button>
        <button
          className="git-sync-submit"
          disabled={action.pending}
          onClick={() => void action.submit(undefined)}
          type="button"
        >
          <GitPullRequest aria-hidden="true" size={14} strokeWidth={1.8} />
          {action.pending ? "Requesting Git sync" : "Request Git sync"}
        </button>
      </footer>
    </section>
  );
}
