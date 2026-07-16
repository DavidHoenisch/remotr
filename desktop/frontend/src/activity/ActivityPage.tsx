import { ArrowDown, ArrowRight, Filter, ScrollText } from "lucide-react";
import { type FormEvent, useRef, useState } from "react";

import { DataState, type DataStateKind } from "../states/DataState";
import "./ActivityPage.css";

export interface ActivityDetailView {
  key: string;
  value: string;
}

export interface ActivityEventView {
  action: string;
  actor: string;
  details: ActivityDetailView[];
  eventId: string;
  occurredAt: string;
  requestId: string;
  resourceId: string;
  resourceType: string;
  status: string;
}

interface ActivitySectionError {
  guidance: string;
  kind: string;
  message: string;
}

export interface ActivitySectionView {
  error?: ActivitySectionError;
  snapshot: {
    failedAt?: string;
    loadedAt: string;
    observedAt?: string;
  };
  state: string;
}

export interface ActivityPageRequest {
  action: string;
  actorType: string;
  cursor: string;
  seenEventIds: string[];
  since: string;
  until: string;
}

export interface ActivityPageView {
  events: ActivityEventView[];
  nextCursor: string;
  section: ActivitySectionView;
}

interface ActivityPageProps {
  initialEvents: ActivityEventView[];
  initialNextCursor?: string;
  initialSection: ActivitySectionView;
  loadPage?: (request: ActivityPageRequest) => Promise<ActivityPageView>;
  onInspect?: (event: ActivityEventView) => void;
}

type ActivityFilters = Omit<ActivityPageRequest, "cursor" | "seenEventIds">;

const emptyFilters: ActivityFilters = {
  action: "",
  actorType: "",
  since: "",
  until: "",
};

const actionLabels: Record<string, string> = {
  endpoint_label_added: "Endpoint label added",
  endpoint_label_removed: "Endpoint label removed",
  endpoint_label_replaced: "Endpoint label replaced",
};

function displayAction(action: string): string {
  return actionLabels[action] ?? action;
}

function uniqueEvents(
  current: ActivityEventView[],
  incoming: ActivityEventView[],
): ActivityEventView[] {
  const seen = new Set(current.map((event) => event.eventId));
  const merged = [...current];
  for (const event of incoming) {
    if (!event.eventId || seen.has(event.eventId)) {
      continue;
    }
    seen.add(event.eventId);
    merged.push(event);
  }
  return merged;
}

function sectionKind(section: ActivitySectionView): DataStateKind {
  if (section.state === "loading") {
    return "loading";
  }
  if (section.state === "empty") {
    return "empty";
  }
  if (section.state === "partial") {
    return "partial";
  }
  if (section.state === "stale") {
    return "stale";
  }
  if (section.error?.kind === "authorization") {
    return "authorization";
  }
  if (section.error?.kind === "connection") {
    return "connection";
  }
  return "unexpected";
}

function failureTitle(section: ActivitySectionView): string {
  if (section.error?.kind === "authorization") {
    return "Activity unavailable";
  }
  if (section.error?.kind === "connection") {
    return "Activity connection failed";
  }
  return "Activity could not be loaded";
}

