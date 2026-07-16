import { Download, FileSearch, ShieldCheck } from "lucide-react";
import { useState } from "react";

import { normalizeActionError } from "../actions/useActionController";
import type {
  AssetInventoryView,
  AuditExportInfoView,
  DiagnosticLifecycleView,
  FirewallExportRequest,
  FirewallReportView,
  FleetOperationalReportsView,
  ReadExportSaveResult,
} from "./readExport";
import "./ReadExportPage.css";

interface ReadExportPageProps {
  endpoints: Array<{ endpointId: string; fleet: string }>;
  fleets: string[];
  loadAssetInventory?: () => Promise<AssetInventoryView>;
  loadAuditExportInfo?: () => Promise<AuditExportInfoView>;
  loadDiagnosticRequest?: (requestId: string) => Promise<DiagnosticLifecycleView>;
  loadFirewallReport?: (endpointId: string) => Promise<FirewallReportView>;
  loadFleetOperationalReports?: (
    fleet: string,
  ) => Promise<FleetOperationalReportsView>;
  saveAssetInventory?: (format: "csv" | "json") => Promise<ReadExportSaveResult>;
  saveFirewallReport?: (
    request: FirewallExportRequest,
  ) => Promise<ReadExportSaveResult>;
}

interface ReportFailure {
  guidance: string;
  message: string;
}

function ReportFailureNote({ failure }: { failure?: ReportFailure }) {
  if (!failure) {
    return null;
  }
  return (
    <div className="report-failure" role="alert">
      <strong>{failure.message}</strong>
      <span>{failure.guidance}</span>
    </div>
  );
}

function SaveResult({ result }: { result?: ReadExportSaveResult }) {
  if (!result) {
    return null;
  }
  return (
    <p className="report-save-result" role="status">
      {result.status === "saved" ? (
        <>
          Saved to <span data-mono>{result.path}</span>
        </>
      ) : (
        "Save canceled."
      )}
    </p>
  );
}

function failureFrom(error: unknown): ReportFailure {
  const normalized = normalizeActionError(error);
  return { guidance: normalized.guidance, message: normalized.message };
}

