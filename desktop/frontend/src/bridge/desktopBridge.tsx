import {
  createContext,
  type PropsWithChildren,
  useContext,
} from "react";

import {
  ClearDeploymentToken,
  ClearEnrollmentToken,
  CopyDeploymentToken,
  CopyEnrollmentToken,
  CreateDeploymentToken,
  CreateEnrollmentToken,
  GetApplicationInfo,
  GetDiagnosticCapabilities,
  ListDeploymentTokens,
  LoadAssetInventory,
  LoadAuditExportInfo,
  LoadDiagnosticRequest,
  LoadDeploymentToken,
  LoadFirewallReport,
  LoadFleetOperationalReports,
  RemoveEndpoint,
  RemoveEndpointLabel,
  RequestEndpointAgentUpgrade,
  RequestDiagnosticCollection,
  RequestFleetAgentUpgrade,
  RequestGitSync,
  RevokeDeploymentToken,
  SaveAssetInventory,
  SaveDiagnosticBundle,
  SaveDeploymentToken,
  SaveFirewallReport,
  SetEndpointLabel,
} from "../../wailsjs/go/main/App";
import type { ActionAcknowledgement } from "../actions/useActionController";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "../actions/enrollmentToken";
import type {
  EndpointLabelRemoveRequest,
  EndpointLabelResult,
  EndpointLabelSetRequest,
} from "../actions/endpointLabel";
import type {
  EndpointUpgradeRequest,
  EndpointUpgradeResult,
} from "../actions/endpointUpgrade";
import type {
  FleetUpgradeRequest,
  FleetUpgradeResult,
} from "../actions/fleetUpgrade";
import type {
  DiagnosticCapabilities,
  DiagnosticCollectionRequest,
  DiagnosticCollectionResult,
} from "../actions/diagnosticCollection";
import type { DiagnosticBundleSaveResult } from "../actions/diagnosticBundle";
import type {
  EndpointRemovalRequest,
  EndpointRemovalResult,
} from "../actions/endpointRemoval";
import type {
  DeploymentTokenCreateRequest,
  DeploymentTokenCreateResult,
  DeploymentTokenRevokeRequest,
  DeploymentTokenSaveResult,
  DeploymentTokenView,
} from "../actions/deploymentToken";
import type {
  AssetInventoryView,
  AuditExportInfoView,
  DiagnosticLifecycleView,
  FirewallExportRequest,
  FirewallReportView,
  FleetOperationalReportsView,
  ReadExportSaveResult,
  ReportSectionResult,
} from "../reports/readExport";

export interface ApplicationInfo {
  name: string;
  version: string;
}

export interface DesktopBridge {
  clearDeploymentToken(): Promise<void>;
  clearEnrollmentToken(): Promise<void>;
  copyDeploymentToken(): Promise<void>;
  copyEnrollmentToken(): Promise<void>;
  createEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<EnrollmentTokenResult>;
  createDeploymentToken(
    request: DeploymentTokenCreateRequest,
  ): Promise<DeploymentTokenCreateResult>;
  getApplicationInfo(): Promise<ApplicationInfo>;
  getDiagnosticCapabilities(): Promise<DiagnosticCapabilities>;
  listDeploymentTokens(): Promise<DeploymentTokenView[]>;
  loadAssetInventory(): Promise<AssetInventoryView>;
  loadAuditExportInfo(): Promise<AuditExportInfoView>;
  loadDeploymentToken(label: string): Promise<DeploymentTokenView>;
  loadDiagnosticRequest(requestId: string): Promise<DiagnosticLifecycleView>;
  loadFirewallReport(endpointId: string): Promise<FirewallReportView>;
  loadFleetOperationalReports(
    fleet: string,
  ): Promise<FleetOperationalReportsView>;
  removeEndpointLabel(
    request: EndpointLabelRemoveRequest,
  ): Promise<EndpointLabelResult>;
  removeEndpoint(request: EndpointRemovalRequest): Promise<EndpointRemovalResult>;
  requestEndpointAgentUpgrade(
    request: EndpointUpgradeRequest,
  ): Promise<EndpointUpgradeResult>;
  requestDiagnosticCollection(
    request: DiagnosticCollectionRequest,
  ): Promise<DiagnosticCollectionResult>;
  requestFleetAgentUpgrade(
    request: FleetUpgradeRequest,
  ): Promise<FleetUpgradeResult>;
  requestGitSync(): Promise<ActionAcknowledgement>;
  revokeDeploymentToken(
    request: DeploymentTokenRevokeRequest,
  ): Promise<DeploymentTokenView>;
  saveAssetInventory(format: "csv" | "json"): Promise<ReadExportSaveResult>;
  saveDiagnosticBundle(requestId: string): Promise<DiagnosticBundleSaveResult>;
  saveDeploymentToken(label: string): Promise<DeploymentTokenSaveResult>;
  saveFirewallReport(
    request: FirewallExportRequest,
  ): Promise<ReadExportSaveResult>;
  setEndpointLabel(
    request: EndpointLabelSetRequest,
  ): Promise<EndpointLabelResult>;
}

