import { GitPullRequest } from "lucide-react";

import "./GitDesiredStateBoundary.css";

export function GitDesiredStateBoundary() {
  return (
    <aside
      aria-label="Git desired-state boundary"
      className="git-desired-state-boundary"
      role="note"
    >
      <GitPullRequest aria-hidden="true" size={18} strokeWidth={1.8} />
      <div>
        <strong>Desired state stays in Git</strong>
        <p>
          Git review is required before server sync can advance a Release ref.
          Remotr Desktop does not edit, stage, commit, push, merge, or directly
          apply Configuration content.
        </p>
      </div>
    </aside>
  );
}