export function ReadExportPage({
  endpoints,
  fleets,
  loadAssetInventory,
  loadAuditExportInfo,
  loadDiagnosticRequest,
  loadFirewallReport,
  loadFleetOperationalReports,
  saveAssetInventory,
  saveFirewallReport,
}: ReadExportPageProps) {
  const [inventory, setInventory] = useState<AssetInventoryView>();
  const [inventoryPending, setInventoryPending] = useState(false);
  const [inventoryFailure, setInventoryFailure] = useState<ReportFailure>();
  const [inventorySave, setInventorySave] = useState<ReadExportSaveResult>();
  const [fleet, setFleet] = useState(fleets[0] ?? "");
  const [fleetReports, setFleetReports] =
    useState<FleetOperationalReportsView>();
  const [fleetPending, setFleetPending] = useState(false);
  const [fleetFailure, setFleetFailure] = useState<ReportFailure>();
  const [firewallEndpoint, setFirewallEndpoint] = useState(
    endpoints[0]?.endpointId ?? "",
  );
  const [firewallReport, setFirewallReport] = useState<FirewallReportView>();
  const [firewallPending, setFirewallPending] = useState(false);
  const [firewallFailure, setFirewallFailure] = useState<ReportFailure>();
  const [firewallSave, setFirewallSave] = useState<ReadExportSaveResult>();
  const [auditInfo, setAuditInfo] = useState<AuditExportInfoView>();
  const [auditPending, setAuditPending] = useState(false);
  const [auditFailure, setAuditFailure] = useState<ReportFailure>();
  const [diagnosticRequestId, setDiagnosticRequestId] = useState("");
  const [diagnostic, setDiagnostic] = useState<DiagnosticLifecycleView>();
  const [diagnosticPending, setDiagnosticPending] = useState(false);
  const [diagnosticFailure, setDiagnosticFailure] = useState<ReportFailure>();

  const loadInventory = async () => {
    if (!loadAssetInventory || inventoryPending) return;
    setInventoryPending(true);
    setInventoryFailure(undefined);
    setInventorySave(undefined);
    try {
      setInventory(await loadAssetInventory());
    } catch (error: unknown) {
      setInventoryFailure(failureFrom(error));
    } finally {
      setInventoryPending(false);
    }
  };

  const saveInventory = async (format: "csv" | "json") => {
    if (!saveAssetInventory) return;
    setInventoryFailure(undefined);
    try {
      setInventorySave(await saveAssetInventory(format));
    } catch (error: unknown) {
      setInventoryFailure(failureFrom(error));
    }
  };

  const loadFleetReports = async () => {
    if (!loadFleetOperationalReports || !fleet || fleetPending) return;
    setFleetPending(true);
    setFleetFailure(undefined);
    try {
      setFleetReports(await loadFleetOperationalReports(fleet));
    } catch (error: unknown) {
      setFleetFailure(failureFrom(error));
    } finally {
      setFleetPending(false);
    }
  };

  const loadFirewall = async () => {
    if (!loadFirewallReport || !firewallEndpoint || firewallPending) return;
    setFirewallPending(true);
    setFirewallFailure(undefined);
    setFirewallSave(undefined);
    try {
      setFirewallReport(await loadFirewallReport(firewallEndpoint));
    } catch (error: unknown) {
      setFirewallFailure(failureFrom(error));
    } finally {
      setFirewallPending(false);
    }
  };

  const saveFirewall = async (format: "csv" | "json") => {
    if (!saveFirewallReport || !firewallEndpoint) return;
    setFirewallFailure(undefined);
    try {
      setFirewallSave(
        await saveFirewallReport({ endpointId: firewallEndpoint, format }),
      );
    } catch (error: unknown) {
      setFirewallFailure(failureFrom(error));
    }
  };

  const loadAuditInfo = async () => {
    if (!loadAuditExportInfo || auditPending) return;
    setAuditPending(true);
    setAuditFailure(undefined);
    try {
      setAuditInfo(await loadAuditExportInfo());
    } catch (error: unknown) {
      setAuditFailure(failureFrom(error));
    } finally {
      setAuditPending(false);
    }
  };

  const loadDiagnostic = async () => {
    const requestId = diagnosticRequestId.trim();
    if (!loadDiagnosticRequest || !requestId || diagnosticPending) return;
    setDiagnosticPending(true);
    setDiagnosticFailure(undefined);
    try {
      setDiagnostic(await loadDiagnosticRequest(requestId));
    } catch (error: unknown) {
      setDiagnosticFailure(failureFrom(error));
    } finally {
      setDiagnosticPending(false);
    }
  };

  return (
    <section aria-label="Read and export reports" className="reports-workspace">
      <div className="reports-intro">
        <span className="page-kicker">Operational evidence</span>
        <h2>Read and export reports</h2>
        <p>
          Inspect structured server evidence here. CSV and JSON exports use a
          native file destination that you choose.
        </p>
      </div>

      <div className="reports-grid">
        <article className="report-panel report-panel-wide">
          <header>
            <div>
              <span className="report-number">01</span>
              <h3>Asset inventory</h3>
              <p>Bounded system and agent evidence across managed Endpoints.</p>
            </div>
            <button
              disabled={!loadAssetInventory || inventoryPending}
              onClick={() => void loadInventory()}
              type="button"
            >
              <FileSearch aria-hidden="true" size={14} />
              {inventoryPending ? "Loading inventory…" : "Load asset inventory"}
            </button>
          </header>
          <ReportFailureNote failure={inventoryFailure} />
          {inventory ? (
            <>
              <div className="report-table-frame">
                <table aria-label="Asset inventory">
                  <thead>
                    <tr>
                      <th>Endpoint</th>
                      <th>Fleet</th>
                      <th>Operating system</th>
                      <th>CPU</th>
                      <th>RAM</th>
                      <th>Network</th>
                      <th>Encryption</th>
                      <th>Agent</th>
                    </tr>
                  </thead>
                  <tbody>
                    {inventory.rows.map((row) => (
                      <tr key={row.endpointId}>
                        <td data-mono>{row.endpointId}</td>
                        <td>{row.fleet}</td>
                        <td>{row.os || "Not reported"}</td>
                        <td>{row.cpu || "Not reported"}</td>
                        <td>{row.ram || "—"}</td>
                        <td>
                          <span>{row.primaryIp || "—"}</span>
                          <small data-mono>{row.macAddress}</small>
                        </td>
                        <td>{row.diskEncryption || "Not reported"}</td>
                        <td data-mono>{row.agentVersion || "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <footer>
                <span>{inventory.rows.length} Endpoint records</span>
                <div className="report-actions">
                  <button onClick={() => void saveInventory("csv")} type="button">
                    <Download aria-hidden="true" size={14} /> Save inventory as CSV
                  </button>
                  <button onClick={() => void saveInventory("json")} type="button">
                    Save inventory as JSON
                  </button>
                </div>
              </footer>
              <SaveResult result={inventorySave} />
            </>
          ) : null}
        </article>

        <article className="report-panel">
          <header>
            <div>
              <span className="report-number">02</span>
              <h3>Fleet State and schedules</h3>
              <p>Desired-state results paired with reported cron evidence.</p>
            </div>
          </header>
          <div className="report-controls">
            <label>
              Fleet report scope
              <select
                aria-label="Fleet report scope"
                onChange={(event) => setFleet(event.target.value)}
                value={fleet}
              >
                {fleets.map((name) => (
                  <option key={name} value={name}>{name}</option>
                ))}
              </select>
            </label>
            <button
              disabled={!loadFleetOperationalReports || !fleet || fleetPending}
              onClick={() => void loadFleetReports()}
              type="button"
            >
              {fleetPending ? "Loading reports…" : "Load Fleet reports"}
            </button>
          </div>
          <ReportFailureNote failure={fleetFailure} />
          {fleetReports ? (
            <div className="report-evidence-split">
              <section aria-label="Fleet State evidence">
                <h4>State</h4>
                {fleetReports.states.flatMap((state) =>
                  state.items.map((item) => (
                    <div className="report-evidence-row" key={`${state.endpointId}-${item.address}`}>
                      <span data-mono>{state.endpointId}</span>
                      <strong>{item.desiredSummary}</strong>
                      <small>{item.observedSummary}</small>
                    </div>
                  )),
                )}
              </section>
              <section aria-label="Fleet schedule evidence">
                <h4>Schedules</h4>
                {fleetReports.schedules.map((schedule) => (
                  <div className="report-evidence-row" key={`${schedule.endpointId}-${schedule.name}`}>
                    <strong>{schedule.name}</strong>
                    <span data-mono>{schedule.schedule}</span>
                    <small>{schedule.lastStatus || "Not yet reported"}</small>
                  </div>
                ))}
              </section>
            </div>
          ) : null}
        </article>

        <article className="report-panel">
          <header>
            <div>
              <span className="report-number">03</span>
              <h3>Firewall</h3>
              <p>Policy audit events and the latest live configuration.</p>
            </div>
            <ShieldCheck aria-hidden="true" className="report-panel-icon" size={20} />
          </header>
          <div className="report-controls">
            <label>
              Firewall Endpoint
              <select
                aria-label="Firewall Endpoint"
                onChange={(event) => setFirewallEndpoint(event.target.value)}
                value={firewallEndpoint}
              >
                {endpoints.map((item) => (
                  <option key={item.endpointId} value={item.endpointId}>
                    {item.endpointId}
                  </option>
                ))}
              </select>
            </label>
            <button
              disabled={!loadFirewallReport || !firewallEndpoint || firewallPending}
              onClick={() => void loadFirewall()}
              type="button"
            >
              {firewallPending ? "Loading report…" : "Load Firewall report"}
            </button>
          </div>
          <ReportFailureNote failure={firewallFailure} />
          {firewallReport ? (
            <>
              <dl className="report-definition-grid">
                <div><dt>Backend</dt><dd>{firewallReport.live.backend || "Not reported"}</dd></div>
                <div><dt>Default zone</dt><dd>{firewallReport.live.defaultZone || "—"}</dd></div>
                <div><dt>Audit events</dt><dd data-numeric>{firewallReport.audit.length}</dd></div>
              </dl>
              <div className="report-chip-list">
                {firewallReport.audit.map((event) => (
                  <span key={`${event.timestamp}-${event.ruleName}`}>{event.ruleName}</span>
                ))}
                {firewallReport.live.zones.flatMap((zone) =>
                  zone.ports.map((port) => <span key={`${zone.name}-${port}`}>{port}</span>),
                )}
              </div>
              <footer>
                <span>{firewallReport.endpointId}</span>
                <div className="report-actions">
                  <button onClick={() => void saveFirewall("csv")} type="button">
                    <Download aria-hidden="true" size={14} /> Save Firewall as CSV
                  </button>
                  <button onClick={() => void saveFirewall("json")} type="button">
                    Save Firewall as JSON
                  </button>
                </div>
              </footer>
              <SaveResult result={firewallSave} />
            </>
          ) : null}
        </article>

        <article className="report-panel report-panel-compact">
          <header>
            <div>
              <span className="report-number">04</span>
              <h3>Audit export</h3>
              <p>Server-advertised path and schema key for audit consumers.</p>
            </div>
          </header>
          <button
            disabled={!loadAuditExportInfo || auditPending}
            onClick={() => void loadAuditInfo()}
            type="button"
          >
            {auditPending ? "Loading export info…" : "Load audit export info"}
          </button>
          <ReportFailureNote failure={auditFailure} />
          {auditInfo ? (
            <dl className="report-definition-grid report-definition-stack">
              <div><dt>Export path</dt><dd data-mono>{auditInfo.exportPath}</dd></div>
              <div><dt>Path key</dt><dd data-mono>{auditInfo.pathKey}</dd></div>
            </dl>
          ) : null}
        </article>

        <article className="report-panel report-panel-compact">
          <header>
            <div>
              <span className="report-number">05</span>
              <h3>Diagnostic lifecycle</h3>
              <p>Inspect one request from creation through bundle readiness.</p>
            </div>
          </header>
          <div className="report-controls">
            <label>
              Diagnostic request ID
              <input
                aria-label="Diagnostic request ID"
                onChange={(event) => setDiagnosticRequestId(event.target.value)}
                value={diagnosticRequestId}
              />
            </label>
            <button
              disabled={!loadDiagnosticRequest || !diagnosticRequestId.trim() || diagnosticPending}
              onClick={() => void loadDiagnostic()}
              type="button"
            >
              {diagnosticPending ? "Loading lifecycle…" : "Load diagnostic lifecycle"}
            </button>
          </div>
          <ReportFailureNote failure={diagnosticFailure} />
          {diagnostic ? (
            <div
              aria-label={`Diagnostic lifecycle ${diagnostic.requestId}`}
              className="diagnostic-lifecycle"
              role="status"
            >
              <strong>{diagnostic.status}</strong>
              <span data-mono>{diagnostic.requestId}</span>
              <dl>
                <div><dt>Endpoint</dt><dd>{diagnostic.endpointId}</dd></div>
                <div><dt>Completed</dt><dd>{diagnostic.completedAt || "Pending"}</dd></div>
                <div><dt>Bundle size</dt><dd>{diagnostic.sizeBytes} bytes</dd></div>
              </dl>
            </div>
          ) : null}
        </article>
      </div>
    </section>
  );
}