interface GeneratedGitSyncResult {
  acceptedAt: string;
  action: string;
  affectedEvidence: string[];
  summary: string;
  target: string;
}

interface GeneratedEnrollmentTokenResult {
  expiresAt: string;
  fleet: string;
  token: string;
}

interface GeneratedEndpointLabelResult {
  effect: string;
  endpointId: string;
  key: string;
  value: string;
  labels: Array<{ key: string; value: string }>;
}

interface GeneratedEndpointUpgradeResult {
  affectedEvidence: string[];
  endpointId: string;
  status: string;
  version: string;
}

interface GeneratedFleetUpgradeResult {
  acceptedEndpoints: number;
  fleet: string;
  status: string;
  version: string;
}

interface GeneratedDiagnosticCapabilities {
  collectors: string[];
  maxTimeSpanSeconds: number;
}

interface GeneratedDiagnosticCollectionResult {
  collectors: string[];
  createdAt?: string;
  endpointId: string;
  expiresAt?: string;
  requestId: string;
  since: string;
  status: string;
  until: string;
}

interface GeneratedDiagnosticBundleSaveResult {
  path?: string;
  sizeBytes?: number;
  status: string;
}

interface GeneratedReadExportSaveResult {
  path?: string;
  sizeBytes?: number;
  status: string;
}

interface GeneratedDeploymentTokenSaveResult {
  path?: string;
  sizeBytes?: number;
  status: string;
}

interface GeneratedDeploymentTokenView {
  createdAt: string;
  expiresAt: string;
  fleet: string;
  id: string;
  label: string;
  lastUsedAt?: string;
  revokedAt?: string;
  status: string;
}

interface GeneratedDeploymentTokenCreateResult {
  metadata: GeneratedDeploymentTokenView;
  token: string;
}

interface GeneratedEndpointRemovalResult {
  affectedEvidence: string[];
  credentialStatus: string;
  endpointId: string;
  status: string;
}

function adaptEndpointLabelEffect(
  effect: string,
): EndpointLabelResult["effect"] {
  if (effect === "added" || effect === "removed" || effect === "replaced") {
    return effect;
  }
  throw new Error("The native bridge returned an unknown Label effect.");
}

