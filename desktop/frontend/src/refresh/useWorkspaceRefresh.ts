import { useCallback, useEffect, useRef, useState } from "react";

import type { OverviewWorkspace } from "../overview/Overview";

export interface RefreshClock {
  clearInterval: (id: number) => void;
  now: () => Date;
  setInterval: (callback: () => void, intervalMs: number) => number;
}

export interface WorkspaceVisibility {
  isVisible: () => boolean;
  subscribe: (listener: (visible: boolean) => void) => () => void;
}

export interface WorkspaceRefreshFailure {
  failedAt: string;
  guidance: string;
  kind: string;
  message: string;
}

interface WorkspaceRefreshOptions {
  clock?: RefreshClock;
  intervalMs?: number;
  loadWorkspace?: () => Promise<OverviewWorkspace>;
  visibility?: WorkspaceVisibility;
  workspace?: OverviewWorkspace;
}

const browserRefreshClock: RefreshClock = {
  clearInterval(id) {
    window.clearInterval(id);
  },
  now() {
    return new Date();
  },
  setInterval(callback, intervalMs) {
    return window.setInterval(callback, intervalMs);
  },
};

const browserWorkspaceVisibility: WorkspaceVisibility = {
  isVisible() {
    return document.visibilityState === "visible";
  },
  subscribe(listener) {
    const handleVisibilityChange = () =>
      listener(document.visibilityState === "visible");
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () =>
      document.removeEventListener("visibilitychange", handleVisibilityChange);
  },
};

function editingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) {
    return false;
  }
  return Boolean(
    target.closest(
      'input, textarea, select, [contenteditable=""], [contenteditable="true"], [role="textbox"]',
    ),
  );
}

function classifyRefreshFailure(
  error: unknown,
  failedAt: string,
): WorkspaceRefreshFailure {
  const classified =
    typeof error === "object" && error !== null
      ? (error as {
          guidance?: unknown;
          kind?: unknown;
          message?: unknown;
        })
      : undefined;
  return {
    failedAt,
    guidance:
      typeof classified?.guidance === "string"
        ? classified.guidance
        : "Check the active profile and server connection, then retry.",
    kind:
      typeof classified?.kind === "string"
        ? classified.kind
        : "unexpected",
    message:
      typeof classified?.message === "string"
        ? classified.message
        : "Workspace refresh could not be completed safely.",
  };
}

export function useWorkspaceRefresh({
  clock = browserRefreshClock,
  intervalMs = 30_000,
  loadWorkspace,
  visibility = browserWorkspaceVisibility,
  workspace: suppliedWorkspace,
}: WorkspaceRefreshOptions) {
  const [workspace, setWorkspace] = useState(suppliedWorkspace);
  const [failure, setFailure] = useState<WorkspaceRefreshFailure>();
  const [refreshing, setRefreshing] = useState(false);
  const inFlight = useRef(false);
  const mounted = useRef(true);

  useEffect(() => {
    setWorkspace(suppliedWorkspace);
    setFailure(undefined);
  }, [suppliedWorkspace]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const refresh = useCallback(() => {
    if (!loadWorkspace || inFlight.current || !mounted.current) {
      return;
    }

    inFlight.current = true;
    setRefreshing(true);
    void loadWorkspace()
      .then((nextWorkspace) => {
        if (!mounted.current) {
          return;
        }
        setWorkspace(nextWorkspace);
        setFailure(undefined);
      })
      .catch((error: unknown) => {
        if (!mounted.current) {
          return;
        }
        setFailure(classifyRefreshFailure(error, clock.now().toISOString()));
      })
      .finally(() => {
        inFlight.current = false;
        if (mounted.current) {
          setRefreshing(false);
        }
      });
  }, [clock, loadWorkspace]);

  const updateWorkspace = useCallback(
    (
      update: (
        current: OverviewWorkspace | undefined,
      ) => OverviewWorkspace | undefined,
    ) => {
      setWorkspace(update);
    },
    [],
  );

  useEffect(() => {
    if (!loadWorkspace) {
      return;
    }

    const intervalID = clock.setInterval(() => {
      if (visibility.isVisible()) {
        refresh();
      }
    }, intervalMs);
    const unsubscribe = visibility.subscribe((visible) => {
      if (visible) {
        refresh();
      }
    });
    return () => {
      clock.clearInterval(intervalID);
      unsubscribe();
    };
  }, [clock, intervalMs, loadWorkspace, refresh, visibility]);

  useEffect(() => {
    if (!loadWorkspace) {
      return;
    }

    const handleRefreshShortcut = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== "r") {
        return;
      }
      event.preventDefault();
      if (!editingTarget(event.target)) {
        refresh();
      }
    };
    window.addEventListener("keydown", handleRefreshShortcut);
    return () => window.removeEventListener("keydown", handleRefreshShortcut);
  }, [loadWorkspace, refresh]);

  return {
    failure,
    refresh,
    refreshing,
    updateWorkspace,
    workspace,
  };
}