export function ActivityPage({
  initialEvents,
  initialNextCursor = "",
  initialSection,
  loadPage,
  onInspect,
}: ActivityPageProps) {
  const [events, setEvents] = useState(() => uniqueEvents([], initialEvents));
  const [nextCursor, setNextCursor] = useState(initialNextCursor);
  const [section, setSection] = useState(initialSection);
  const [draftFilters, setDraftFilters] =
    useState<ActivityFilters>(emptyFilters);
  const [activeFilters, setActiveFilters] =
    useState<ActivityFilters>(emptyFilters);
  const [loading, setLoading] = useState(false);
  const requestGeneration = useRef(0);

  const runRequest = (
    request: ActivityPageRequest,
    append: boolean,
  ) => {
    if (!loadPage) {
      return;
    }

    const generation = ++requestGeneration.current;
    setLoading(true);
    void loadPage(request)
      .then((page) => {
        if (generation !== requestGeneration.current) {
          return;
        }
        setEvents((current) =>
          uniqueEvents(append ? current : [], page.events),
        );
        setNextCursor(page.nextCursor);
        setSection(page.section);
        setLoading(false);
      })
      .catch((error: unknown) => {
        if (generation !== requestGeneration.current) {
          return;
        }
        const classified =
          typeof error === "object" && error !== null
            ? (error as {
                guidance?: unknown;
                kind?: unknown;
                message?: unknown;
              })
            : undefined;
        setEvents([]);
        setNextCursor("");
        setSection({
          error: {
            guidance:
              typeof classified?.guidance === "string"
                ? classified.guidance
                : "Check the connection and apply the Activity filters again.",
            kind:
              typeof classified?.kind === "string"
                ? classified.kind
                : "unexpected",
            message:
              typeof classified?.message === "string"
                ? classified.message
                : "Activity could not be loaded safely.",
          },
          snapshot: { loadedAt: "" },
          state: "unavailable",
        });
        setLoading(false);
      });
  };

  const applyFilters = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const filters = { ...draftFilters };
    setActiveFilters(filters);
    runRequest(
      {
        ...filters,
        cursor: "",
        seenEventIds: [],
      },
      false,
    );
  };

  const loadMore = () => {
    runRequest(
      {
        ...activeFilters,
        cursor: nextCursor,
        seenEventIds: events.map((event) => event.eventId),
      },
      true,
    );
  };

  const unavailable =
    section.state === "unavailable" || section.state === "failed";

  return (
    <section aria-label="Activity evidence" className="activity-page">
      <header className="activity-heading">
        <div>
          <span className="page-kicker">Bounded audit record</span>
          <h2>Activity evidence</h2>
        </div>
        <strong data-numeric>
          {events.length} event{events.length === 1 ? "" : "s"}
        </strong>
      </header>

      <form
        aria-label="Activity filters"
        className="activity-filters"
        onSubmit={applyFilters}
      >
        <label>
          <span>Since</span>
          <input
            aria-label="Activity since"
            onChange={(event) =>
              setDraftFilters((current) => ({
                ...current,
                since: event.target.value,
              }))
            }
            placeholder="RFC 3339"
            type="text"
            value={draftFilters.since}
          />
        </label>
        <label>
          <span>Until</span>
          <input
            aria-label="Activity until"
            onChange={(event) =>
              setDraftFilters((current) => ({
                ...current,
                until: event.target.value,
              }))
            }
            placeholder="RFC 3339"
            type="text"
            value={draftFilters.until}
          />
        </label>
        <label>
          <span>Action</span>
          <input
            aria-label="Activity action"
            onChange={(event) =>
              setDraftFilters((current) => ({
                ...current,
                action: event.target.value,
              }))
            }
            placeholder="git_sync"
            type="text"
            value={draftFilters.action}
          />
        </label>
        <label>
          <span>Actor type</span>
          <input
            aria-label="Activity actor type"
            onChange={(event) =>
              setDraftFilters((current) => ({
                ...current,
                actorType: event.target.value,
              }))
            }
            placeholder="operator"
            type="text"
            value={draftFilters.actorType}
          />
        </label>
        <button disabled={!loadPage || loading} type="submit">
          <Filter aria-hidden="true" size={14} strokeWidth={1.8} />
          Apply Activity filters
        </button>
      </form>

      {unavailable ? (
        <div className="activity-local-state">
          <DataState
            guidance={section.error?.guidance}
            kind={sectionKind(section)}
            message={
              section.error?.message ?? "Activity evidence is unavailable."
            }
            title={failureTitle(section)}
          />
        </div>
      ) : events.length === 0 && !loading ? (
        <div className="activity-local-state">
          <DataState
            kind="empty"
            message="No server audit events match the active Activity filters."
            title="No Activity events"
          />
        </div>
      ) : (
        <div className="activity-table-scroll">
          <table aria-label="Activity">
            <thead>
              <tr>
                <th scope="col">Event ID</th>
                <th scope="col">Time</th>
                <th scope="col">Actor</th>
                <th scope="col">Action</th>
                <th scope="col">Resource</th>
                <th scope="col">Status</th>
                <th scope="col">Request ID</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {events.map((event) => (
                <tr key={event.eventId}>
                  <td data-mono>{event.eventId}</td>
                  <td data-mono>{event.occurredAt}</td>
                  <td data-mono>{event.actor}</td>
                  <td data-mono>{displayAction(event.action)}</td>
                  <td className="activity-resource">
                    <span data-mono>{event.resourceId}</span>
                    <small>{event.resourceType}</small>
                  </td>
                  <td>
                    <span className="activity-status" data-status={event.status}>
                      {event.status}
                    </span>
                  </td>
                  <td data-mono>{event.requestId || "Not reported"}</td>
                  <td className="activity-actions">
                    <button
                      aria-label={`Inspect ${event.eventId}`}
                      onClick={() => onInspect?.(event)}
                      type="button"
                    >
                      <ArrowRight
                        aria-hidden="true"
                        size={15}
                        strokeWidth={1.8}
                      />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {loading ? (
        <div aria-live="polite" className="activity-loading">
          <ScrollText aria-hidden="true" size={14} strokeWidth={1.8} />
          Loading bounded Activity page…
        </div>
      ) : null}

      {nextCursor && !unavailable ? (
        <footer className="activity-pagination">
          <span>More server events are available.</span>
          <button disabled={!loadPage || loading} onClick={loadMore} type="button">
            <ArrowDown aria-hidden="true" size={14} strokeWidth={1.8} />
            Load more Activity
          </button>
        </footer>
      ) : null}
    </section>
  );
}
