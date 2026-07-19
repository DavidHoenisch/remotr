import {
  AlertTriangle,
  ArrowUpDown,
  CheckCircle2,
  CircleSlash,
  Clock3,
  Columns3,
  ExternalLink,
  HelpCircle,
  Tags,
  Search,
  Upload,
  XCircle,
} from "lucide-react";
import {
  type ComponentType,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";

import { DataState } from "../states/DataState";
import "../styles/theme.css";
import {
  applyTableView,
  type TableSort,
  type TableSortDirection,
} from "../tables/tableView";
import { useTableSearchShortcut } from "../tables/useTableSearchShortcut";
import "./EndpointTable.css";

interface EndpointLabel {
  key: string;
  value: string;
}

export interface EndpointTableRow {
  activeDigest?: string;
  activeReleaseRef?: string;
  activeSchemaVersion?: number;
  capabilityBlockedTargetRef?: string;
  capabilityDigest?: string;
  capabilityReceivedAt?: string;
  compliance: string;
  desiredAgentVersion: string;
  endpointId: string;
  evidenceAt?: string;
  fleet: string;
  freshness: string;
  labels: EndpointLabel[];
  missingRequirements?: Array<{ id: string; revision?: string }>;
  offeredDigest?: string;
  offeredReleaseRef?: string;
  offeredSchemaVersion?: number;
  releaseRef: string;
  reportedAgentVersion: string;
  targetReleaseRef?: string;
  unmanaged?: boolean;
  usernames: string[];
}

interface EndpointTableProps {
  endpoints: EndpointTableRow[];
  initialFilters?: EndpointTableInitialFilters;
  labelColumns: string[];
  onCreateEnrollmentToken?: () => void;
  onManageLabels?: (endpoint: EndpointTableRow) => void;
  onOpenEndpoint?: (endpointId: string) => void;
  onRequestAgentUpgrade?: (endpoint: EndpointTableRow) => void;
}

export interface EndpointTableInitialFilters {
  compliance?: string[];
  fleet?: string[];
  freshness?: string[];
}

type EndpointFilterKey = "compliance" | "fleet" | "freshness";
type EndpointSortKey =
  | "compliance"
  | "desiredAgentVersion"
  | "endpointId"
  | "evidenceAt"
  | "fleet"
  | "freshness"
  | "releaseRef"
  | "reportedAgentVersion"
  | `label:${string}`;

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

const complianceOptions = Object.entries(compliancePresentation).map(
  ([value, presentation]) => ({ label: presentation.label, value }),
);
const freshnessOptions = Object.entries(freshnessPresentation).map(
  ([value, presentation]) => ({ label: presentation.label, value }),
);

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

function selectedFilter(values?: string[]): string {
  return values?.[0] ?? "all";
}

function SortableHeading({
  children,
  currentSort,
  label,
  onSort,
  sortKey,
}: {
  children: ReactNode;
  currentSort?: TableSort<EndpointSortKey>;
  label: string;
  onSort: (key: EndpointSortKey) => void;
  sortKey: EndpointSortKey;
}) {
  const direction: TableSortDirection | "none" =
    currentSort?.key === sortKey ? currentSort.direction : "none";
  return (
    <th aria-sort={direction} scope="col">
      <button
        aria-label={`Sort by ${label}`}
        className="endpoint-sort"
        onClick={() => onSort(sortKey)}
        type="button"
      >
        <span>{children}</span>
        <ArrowUpDown aria-hidden="true" size={12} strokeWidth={1.8} />
      </button>
    </th>
  );
}

export function EndpointTable({
  endpoints,
  initialFilters,
  labelColumns,
  onCreateEnrollmentToken,
  onManageLabels,
  onOpenEndpoint,
  onRequestAgentUpgrade,
}: EndpointTableProps) {
  const configuredLabelKeys = uniqueKeys(labelColumns);
  const configuredLabelKeySignature = configuredLabelKeys.join("\u0000");
  const [visibleLabelKeys, setVisibleLabelKeys] = useState(
    () => configuredLabelKeys,
  );
  const previousConfiguredLabelKeys = useRef(configuredLabelKeys);
  const [showColumns, setShowColumns] = useState(false);
  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState<Record<EndpointFilterKey, string>>({
    compliance: selectedFilter(initialFilters?.compliance),
    fleet: selectedFilter(initialFilters?.fleet),
    freshness: selectedFilter(initialFilters?.freshness),
  });
  const [sort, setSort] = useState<TableSort<EndpointSortKey>>();
  const searchInput = useRef<HTMLInputElement>(null);
  useTableSearchShortcut(searchInput);

  const initialCompliance = selectedFilter(initialFilters?.compliance);
  const initialFleet = selectedFilter(initialFilters?.fleet);
  const initialFreshness = selectedFilter(initialFilters?.freshness);
  useEffect(() => {
    setFilters({
      compliance: initialCompliance,
      fleet: initialFleet,
      freshness: initialFreshness,
    });
  }, [initialCompliance, initialFleet, initialFreshness]);

  useEffect(() => {
    const previous = previousConfiguredLabelKeys.current;
    setVisibleLabelKeys((current) =>
      configuredLabelKeys.filter(
        (key) => current.includes(key) || !previous.includes(key),
      ),
    );
    previousConfiguredLabelKeys.current = configuredLabelKeys;
  }, [configuredLabelKeySignature]);

  const fleetOptions = Array.from(
    new Set(endpoints.map((endpoint) => endpoint.fleet)),
  ).toSorted();
  const visibleEndpoints = applyTableView({
    filterValue: (endpoint, key) => endpoint[key],
    filters,
    identity: (endpoint) => endpoint.endpointId,
    query,
    rows: endpoints,
    searchValues: (endpoint) => [
      endpoint.endpointId,
      endpoint.fleet,
      ...endpoint.usernames,
      ...endpoint.labels.flatMap((label) => [label.key, label.value]),
    ],
    sort,
    sortValue: (endpoint, key) => {
      switch (key) {
        case "compliance":
        case "desiredAgentVersion":
        case "endpointId":
        case "fleet":
        case "freshness":
        case "releaseRef":
        case "reportedAgentVersion":
          return endpoint[key];
        case "evidenceAt":
          return endpoint.evidenceAt ?? "";
        default: {
          const labelKey = key.slice("label:".length);
          return (
            endpoint.labels.find((label) => label.key === labelKey)?.value ?? ""
          );
        }
      }
    },
  });
  const hasActiveFilters =
    query.trim().length > 0 ||
    Object.values(filters).some((value) => value !== "all");
  const selectorSummary = (
    Object.entries(filters) as Array<[EndpointFilterKey, string]>
  )
    .filter(([, value]) => value !== "all")
    .map(([name, value]) => `${name}: ${value}`)
    .join(" · ");

  const updateFilter = (key: EndpointFilterKey, value: string) => {
    setFilters((current) => ({ ...current, [key]: value }));
  };

  const changeSort = (key: EndpointSortKey) => {
    setSort((current) => ({
      direction:
        current?.key === key && current.direction === "ascending"
          ? "descending"
          : "ascending",
      key,
    }));
  };

  const clearFilters = () => {
    setQuery("");
    setFilters({ compliance: "all", fleet: "all", freshness: "all" });
  };

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
        <div className="endpoint-table-summary">
          <div>
            <span className="page-kicker">Inventory result</span>
            <strong aria-live="polite" data-numeric>
              {hasActiveFilters
                ? `${visibleEndpoints.length} of ${endpoints.length}`
                : endpoints.length}{" "}
              {endpoints.length === 1 ? "Endpoint" : "Endpoints"}
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
        </div>

        <div className="endpoint-table-filters">
          <label className="endpoint-search">
            <Search aria-hidden="true" size={15} strokeWidth={1.8} />
            <input
              aria-label="Search Endpoints"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search ID, Fleet, user, or Label"
              ref={searchInput}
              type="search"
              value={query}
            />
            <kbd>/</kbd>
          </label>
          <select
            aria-label="Fleet filter"
            onChange={(event) => updateFilter("fleet", event.target.value)}
            value={filters.fleet}
          >
            <option value="all">All Fleets</option>
            {fleetOptions.map((fleet) => (
              <option key={fleet} value={fleet}>
                {fleet}
              </option>
            ))}
          </select>
          <select
            aria-label="Compliance filter"
            onChange={(event) =>
              updateFilter("compliance", event.target.value)
            }
            value={filters.compliance}
          >
            <option value="all">All compliance</option>
            {complianceOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <select
            aria-label="Freshness filter"
            onChange={(event) =>
              updateFilter("freshness", event.target.value)
            }
            value={filters.freshness}
          >
            <option value="all">All freshness</option>
            {freshnessOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <button
            className="endpoint-clear-filters"
            disabled={!hasActiveFilters}
            onClick={clearFilters}
            type="button"
          >
            Clear all filters
          </button>
        </div>
        {selectorSummary ? (
          <p className="endpoint-applied-filters">
            Applied filters — {selectorSummary}
          </p>
        ) : null}
      </header>

      <div className="endpoint-table-scroll">
        <table aria-label="Endpoints">
          <thead>
            <tr>
              <SortableHeading
                currentSort={sort}
                label="Endpoint"
                onSort={changeSort}
                sortKey="endpointId"
              >
                Endpoint
              </SortableHeading>
              <SortableHeading
                currentSort={sort}
                label="Fleet"
                onSort={changeSort}
                sortKey="fleet"
              >
                Fleet
              </SortableHeading>
              <SortableHeading
                currentSort={sort}
                label="Compliance"
                onSort={changeSort}
                sortKey="compliance"
              >
                Compliance
              </SortableHeading>
              <SortableHeading
                currentSort={sort}
                label="Check-in freshness"
                onSort={changeSort}
                sortKey="freshness"
              >
                Check-in freshness
              </SortableHeading>
              <SortableHeading
                currentSort={sort}
                label="Reported agent"
                onSort={changeSort}
                sortKey="reportedAgentVersion"
              >
                Reported agent
              </SortableHeading>
              <SortableHeading
                currentSort={sort}
                label="Desired agent"
                onSort={changeSort}
                sortKey="desiredAgentVersion"
              >
                Desired agent
              </SortableHeading>
              <SortableHeading
                currentSort={sort}
                label="Active Release"
                onSort={changeSort}
                sortKey="releaseRef"
              >
                Active Release
              </SortableHeading>
              {visibleLabelKeys.map((key) => (
                <SortableHeading
                  currentSort={sort}
                  key={key}
                  label={labelHeading(key)}
                  onSort={changeSort}
                  sortKey={`label:${key}`}
                >
                  {labelHeading(key)}
                </SortableHeading>
              ))}
              <SortableHeading
                currentSort={sort}
                label="Last evidence"
                onSort={changeSort}
                sortKey="evidenceAt"
              >
                Last evidence
              </SortableHeading>
              <th className="endpoint-actions-heading" scope="col">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {visibleEndpoints.map((endpoint) => {
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
                  <td data-mono>
                    {endpoint.activeReleaseRef || endpoint.releaseRef || "Not reported"}
                  </td>
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
                    {onRequestAgentUpgrade ? (
                      <button
                        aria-label={`Request agent upgrade for ${endpoint.endpointId}`}
                        onClick={() => onRequestAgentUpgrade(endpoint)}
                        type="button"
                      >
                        <Upload
                          aria-hidden="true"
                          size={15}
                          strokeWidth={1.8}
                        />
                      </button>
                    ) : null}
                    {onManageLabels ? (
                      <button
                        aria-label={`Manage Labels for ${endpoint.endpointId}`}
                        onClick={() => onManageLabels(endpoint)}
                        type="button"
                      >
                        <Tags
                          aria-hidden="true"
                          size={15}
                          strokeWidth={1.8}
                        />
                      </button>
                    ) : null}
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
      {visibleEndpoints.length === 0 ? (
        <p className="endpoint-no-results" role="status">
          No Endpoints match the current search and filters.
        </p>
      ) : null}
    </section>
  );
}
