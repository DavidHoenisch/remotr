import { AlertTriangle, CheckCircle2, CircleSlash, Clock3 } from "lucide-react";
import {
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  useRef,
  useState,
} from "react";

import { DataState, type DataStateKind } from "../states/DataState";
import "./EndpointInvestigation.css";

interface ClassifiedSectionError {
  guidance: string;
  kind: string;
  message: string;
}

interface EndpointDetailSection {
  error?: ClassifiedSectionError;
  snapshot: {
    failedAt?: string;
    loadedAt?: string;
    observedAt?: string;
  };
  state: string;
}

interface EndpointDetailHeader {
  compliance: string;
  desiredAgentVersion: string;
  endpointId: string;
  evidenceAt?: string;
  fleet: string;
  freshness: string;
  labels: Array<{ key: string; value: string }>;
  releaseRef: string;
  reportedAgentVersion: string;
  usernames: string[];
}

interface StateEvidenceSubresult {
  desiredSummary: string;
  observedSummary: string;
  reasonCode: string;
  status: string;
  target: string;
}

interface StateEvidenceItem {
  address: string;
  description: string;
  desiredSummary: string;
  name: string;
  observedSummary: string;
  provider: string;
  reasonCode: string;
  status: string;
  subresults: StateEvidenceSubresult[];
  subresultsTruncated: boolean;
}

interface StateEvidence {
  digest: string;
  endpointId: string;
  items: StateEvidenceItem[];
  releaseRef: string;
  reportedAt: string;
  status: string;
}

interface ScheduleEvidence {
  applicable: boolean;
  lastCompletedAt: string;
  lastMessage: string;
  lastScheduledFor: string;
  lastStatus: string;
  name: string;
  schedule: string;
}

interface FirewallEvidence {
  action: string;
  backend: string;
  enforced: boolean;
  ports: number[];
  protocol: string;
  ruleName: string;
  sources: string[];
  timestamp: string;
  wouldHave: string;
}

interface SystemEvidence {
  cpu: string;
  cpuCores: string;
  digest: string;
  hostname: string;
  kernel: string;
  memory: string;
  os: string;
  reportedAt: string;
}

export interface EndpointDetailView {
  firewall: FirewallEvidence[];
  firewallTruncated: boolean;
  header: EndpointDetailHeader;
  schedules: ScheduleEvidence[];
  schedulesTruncated: boolean;
  sections: {
    firewall: EndpointDetailSection;
    overview: EndpointDetailSection;
    schedules: EndpointDetailSection;
    state: EndpointDetailSection;
    system: EndpointDetailSection;
  };
  state: StateEvidence;
  stateTruncated: boolean;
  system: SystemEvidence;
}

type EvidenceTab = "overview" | "state" | "schedules" | "firewall" | "system";

const tabs: Array<{ id: EvidenceTab; label: string; title: string }> = [
  { id: "overview", label: "Overview", title: "Endpoint overview" },
  { id: "state", label: "State", title: "State evidence" },
  { id: "schedules", label: "Schedules", title: "Schedule evidence" },
  { id: "firewall", label: "Firewall", title: "Firewall evidence" },
  { id: "system", label: "System", title: "System evidence" },
];

