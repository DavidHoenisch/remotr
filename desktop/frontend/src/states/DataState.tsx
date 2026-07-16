import {
  CircleEllipsis,
  Clock3,
  Inbox,
  Loader2,
  RefreshCw,
  ShieldAlert,
  TriangleAlert,
  WifiOff,
} from "lucide-react";
import { type ReactNode, useId } from "react";

import "../styles/theme.css";
import "./DataState.css";

export type DataStateKind =
  | "loading"
  | "empty"
  | "partial"
  | "stale"
  | "authorization"
  | "connection"
  | "unexpected";

interface DataStateAction {
  label: string;
  onAction: () => void;
}

interface DataStateProps {
  action?: DataStateAction;
  children?: ReactNode;
  kind: DataStateKind;
  message: string;
  title: string;
}

function StateIcon({ kind }: { kind: DataStateKind }) {
  const iconProps = {
    "aria-hidden": true as const,
    size: 20,
    strokeWidth: 1.8,
  };

  switch (kind) {
    case "loading":
      return <Loader2 className="data-state-spinner" {...iconProps} />;
    case "empty":
      return <Inbox {...iconProps} />;
    case "partial":
      return <CircleEllipsis {...iconProps} />;
    case "stale":
      return <Clock3 {...iconProps} />;
    case "authorization":
      return <ShieldAlert {...iconProps} />;
    case "connection":
      return <WifiOff {...iconProps} />;
    case "unexpected":
      return <TriangleAlert {...iconProps} />;
  }
}

export function DataState({
  action,
  children,
  kind,
  message,
  title,
}: DataStateProps) {
  const titleId = useId();
  const isFailure = kind === "connection" || kind === "unexpected";
  const retainEvidence = kind === "partial" || kind === "stale";

  return (
    <section
      aria-busy={kind === "loading" ? true : undefined}
      aria-labelledby={titleId}
      className="data-state-surface"
      data-kind={kind}
      role={isFailure ? "alert" : "status"}
    >
      <span className="data-state-icon" aria-hidden="true">
        <StateIcon kind={kind} />
      </span>
      <div className="data-state-copy">
        <strong id={titleId}>{title}</strong>
        <p>{message}</p>
        {action ? (
          <button onClick={action.onAction} type="button">
            <RefreshCw aria-hidden="true" size={14} strokeWidth={1.8} />
            {action.label}
          </button>
        ) : null}
      </div>
      {retainEvidence && children ? (
        <div className="data-state-retained">{children}</div>
      ) : null}
    </section>
  );
}
