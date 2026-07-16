import {
  ChevronDown,
  Circle,
  GitPullRequest,
  LayoutDashboard,
  Monitor,
  ScrollText,
  Server,
  Stethoscope,
  X,
} from "lucide-react";
import {
  type ComponentType,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from "react";

import "../styles/theme.css";
import "./AppShell.css";

export type AppPage =
  | "overview"
  | "endpoints"
  | "fleets"
  | "change-requests"
  | "diagnostics"
  | "activity";

interface ConnectionContext {
  connected?: boolean;
  operatorId: string;
  profileName: string;
  serverLabel: string;
}

interface ShellOverlay {
  content: ReactNode;
  onClose: () => void;
  title: string;
}

interface AppShellProps {
  activePage?: AppPage;
  connection: ConnectionContext;
  fleetScope: string;
  initialPage?: AppPage;
  overlay?: ShellOverlay;
  onPageChange?: (page: AppPage) => void;
  renderPage: (page: AppPage) => ReactNode;
}

interface NavigationItem {
  icon: ComponentType<{
    "aria-hidden"?: boolean | "false" | "true";
    size?: number;
    strokeWidth?: number;
  }>;
  id: AppPage;
  label: string;
  summary: string;
}

interface NavigationGroup {
  label: string;
  items: NavigationItem[];
}

const navigationGroups: NavigationGroup[] = [
  {
    label: "Fleet management",
    items: [
      {
        id: "overview",
        label: "Overview",
        summary: "Fleet posture, recent activity, and operator priorities.",
        icon: LayoutDashboard,
      },
      {
        id: "endpoints",
        label: "Endpoints",
        summary: "Inventory, compliance, freshness, and applied state.",
        icon: Monitor,
      },
      {
        id: "fleets",
        label: "Fleets",
        summary: "Membership, versions, and aggregate operational posture.",
        icon: Server,
      },
    ],
  },
  {
    label: "Operations",
    items: [
      {
        id: "change-requests",
        label: "Change requests",
        summary: "Review desired-state changes and their current evidence.",
        icon: GitPullRequest,
      },
      {
        id: "diagnostics",
        label: "Diagnostics",
        summary: "Inspect safe diagnostic results across managed systems.",
        icon: Stethoscope,
      },
      {
        id: "activity",
        label: "Activity",
        summary: "Trace operator and service events in chronological order.",
        icon: ScrollText,
      },
    ],
  },
];

const navigationItems = navigationGroups.flatMap((group) => group.items);

function pageDetails(page: AppPage): NavigationItem {
  return navigationItems.find((item) => item.id === page) ?? navigationItems[0];
}

export function AppShell({
  activePage: controlledPage,
  connection,
  fleetScope,
  initialPage = "overview",
  overlay,
  onPageChange,
  renderPage,
}: AppShellProps) {
  const [localPage, setLocalPage] = useState<AppPage>(initialPage);
  const overlayTitleId = useId();
  const activePage = controlledPage ?? localPage;
  const active = pageDetails(activePage);
  const connected = connection.connected ?? true;

  const selectPage = (page: AppPage) => {
    if (controlledPage === undefined) {
      setLocalPage(page);
    }
    onPageChange?.(page);
  };

  useEffect(() => {
    if (!overlay) {
      return;
    }

    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        overlay.onClose();
      }
    };

    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [overlay]);

  return (
    <div className="app-shell">
      <header className="connection-bar">
        <div className="product-lockup" aria-label="Remotr desktop">
          <span className="product-mark" aria-hidden="true">
            R
          </span>
          <span className="product-name">REMOTR</span>
        </div>

        <div className="connection-context" aria-label="Active connection">
          <span className="connection-state" data-connected={connected}>
            <Circle aria-hidden="true" size={9} strokeWidth={0} />
            {connected ? "Connected" : "Not connected"}
          </span>
          <span className="connection-profile-name">
            {connection.profileName}
          </span>
          <span className="connection-server" data-mono>
            {connection.serverLabel}
          </span>
        </div>

        <div className="operator-context">
          <span className="operator-id" data-mono title={connection.operatorId}>
            {connection.operatorId}
          </span>
          <button className="fleet-scope" type="button">
            <span>Fleet: {fleetScope}</span>
            <ChevronDown aria-hidden="true" size={14} strokeWidth={1.8} />
          </button>
        </div>
      </header>

      <div className="shell-workspace">
        <aside className="navigation-rail">
          <div className="navigation-intro">
            <span className="navigation-kicker">Operator console</span>
            <strong>Fleet workspace</strong>
          </div>

          <nav aria-label="Primary navigation">
            {navigationGroups.map((group) => (
              <section className="navigation-group" key={group.label}>
                <h2>{group.label}</h2>
                <div className="navigation-items">
                  {group.items.map((item) => {
                    const Icon = item.icon;
                    const isActive = item.id === activePage;

                    return (
                      <button
                        aria-current={isActive ? "page" : undefined}
                        className="navigation-item"
                        key={item.id}
                        onClick={() => selectPage(item.id)}
                        type="button"
                      >
                        <Icon aria-hidden="true" size={17} strokeWidth={1.8} />
                        <span>{item.label}</span>
                      </button>
                    );
                  })}
                </div>
              </section>
            ))}
          </nav>

          <div className="navigation-footnote">
            <span className="navigation-footnote-mark" aria-hidden="true" />
            <span>Desired state from Git</span>
          </div>
        </aside>

        <main className="shell-main">
          <header className="page-header">
            <div>
              <span className="page-kicker">Fleet operations</span>
              <h1>{active.label}</h1>
            </div>
            <p>{active.summary}</p>
          </header>

          <div className="content-frame" key={activePage}>
            {renderPage(activePage)}
          </div>
        </main>
      </div>

      {overlay ? (
        <div className="overlay-layer">
          <div aria-hidden="true" className="overlay-scrim" />
          <section
            aria-labelledby={overlayTitleId}
            aria-modal="true"
            className="detail-overlay"
            role="dialog"
          >
            <header className="detail-overlay-header">
              <div>
                <span className="page-kicker">Focused evidence</span>
                <h2 id={overlayTitleId}>{overlay.title}</h2>
              </div>
              <button
                aria-label={`Close ${overlay.title}`}
                className="overlay-close"
                onClick={overlay.onClose}
                type="button"
              >
                <X aria-hidden="true" size={18} strokeWidth={1.8} />
              </button>
            </header>
            <div className="detail-overlay-content">{overlay.content}</div>
          </section>
        </div>
      ) : null}
    </div>
  );
}