function displayStatus(value: string): string {
  const words = value.replaceAll("_", " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

function statusTone(status: string): string {
  if (status === "compliant" || status === "recent") {
    return "compliant";
  }
  if (status === "drifted" || status === "stale") {
    return "drifted";
  }
  if (status.includes("failed")) {
    return "error";
  }
  return "neutral";
}

function EvidenceStatus({ status }: { status: string }) {
  const tone = statusTone(status);
  const Icon =
    tone === "compliant"
      ? CheckCircle2
      : tone === "drifted"
        ? AlertTriangle
        : tone === "error"
          ? Clock3
          : CircleSlash;
  return (
    <span className="investigation-status" data-tone={tone}>
      <Icon aria-hidden="true" size={14} strokeWidth={1.8} />
      {displayStatus(status)}
    </span>
  );
}

function stateKind(section: EndpointDetailSection): DataStateKind {
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

function EvidenceSection({
  children,
  emptyMessage,
  section,
  title,
}: {
  children: ReactNode;
  emptyMessage: string;
  section: EndpointDetailSection;
  title: string;
}) {
  if (section.state === "ready") {
    return children;
  }

  const kind = stateKind(section);
  const unavailable = section.state === "unavailable" || section.state === "failed";
  const stateTitle =
    section.state === "empty"
      ? `No ${title.toLocaleLowerCase()}`
      : unavailable
        ? `${title} unavailable`
        : section.state === "loading"
          ? `Loading ${title.toLocaleLowerCase()}`
          : `${title} ${section.state}`;
  const message =
    section.state === "empty"
      ? emptyMessage
      : section.error?.message ?? `The latest ${title.toLocaleLowerCase()} is still being prepared.`;

  return (
    <DataState
      guidance={section.error?.guidance}
      kind={kind}
      message={message}
      title={stateTitle}
    >
      {children}
    </DataState>
  );
}

function EvidenceGrid({
  rows,
}: {
  rows: Array<{ label: string; value: ReactNode }>;
}) {
  return (
    <dl className="investigation-evidence-grid">
      {rows.map((row) => (
        <div key={row.label}>
          <dt>{row.label}</dt>
          <dd>{row.value || "Not reported"}</dd>
        </div>
      ))}
    </dl>
  );
}

function OverviewEvidence({ detail }: { detail: EndpointDetailView }) {
  const { header } = detail;
  return (
    <EvidenceGrid
      rows={[
        { label: "Desired agent", value: header.desiredAgentVersion },
        {
          label: "Users",
          value: header.usernames.length > 0 ? header.usernames.join(", ") : "None reported",
        },
        { label: "Last evidence", value: header.evidenceAt ?? "Never" },
        { label: "State digest", value: detail.state.digest },
        { label: "System digest", value: detail.system.digest },
        { label: "System observed", value: detail.system.reportedAt },
      ]}
    />
  );
}

function StateEvidenceView({ detail }: { detail: EndpointDetailView }) {
  return (
    <div className="investigation-stack">
      <EvidenceGrid
        rows={[
          { label: "Endpoint", value: detail.state.endpointId },
          { label: "Release ref", value: detail.state.releaseRef },
          { label: "Digest", value: detail.state.digest },
          { label: "Reported", value: detail.state.reportedAt },
          {
            label: "Status",
            value: <EvidenceStatus status={detail.state.status} />,
          },
        ]}
      />
      {detail.state.items.map((item) => (
        <article className="investigation-evidence-card" key={item.address}>
          <header>
            <div>
              <span data-mono>{item.address}</span>
              <h4>{item.name}</h4>
            </div>
            <EvidenceStatus status={item.status} />
          </header>
          {item.description ? <p>{item.description}</p> : null}
          <EvidenceGrid
            rows={[
              { label: "Provider", value: item.provider },
              { label: "Reason", value: item.reasonCode },
              { label: "Desired", value: item.desiredSummary },
              { label: "Observed", value: item.observedSummary },
            ]}
          />
          {item.subresults.map((subresult) => (
            <div className="investigation-subresult" key={subresult.target}>
              <strong data-mono>{subresult.target}</strong>
              <EvidenceStatus status={subresult.status} />
              <span>{subresult.desiredSummary}</span>
              <span>{subresult.observedSummary}</span>
            </div>
          ))}
          {item.subresultsTruncated ? (
            <p className="investigation-truncated">Additional subresults omitted.</p>
          ) : null}
        </article>
      ))}
      {detail.stateTruncated ? (
        <p className="investigation-truncated">Additional State items omitted.</p>
      ) : null}
    </div>
  );
}

function ScheduleEvidenceView({ detail }: { detail: EndpointDetailView }) {
  return (
    <div className="investigation-card-grid">
      {detail.schedules.map((schedule) => (
        <article className="investigation-evidence-card" key={schedule.name}>
          <header>
            <div>
              <span data-mono>{schedule.schedule}</span>
              <h4>{schedule.name}</h4>
            </div>
            <span className="investigation-applicability">
              {schedule.applicable ? "Applicable" : "Not applicable"}
            </span>
          </header>
          <EvidenceGrid
            rows={[
              { label: "Last status", value: schedule.lastStatus },
              { label: "Last message", value: schedule.lastMessage },
              { label: "Scheduled for", value: schedule.lastScheduledFor },
              { label: "Completed", value: schedule.lastCompletedAt },
            ]}
          />
        </article>
      ))}
      {detail.schedulesTruncated ? (
        <p className="investigation-truncated">Additional schedules omitted.</p>
      ) : null}
    </div>
  );
}

function FirewallEvidenceView({ detail }: { detail: EndpointDetailView }) {
  return (
    <div className="investigation-card-grid">
      {detail.firewall.map((entry, index) => (
        <article
          className="investigation-evidence-card"
          key={`${entry.timestamp}-${entry.ruleName}-${index}`}
        >
          <header>
            <div>
              <span data-mono>{entry.timestamp}</span>
              <h4>{entry.ruleName}</h4>
            </div>
            <span className="investigation-applicability">
              {entry.enforced ? "Enforced" : "Observed"}
            </span>
          </header>
          <EvidenceGrid
            rows={[
              { label: "Action", value: entry.action },
              { label: "Protocol", value: entry.protocol },
              { label: "Ports", value: entry.ports.join(", ") || "None" },
              { label: "Sources", value: entry.sources.join(", ") || "None" },
              { label: "Backend", value: entry.backend },
              { label: "Predicted effect", value: entry.wouldHave },
            ]}
          />
        </article>
      ))}
      {detail.firewallTruncated ? (
        <p className="investigation-truncated">Additional firewall evidence omitted.</p>
      ) : null}
    </div>
  );
}

function SystemEvidenceView({ detail }: { detail: EndpointDetailView }) {
  return (
    <EvidenceGrid
      rows={[
        { label: "Hostname", value: detail.system.hostname },
        { label: "Operating system", value: detail.system.os },
        { label: "Kernel", value: detail.system.kernel },
        { label: "CPU", value: detail.system.cpu },
        { label: "CPU cores", value: detail.system.cpuCores },
        { label: "Memory", value: detail.system.memory },
        { label: "Digest", value: detail.system.digest },
        { label: "Reported", value: detail.system.reportedAt },
      ]}
    />
  );
}

export function EndpointInvestigation({ detail }: { detail: EndpointDetailView }) {
  const [activeTab, setActiveTab] = useState<EvidenceTab>("overview");
  const tabRefs = useRef<Partial<Record<EvidenceTab, HTMLButtonElement | null>>>(
    {},
  );
  const { header } = detail;
  const selectedTab = tabs.find((tab) => tab.id === activeTab) ?? tabs[0];
  const panel = (() => {
    switch (activeTab) {
      case "overview":
        return <OverviewEvidence detail={detail} />;
      case "state":
        return <StateEvidenceView detail={detail} />;
      case "schedules":
        return <ScheduleEvidenceView detail={detail} />;
      case "firewall":
        return <FirewallEvidenceView detail={detail} />;
      case "system":
        return <SystemEvidenceView detail={detail} />;
    }
  })();

  const moveTab = (
    event: ReactKeyboardEvent<HTMLButtonElement>,
    currentIndex: number,
  ) => {
    let nextIndex: number | undefined;
    switch (event.key) {
      case "ArrowLeft":
        nextIndex = (currentIndex - 1 + tabs.length) % tabs.length;
        break;
      case "ArrowRight":
        nextIndex = (currentIndex + 1) % tabs.length;
        break;
      case "Home":
        nextIndex = 0;
        break;
      case "End":
        nextIndex = tabs.length - 1;
        break;
    }
    if (nextIndex === undefined) {
      return;
    }

    event.preventDefault();
    const nextTab = tabs[nextIndex];
    setActiveTab(nextTab.id);
    tabRefs.current[nextTab.id]?.focus();
  };

  return (
    <div className="endpoint-investigation">
      <section aria-label="Endpoint summary" className="investigation-summary">
        <EvidenceGrid
          rows={[
            { label: "Endpoint", value: header.endpointId },
            { label: "Fleet", value: header.fleet },
            {
              label: "Compliance",
              value: <EvidenceStatus status={header.compliance} />,
            },
            {
              label: "Freshness",
              value: <EvidenceStatus status={header.freshness} />,
            },
            { label: "Reported agent", value: header.reportedAgentVersion },
            { label: "Release ref", value: header.releaseRef },
          ]}
        />
        {header.labels.length > 0 ? (
          <div className="investigation-labels" aria-label="Endpoint Labels">
            {header.labels.map((label) => (
              <span key={label.key}>
                <span>{label.key}</span>
                <strong>{label.value}</strong>
              </span>
            ))}
          </div>
        ) : (
          <p className="investigation-no-labels">No Labels reported.</p>
        )}
      </section>

      <div aria-label="Endpoint evidence" className="investigation-tabs" role="tablist">
        {tabs.map((tab, index) => (
          <button
            aria-controls={`endpoint-panel-${tab.id}`}
            aria-selected={activeTab === tab.id}
            id={`endpoint-tab-${tab.id}`}
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            onKeyDown={(event) => moveTab(event, index)}
            ref={(element) => {
              tabRefs.current[tab.id] = element;
            }}
            role="tab"
            tabIndex={activeTab === tab.id ? 0 : -1}
            type="button"
          >
            {tab.label}
          </button>
        ))}
      </div>

      <section
        aria-labelledby={`endpoint-tab-${activeTab}`}
        className="investigation-panel"
        id={`endpoint-panel-${activeTab}`}
        role="tabpanel"
      >
        <EvidenceSection
          emptyMessage={`The Endpoint reported no ${selectedTab.title.toLocaleLowerCase()}.`}
          section={detail.sections[activeTab]}
          title={selectedTab.title}
        >
          {panel}
        </EvidenceSection>
      </section>
    </div>
  );
}
