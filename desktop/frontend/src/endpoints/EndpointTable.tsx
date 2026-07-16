import {
  AlertTriangle,
  CheckCircle2,
  CircleSlash,
  Clock3,
  Columns3,
  ExternalLink,
  HelpCircle,
  XCircle,
} from "lucide-react";
import { type ComponentType, useState } from "react";

import { DataState } from "../states/DataState";
import "../styles/theme.css";
import "./EndpointTable.css";

interface EndpointLabel {
  key: string;
  value: string;
}

export interface EndpointTableRow {
  compliance: string;
  desiredAgentVersion: string;
  endpointId: string;
  evidenceAt?: string;
  fleet: string;
  freshness: string;
  labels: EndpointLabel[];
  releaseRef: string;
  reportedAgentVersion: string;
  usernames: string[];
}

interface EndpointTableProps {
  endpoints: EndpointTableRow[];
  labelColumns: string[];
  onCreateEnrollmentToken?: () => void;
  onOpenEndpoint?: (endpointId: string) => void;
}

interface StatusPresentation {
  icon: ComponentType<{
    "aria-hidden"?: boolean | "false" | "true";
    size?: number;
    strokeWidth?: number;
  }>;
  label: string;
  tone: string;
}

const compliancePresentation: Record<string, StatusPresentation> = {
  compliant: {
    icon: CheckCircle2,
    label: "Compliant",
    tone: "compliant",
  },
  drifted: {
    icon: AlertTriangle,
    label: "Drifted",
    tone: "drifted",
  },
  unsupported: {
    icon: HelpCircle,
    label: "Unsupported",
    tone: "neutral",
  },
  check_failed: {
    icon: XCircle,
    label: "Check failed",
    tone: "error",
  },
  deferred: {
    icon: Clock3,
    label: "Deferred",
    tone: "info",
  },
  apply_failed: {
    icon: XCircle,
    label: "Apply failed",
    tone: "error",
  },
  not_reported: {
    icon: CircleSlash,
    label: "Not reported",
    tone: "neutral",
  },
};

const freshnessPresentation: Record<string, StatusPresentation> = {
  recent: {
    icon: CheckCircle2,
    label: "Recent",
    tone: "compliant",
  },
  stale: {
    icon: Clock3,
    label: "Stale",
    tone: "drifted",
  },
  never_reported: {
    icon: CircleSlash,
    label: "Never reported",
    tone: "neutral",
  },
};

const unknownStatus: StatusPresentation = {
  icon: HelpCircle,
  label: "Unknown",
  tone: "neutral",
};

function uniqueKeys(keys: string[]): string[] {
  return keys.filter(
    (key, index) => key.length > 0 && keys.indexOf(key) === index,
  );
}

function labelHeading(key: string): string {
  const words = key.replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function valueOrNotReported(value: string): string {
  return value || "Not reported";
}

function formatEvidenceTime(value?: string): string {
  if (!value) {
    return "Never";
  }
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return value;
  }
  const year = timestamp.getUTCFullYear();
  const month = String(timestamp.getUTCMonth() + 1).padStart(2, "0");
  const day = String(timestamp.getUTCDate()).padStart(2, "0");
  const hour = String(timestamp.getUTCHours()).padStart(2, "0");
  const minute = String(timestamp.getUTCMinutes()).padStart(2, "0");
  return `${year}-${month}-${day} ${hour}:${minute} UTC`;
}

function StatusToken({ presentation }: { presentation: StatusPresentation }) {
  const Icon = presentation.icon;
  return (
    <span className="endpoint-status" data-tone={presentation.tone}>
      <Icon aria-hidden="true" size={14} strokeWidth={1.9} />
      <span>{presentation.label}</span>
    </span>
  );
}

