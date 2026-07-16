import { AlertTriangle, RefreshCw } from "lucide-react";

import type { WorkspaceRefreshFailure } from "./useWorkspaceRefresh";
import "./WorkspaceFreshness.css";

export function WorkspaceFreshness({
  failure,
  loadedAt,
  onRefresh,
}: {
  failure: WorkspaceRefreshFailure;
  loadedAt: string;
  onRefresh: () => void;
}) {
  return (
    <section aria-label="Workspace freshness" className="workspace-freshness">
      <AlertTriangle aria-hidden="true" size={17} strokeWidth={1.8} />
      <div>
        <strong>Stale workspace evidence</strong>
        <span data-mono>Data loaded {loadedAt || "Not reported"}</span>
        <span data-mono>Refresh failed {failure.failedAt}</span>
        <p>{failure.message}</p>
        <p>{failure.guidance}</p>
      </div>
      <button onClick={onRefresh} type="button">
        <RefreshCw aria-hidden="true" size={14} strokeWidth={1.8} />
        Retry refresh
      </button>
    </section>
  );
}
