import { useCallback, useRef, useState } from "react";

export type ActionErrorKind =
  | "authorization"
  | "conflict"
  | "connection"
  | "not_found"
  | "rate_limited"
  | "unexpected"
  | "validation";

export interface ActionErrorEnvelope {
  fieldErrors?: Record<string, string>;
  guidance: string;
  kind: ActionErrorKind;
  message: string;
  retryable: boolean;
}

export interface ActionAcknowledgement {
  acceptedAt: string;
  action: string;
  affectedEvidence: string[];
  requestId?: string;
  summary: string;
  target: string;
}

interface ActionControllerOptions<
  Input,
  Result extends ActionAcknowledgement,
  SafeContext,
> {
  execute: (input: Input) => Promise<Result>;
  refreshAffected: (result: Result) => Promise<void> | void;
  safeContext: (input: Input) => SafeContext;
}

interface ActionControllerState<Result, SafeContext> {
  error?: ActionErrorEnvelope;
  pending: boolean;
  refreshError?: ActionErrorEnvelope;
  reset: () => void;
  result?: Result;
  safeContext?: SafeContext;
}

const actionErrorKinds = new Set<ActionErrorKind>([
  "authorization",
  "conflict",
  "connection",
  "not_found",
  "rate_limited",
  "unexpected",
  "validation",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function safeString(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() ? value : fallback;
}

function safeFieldErrors(value: unknown): Record<string, string> | undefined {
  if (!isRecord(value)) {
    return undefined;
  }

  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] =>
      entry[0].length > 0 &&
      typeof entry[1] === "string" &&
      entry[1].trim().length > 0,
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

export function normalizeActionError(error: unknown): ActionErrorEnvelope {
  if (!isRecord(error)) {
    return {
      guidance: "Retry the action or cancel and review the current evidence.",
      kind: "unexpected",
      message: "The action could not be completed safely.",
      retryable: true,
    };
  }

  const kind =
    typeof error.kind === "string" &&
    actionErrorKinds.has(error.kind as ActionErrorKind)
      ? (error.kind as ActionErrorKind)
      : "unexpected";
  const fieldErrors = safeFieldErrors(error.fieldErrors);
  return {
    ...(fieldErrors ? { fieldErrors } : {}),
    guidance: safeString(
      error.guidance,
      "Retry the action or cancel and review the current evidence.",
    ),
    kind,
    message: safeString(
      error.message,
      "The action could not be completed safely.",
    ),
    retryable:
      typeof error.retryable === "boolean" ? error.retryable : kind !== "validation",
  };
}

export function useActionController<
  Input,
  Result extends ActionAcknowledgement,
  SafeContext,
>({
  execute,
  refreshAffected,
  safeContext,
}: ActionControllerOptions<
  Input,
  Result,
  SafeContext
>): ActionControllerState<Result, SafeContext> & {
  submit: (input: Input) => Promise<boolean>;
} {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<ActionErrorEnvelope>();
  const [refreshError, setRefreshError] = useState<ActionErrorEnvelope>();
  const [result, setResult] = useState<Result>();
  const [context, setContext] = useState<SafeContext>();
  const pendingRef = useRef(false);
  const generation = useRef(0);

  const reset = useCallback(() => {
    if (pendingRef.current) {
      return;
    }
    generation.current += 1;
    setPending(false);
    setError(undefined);
    setRefreshError(undefined);
    setResult(undefined);
    setContext(undefined);
  }, []);

  const submit = useCallback(
    async (input: Input): Promise<boolean> => {
      if (pendingRef.current) {
        return false;
      }

      pendingRef.current = true;
      const currentGeneration = ++generation.current;
      setPending(true);
      setError(undefined);
      setRefreshError(undefined);
      setResult(undefined);
      setContext(safeContext(input));

      try {
        const acknowledgement = await execute(input);
        if (currentGeneration !== generation.current) {
          return false;
        }

        setResult(acknowledgement);
        try {
          await refreshAffected(acknowledgement);
        } catch (refreshFailure: unknown) {
          if (currentGeneration === generation.current) {
            setRefreshError(normalizeActionError(refreshFailure));
          }
        }
        return true;
      } catch (failure: unknown) {
        if (currentGeneration === generation.current) {
          setError(normalizeActionError(failure));
        }
        return false;
      } finally {
        if (currentGeneration === generation.current) {
          pendingRef.current = false;
          setPending(false);
        }
      }
    },
    [execute, refreshAffected, safeContext],
  );

  return {
    error,
    pending,
    refreshError,
    reset,
    result,
    safeContext: context,
    submit,
  };
}