export interface GeneratedBindings {
  ClearDeploymentToken(): Promise<void>;
  ClearEnrollmentToken(): Promise<void>;
  CopyDeploymentToken(): Promise<void>;
  CopyEnrollmentToken(): Promise<void>;
  CreateEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<GeneratedEnrollmentTokenResult>;
  CreateDeploymentToken(
    request: DeploymentTokenCreateRequest,
  ): Promise<GeneratedDeploymentTokenCreateResult>;
  GetApplicationInfo(): Promise<ApplicationInfo>;
  GetDiagnosticCapabilities(): Promise<GeneratedDiagnosticCapabilities>;
  ListDeploymentTokens(): Promise<GeneratedDeploymentTokenView[]>;
  LoadAssetInventory(): Promise<AssetInventoryView>;
  LoadAuditExportInfo(): Promise<AuditExportInfoView>;
  LoadDiagnosticRequest(requestId: string): Promise<DiagnosticLifecycleView>;
  LoadDeploymentToken(label: string): Promise<GeneratedDeploymentTokenView>;
  LoadFirewallReport(endpointId: string): Promise<FirewallReportView>;
  LoadFleetOperationalReports(
    fleet: string,
  ): Promise<FleetOperationalReportsView>;
  RemoveEndpoint(
    request: EndpointRemovalRequest,
  ): Promise<GeneratedEndpointRemovalResult>;
  RemoveEndpointLabel(
    request: EndpointLabelRemoveRequest,
  ): Promise<GeneratedEndpointLabelResult>;
  RequestEndpointAgentUpgrade(
    request: EndpointUpgradeRequest,
  ): Promise<GeneratedEndpointUpgradeResult>;
  RequestDiagnosticCollection(
    request: DiagnosticCollectionRequest,
  ): Promise<GeneratedDiagnosticCollectionResult>;
  RequestFleetAgentUpgrade(
    request: FleetUpgradeRequest,
  ): Promise<GeneratedFleetUpgradeResult>;
  RequestGitSync(): Promise<GeneratedGitSyncResult>;
  RevokeDeploymentToken(
    request: DeploymentTokenRevokeRequest,
  ): Promise<GeneratedDeploymentTokenView>;
  SaveAssetInventory(format: string): Promise<GeneratedReadExportSaveResult>;
  SaveDiagnosticBundle(
    requestId: string,
  ): Promise<GeneratedDiagnosticBundleSaveResult>;
  SaveDeploymentToken(
    label: string,
  ): Promise<GeneratedDeploymentTokenSaveResult>;
  SaveFirewallReport(
    request: FirewallExportRequest,
  ): Promise<GeneratedReadExportSaveResult>;
  SetEndpointLabel(
    request: EndpointLabelSetRequest,
  ): Promise<GeneratedEndpointLabelResult>;
}

const generatedBindings: GeneratedBindings = {
  ClearDeploymentToken,
  ClearEnrollmentToken,
  CopyDeploymentToken,
  CopyEnrollmentToken,
  CreateDeploymentToken,
  CreateEnrollmentToken,
  GetApplicationInfo,
  GetDiagnosticCapabilities,
  ListDeploymentTokens,
  LoadAssetInventory,
  LoadAuditExportInfo,
  LoadDiagnosticRequest,
  LoadDeploymentToken,
  LoadFirewallReport,
  LoadFleetOperationalReports,
  RemoveEndpoint,
  RemoveEndpointLabel,
  RequestEndpointAgentUpgrade,
  RequestDiagnosticCollection,
  RequestFleetAgentUpgrade,
  RequestGitSync,
  RevokeDeploymentToken,
  SaveAssetInventory,
  SaveDiagnosticBundle,
  SaveDeploymentToken,
  SaveFirewallReport,
  SetEndpointLabel,
};

function adaptEndpointLabelResult(
  result: GeneratedEndpointLabelResult,
): EndpointLabelResult {
  return {
    effect: adaptEndpointLabelEffect(result.effect),
    endpointId: result.endpointId,
    key: result.key,
    labels: result.labels.map((label) => ({ ...label })),
    value: result.value,
  };
}

function cloneReportSection(section: ReportSectionResult): ReportSectionResult {
  return {
    ...(section.error ? { error: { ...section.error } } : {}),
    snapshot: { ...section.snapshot },
    state: section.state,
  };
}

function adaptReadExportSaveResult(
  result: GeneratedReadExportSaveResult,
): ReadExportSaveResult {
  if (result.status !== "saved" && result.status !== "canceled") {
    throw new Error("The native bridge returned an unknown export save state.");
  }
  return {
    ...(result.path ? { path: result.path } : {}),
    ...(typeof result.sizeBytes === "number"
      ? { sizeBytes: result.sizeBytes }
      : {}),
    status: result.status,
  };
}

function cloneDeploymentToken(
  token: GeneratedDeploymentTokenView,
): DeploymentTokenView {
  return {
    ...token,
    lastUsedAt: token.lastUsedAt ?? "",
    revokedAt: token.revokedAt ?? "",
  };
}