export function EndpointTable({
  endpoints,
  labelColumns,
  onCreateEnrollmentToken,
  onOpenEndpoint,
}: EndpointTableProps) {
  const configuredLabelKeys = uniqueKeys(labelColumns);
  const [visibleLabelKeys, setVisibleLabelKeys] = useState(
    () => configuredLabelKeys,
  );
  const [showColumns, setShowColumns] = useState(false);

  const toggleLabelColumn = (key: string) => {
    setVisibleLabelKeys((current) =>
      current.includes(key)
        ? current.filter((candidate) => candidate !== key)
        : configuredLabelKeys.filter(
            (candidate) => current.includes(candidate) || candidate === key,
          ),
    );
  };

  if (endpoints.length === 0) {
    return (
      <div className="endpoint-table-empty">
        <DataState
          action={
            onCreateEnrollmentToken
              ? {
                  label: "Create enrollment token",
                  onAction: onCreateEnrollmentToken,
                }
              : undefined
          }
          kind="empty"
          message="Connected successfully. No Endpoints are enrolled in this Fleet scope."
          title="No Endpoints enrolled"
        />
      </div>
    );
  }

  return (
    <section className="endpoint-inventory" aria-label="Endpoint inventory">
      <header className="endpoint-table-toolbar">
        <div>
          <span className="page-kicker">Inventory result</span>
          <strong aria-live="polite" data-numeric>
            {endpoints.length} {endpoints.length === 1 ? "Endpoint" : "Endpoints"}
          </strong>
        </div>
        {configuredLabelKeys.length > 0 ? (
          <div className="column-chooser">
            <button
              aria-expanded={showColumns}
              className="column-chooser-trigger"
              onClick={() => setShowColumns((current) => !current)}
              type="button"
            >
              <Columns3 aria-hidden="true" size={15} strokeWidth={1.8} />
              Choose columns
            </button>
            {showColumns ? (
              <fieldset className="column-chooser-menu">
                <legend>Label columns</legend>
                {configuredLabelKeys.map((key) => (
                  <label key={key}>
                    <input
                      checked={visibleLabelKeys.includes(key)}
                      onChange={() => toggleLabelColumn(key)}
                      type="checkbox"
                    />
                    <span>{labelHeading(key)}</span>
                  </label>
                ))}
              </fieldset>
            ) : null}
          </div>
        ) : null}
      </header>

      <div className="endpoint-table-scroll">
        <table aria-label="Endpoints">
          <thead>
            <tr>
              <th scope="col">Endpoint</th>
              <th scope="col">Fleet</th>
              <th scope="col">Compliance</th>
              <th scope="col">Check-in freshness</th>
              <th scope="col">Reported agent</th>
              <th scope="col">Desired agent</th>
              <th scope="col">Release ref</th>
              {visibleLabelKeys.map((key) => (
                <th key={key} scope="col">
                  {labelHeading(key)}
                </th>
              ))}
              <th scope="col">Last evidence</th>
              <th className="endpoint-actions-heading" scope="col">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {endpoints.map((endpoint) => {
              const labels = new Map(
                endpoint.labels.map((label) => [label.key, label.value]),
              );
              return (
                <tr data-endpoint-id={endpoint.endpointId} key={endpoint.endpointId}>
                  <td className="endpoint-identity" data-mono>
                    {endpoint.endpointId}
                  </td>
                  <td data-mono>{endpoint.fleet}</td>
                  <td>
                    <StatusToken
                      presentation={
                        compliancePresentation[endpoint.compliance] ?? unknownStatus
                      }
                    />
                  </td>
                  <td>
                    <StatusToken
                      presentation={
                        freshnessPresentation[endpoint.freshness] ?? unknownStatus
                      }
                    />
                  </td>
                  <td data-mono>
                    {valueOrNotReported(endpoint.reportedAgentVersion)}
                  </td>
                  <td data-mono>
                    {valueOrNotReported(endpoint.desiredAgentVersion)}
                  </td>
                  <td data-mono>{endpoint.releaseRef || "Not reported"}</td>
                  {visibleLabelKeys.map((key) => (
                    <td data-mono key={key}>
                      {labels.get(key) ?? "—"}
                    </td>
                  ))}
                  <td>
                    <time dateTime={endpoint.evidenceAt} data-mono>
                      {formatEvidenceTime(endpoint.evidenceAt)}
                    </time>
                  </td>
                  <td className="endpoint-row-actions">
                    {onOpenEndpoint ? (
                      <button
                        aria-label={`Inspect ${endpoint.endpointId}`}
                        onClick={() => onOpenEndpoint(endpoint.endpointId)}
                        type="button"
                      >
                        <ExternalLink
                          aria-hidden="true"
                          size={15}
                          strokeWidth={1.8}
                        />
                      </button>
                    ) : null}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
