import { useCallback, useEffect, useRef, useState } from "react";

import type {
  RefreshClock,
  WorkspaceVisibility,
} from "../refresh/useWorkspaceRefresh";
import type { ChangeRequestDetailView } from "./ChangeRequestDetail";

const browserClock: RefreshClock = {
  clearInterval: (id) => window.clearInterval(id),
  now: () => new Date(),
  setInterval: (callback, intervalMs) => window.setInterval(callback, intervalMs),
};

const browserVisibility: WorkspaceVisibility = {
  isVisible: () => document.visibilityState === "visible",
  subscribe(listener) {
    const handle = () => listener(document.visibilityState === "visible");
    document.addEventListener("visibilitychange", handle);
    return () => document.removeEventListener("visibilitychange", handle);
  },
};

const browserWatchRandom = () => Math.random();

interface WatchFailure {
  failedAt: string;
  guidance: string;
  message: string;
}

function watchFailure(error: unknown, failedAt: string): WatchFailure {
  const value = typeof error === "object" && error !== null
    ? (error as { guidance?: unknown; message?: unknown })
    : undefined;
  return {
    failedAt,
    guidance: typeof value?.guidance === "string"
      ? value.guidance
      : "Keep the current evidence visible and check the active connection.",
    message: typeof value?.message === "string"
      ? value.message
      : "Change request watch could not refresh safely.",
  };
}

export function useChangeRequestWatch({
  changeRequestId,
  clock = browserClock,
  intervalMs = 2_000,
  loadChangeRequestDetail,
  onUpdate,
  random = browserWatchRandom,
  timeoutMs = 60_000,
  visibility = browserVisibility,
}: {
  changeRequestId: string;
  clock?: RefreshClock;
  intervalMs?: number;
  loadChangeRequestDetail: (changeRequestId: string) => Promise<ChangeRequestDetailView>;
  onUpdate: (detail: ChangeRequestDetailView) => void;
  random?: () => number;
  timeoutMs?: number;
  visibility?: WorkspaceVisibility;
}) {
  const [active, setActive] = useState(false);
  const [failure, setFailure] = useState<WatchFailure>();
  const [lastObservedAt, setLastObservedAt] = useState("");
  const [timedOut, setTimedOut] = useState(false);
  const [cadenceMs, setCadenceMs] = useState(intervalMs);
  const activeRef = useRef(false);
  const deadline = useRef(0);
  const inFlight = useRef(false);
  const mounted = useRef(true);

  const stop = useCallback((timeout = false) => {
    activeRef.current = false;
    setActive(false);
    setTimedOut(timeout);
  }, []);

  const refresh = useCallback(() => {
    if (!activeRef.current || !mounted.current || !visibility.isVisible()) {
      return;
    }
    if (clock.now().getTime() >= deadline.current) {
      stop(true);
      return;
    }
    if (inFlight.current) {
      return;
    }
    inFlight.current = true;
    void loadChangeRequestDetail(changeRequestId)
      .then((next) => {
        if (!mounted.current || !activeRef.current) return;
        if (next.summary.changeRequestId !== changeRequestId) {
          throw new Error("Watched evidence did not match the selected Change request.");
        }
        onUpdate(next);
        setFailure(undefined);
        setLastObservedAt(clock.now().toISOString());
      })
      .catch((error: unknown) => {
        if (mounted.current && activeRef.current) {
          setFailure(watchFailure(error, clock.now().toISOString()));
        }
      })
      .finally(() => {
        inFlight.current = false;
      });
  }, [changeRequestId, clock, loadChangeRequestDetail, onUpdate, stop, visibility]);

  const start = useCallback(() => {
    if (activeRef.current) return;
    const sample = random();
    const boundedSample = Number.isFinite(sample)
      ? Math.min(1, Math.max(0, sample))
      : 0.5;
    setCadenceMs(Math.max(1, Math.round(intervalMs * (0.9 + boundedSample * 0.2))));
    deadline.current = clock.now().getTime() + timeoutMs;
    activeRef.current = true;
    setTimedOut(false);
    setFailure(undefined);
    setActive(true);
    refresh();
  }, [clock, intervalMs, random, refresh, timeoutMs]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      activeRef.current = false;
    };
  }, []);

  useEffect(() => {
    stop(false);
    setFailure(undefined);
    setLastObservedAt("");
  }, [changeRequestId, stop]);

  useEffect(() => {
    if (!active) return;
    const intervalID = clock.setInterval(refresh, cadenceMs);
    const deadlineID = clock.setInterval(() => {
      if (activeRef.current && clock.now().getTime() >= deadline.current) {
        stop(true);
      }
    }, timeoutMs);
    return () => {
      clock.clearInterval(intervalID);
      clock.clearInterval(deadlineID);
    };
  }, [active, cadenceMs, clock, refresh, stop, timeoutMs]);

  useEffect(() => visibility.subscribe((visible) => {
    if (visible && activeRef.current) refresh();
  }), [refresh, visibility]);

  return { active, failure, lastObservedAt, start, stop: () => stop(false), timedOut };
}