function adaptDeploymentTokenSaveResult(
  result: GeneratedDeploymentTokenSaveResult,
): DeploymentTokenSaveResult {
  if (result.status !== "saved" && result.status !== "canceled") {
    throw new Error(
      "The native bridge returned an unknown deployment token save state.",
    );
  }
  return {
    ...(result.path ? { path: result.path } : {}),
    ...(typeof result.sizeBytes === "number"
      ? { sizeBytes: result.sizeBytes }
      : {}),
    status: result.status,
  };
}

function adaptEndpointUpgradeResult(
  result: GeneratedEndpointUpgradeResult,
): EndpointUpgradeResult {
  if (result.status !== "requested") {
    throw new Error("The native bridge returned an unknown upgrade state.");
  }
  return {
    affectedEvidence: [...result.affectedEvidence],
    endpointId: result.endpointId,
    status: "requested",
    version: result.version,
  };
}

function adaptFleetUpgradeResult(
  result: GeneratedFleetUpgradeResult,
): FleetUpgradeResult {
  if (result.status !== "requested") {
    throw new Error(
      "The native bridge returned an unknown Fleet upgrade state.",
    );
  }
  return {
    acceptedEndpoints: result.acceptedEndpoints,
    fleet: result.fleet,
    status: "requested",
    version: result.version,
  };
}

export function createWailsBridge(
  bindings: GeneratedBindings = generatedBindings,
): DesktopBridge {
  return {
    async clearDeploymentToken() {
      await bindings.ClearDeploymentToken();
    },
    async clearEnrollmentToken() {
      await bindings.ClearEnrollmentToken();
    },
    async copyDeploymentToken() {
      await bindings.CopyDeploymentToken();
    },
    async copyEnrollmentToken() {
      await bindings.CopyEnrollmentToken();
    },
    async createEnrollmentToken(request) {
      const result = await bindings.CreateEnrollmentToken({ ...request });

      return {
        expiresAt: result.expiresAt,
        fleet: result.fleet,
        token: result.token,
      };
    },
    async createDeploymentToken(request) {
      const result = await bindings.CreateDeploymentToken({ ...request });
      return {
        metadata: cloneDeploymentToken(result.metadata),
        token: result.token,
      };
    },
    async getApplicationInfo() {
      const info = await bindings.GetApplicationInfo();

      return { name: info.name, version: info.version };
    },
    async getDiagnosticCapabilities() {
      const capabilities = await bindings.GetDiagnosticCapabilities();
      return {
        collectors: [...capabilities.collectors],
        maxTimeSpanSeconds: capabilities.maxTimeSpanSeconds,
      };
    },
    async listDeploymentTokens() {
      const result = await bindings.ListDeploymentTokens();
      return result.map(cloneDeploymentToken);
    },
    async loadAssetInventory() {
      const result = await bindings.LoadAssetInventory();
      return {
        omittedEndpointIds: [...result.omittedEndpointIds],
        rows: result.rows.map((row) => ({ ...row })),
        section: cloneReportSection(result.section),
      };
    },
    async loadAuditExportInfo() {
      const result = await bindings.LoadAuditExportInfo();
      return { exportPath: result.exportPath, pathKey: result.pathKey };
    },
    async loadDiagnosticRequest(requestId) {
      const result = await bindings.LoadDiagnosticRequest(requestId);
      return { ...result, collectors: [...result.collectors] };
    },
    async loadDeploymentToken(label) {
      return cloneDeploymentToken(await bindings.LoadDeploymentToken(label));
    },
    async loadFirewallReport(endpointId) {
      const result = await bindings.LoadFirewallReport(endpointId);
      return {
        audit: result.audit.map((event) => ({
          ...event,
          ports: [...event.ports],
          sources: [...event.sources],
        })),
        endpointId: result.endpointId,
        live: {
          ...result.live,
          zones: result.live.zones.map((zone) => ({
            ...zone,
            ports: [...zone.ports],
            richRules: [...zone.richRules],
            services: [...zone.services],
            sources: [...zone.sources],
          })),
        },
        sections: {
          audit: cloneReportSection(result.sections.audit),
          live: cloneReportSection(result.sections.live),
        },
      };
    },
    async loadFleetOperationalReports(fleet) {
      const result = await bindings.LoadFleetOperationalReports(fleet);
      return {
        fleet: result.fleet,
        schedules: result.schedules.map((schedule) => ({ ...schedule })),
        sections: {
          schedules: cloneReportSection(result.sections.schedules),
          state: cloneReportSection(result.sections.state),
        },
        states: result.states.map((state) => ({
          ...state,
          items: state.items.map((item) => ({
            ...item,
            subresults: item.subresults.map((subresult) => ({ ...subresult })),
          })),
        })),
      };
    },
    async removeEndpointLabel(request) {
      return adaptEndpointLabelResult(
        await bindings.RemoveEndpointLabel({ ...request }),
      );
    },
    async removeEndpoint(request) {
      const result = await bindings.RemoveEndpoint({ ...request });
      if (
        result.status !== "removed" ||
        result.credentialStatus !== "not_enrolled"
      ) {
        throw new Error(
          "The native bridge returned an unknown Endpoint removal state.",
        );
      }
      return {
        affectedEvidence: [...result.affectedEvidence],
        credentialStatus: "not_enrolled",
        endpointId: result.endpointId,
        status: "removed",
      };
    },
    async requestEndpointAgentUpgrade(request) {
      return adaptEndpointUpgradeResult(
        await bindings.RequestEndpointAgentUpgrade({ ...request }),
      );
    },
    async requestDiagnosticCollection(request) {
      const result = await bindings.RequestDiagnosticCollection({
        ...request,
        collectors: [...request.collectors],
      });
      return {
        collectors: [...result.collectors],
        ...(result.createdAt ? { createdAt: result.createdAt } : {}),
        endpointId: result.endpointId,
        ...(result.expiresAt ? { expiresAt: result.expiresAt } : {}),
        requestId: result.requestId,
        since: result.since,
        status: result.status,
        until: result.until,
      };
    },
    async requestFleetAgentUpgrade(request) {
      return adaptFleetUpgradeResult(
        await bindings.RequestFleetAgentUpgrade({ ...request }),
      );
    },
    async requestGitSync() {
      const result = await bindings.RequestGitSync();

      return {
        acceptedAt: result.acceptedAt,
        action: result.action,
        affectedEvidence: [...result.affectedEvidence],
        summary: result.summary,
        target: result.target,
      };
    },
    async revokeDeploymentToken(request) {
      return cloneDeploymentToken(
        await bindings.RevokeDeploymentToken({ ...request }),
      );
    },
    async saveAssetInventory(format) {
      return adaptReadExportSaveResult(
        await bindings.SaveAssetInventory(format),
      );
    },
    async saveDiagnosticBundle(requestId) {
      const result = await bindings.SaveDiagnosticBundle(requestId);
      if (result.status !== "saved" && result.status !== "canceled") {
        throw new Error(
          "The native bridge returned an unknown diagnostic save state.",
        );
      }
      return {
        ...(result.path ? { path: result.path } : {}),
        ...(typeof result.sizeBytes === "number"
          ? { sizeBytes: result.sizeBytes }
          : {}),
        status: result.status,
      };
    },
    async saveDeploymentToken(label) {
      return adaptDeploymentTokenSaveResult(
        await bindings.SaveDeploymentToken(label),
      );
    },
    async saveFirewallReport(request) {
      return adaptReadExportSaveResult(
        await bindings.SaveFirewallReport({ ...request }),
      );
    },
    async setEndpointLabel(request) {
      return adaptEndpointLabelResult(
        await bindings.SetEndpointLabel({ ...request }),
      );
    },
  };
}

const DesktopBridgeContext = createContext<DesktopBridge | undefined>(undefined);

export function BridgeProvider({
  bridge,
  children,
}: PropsWithChildren<{ bridge: DesktopBridge }>) {
  return (
    <DesktopBridgeContext.Provider value={bridge}>
      {children}
    </DesktopBridgeContext.Provider>
  );
}

export function useDesktopBridge(): DesktopBridge {
  const bridge = useContext(DesktopBridgeContext);
  if (bridge === undefined) {
    throw new Error("useDesktopBridge must be used within BridgeProvider");
  }

  return bridge;
}
