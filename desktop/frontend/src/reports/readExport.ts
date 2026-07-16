export interface ReportSectionResult {
  error?: {
    guidance: string;
    kind: string;
    message: string;
  };
  snapshot: {
    failedAt?: string;
    loadedAt?: string;
    observedAt?: string;
  };
  state: string;
}

export interface AssetInventoryRow {
  agentVersion: string;
  cpu: string;
  diskEncryption: string;
  endpointId: string;
  fleet: string;
  kernel: string;
  lastCheckIn: string;
  macAddress: string;
  os: string;
  primaryIp: string;
  ram: string;
  tpm: string;
}

export interface AssetInventoryView {
  omittedEndpointIds: string[];
  rows: AssetInventoryRow[];
  section: ReportSectionResult;
}

export interface StateEvidenceSubresult {
  desiredSummary: string;
  observedSummary: string;
  reasonCode: string;
  status: string;
  target: string;
}

export interface StateEvidenceItem {
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

export interface FleetStateEvidence {
  digest: string;
  endpointId: string;
  items: StateEvidenceItem[];
  releaseRef: string;
  reportedAt: string;
  status: string;
}

export interface FleetScheduleEvidence {
  applicable: boolean;
  endpointId: string;
  lastCompletedAt: string;
  lastMessage: string;
  lastScheduledFor: string;
  lastStatus: string;
  name: string;
  schedule: string;
}

export interface FleetOperationalReportsView {
  fleet: string;
  schedules: FleetScheduleEvidence[];
  sections: {
    schedules: ReportSectionResult;
    state: ReportSectionResult;
  };
  states: FleetStateEvidence[];
}

export interface FirewallAuditEvidence {
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

export interface FirewallZoneEvidence {
  name: string;
  ports: string[];
  richRules: string[];
  services: string[];
  sources: string[];
  target: string;
}

export interface FirewallReportView {
  audit: FirewallAuditEvidence[];
  endpointId: string;
  live: {
    backend: string;
    defaultZone: string;
    ruleset: string;
    rulesetTruncated: boolean;
    zones: FirewallZoneEvidence[];
  };
  sections: {
    audit: ReportSectionResult;
    live: ReportSectionResult;
  };
}

export interface FirewallExportRequest {
  endpointId: string;
  format: "csv" | "json";
}

export interface ReadExportSaveResult {
  path?: string;
  sizeBytes?: number;
  status: "canceled" | "saved";
}

export interface AuditExportInfoView {
  exportPath: string;
  pathKey: string;
}

export interface DiagnosticLifecycleView {
  collectors: string[];
  completedAt: string;
  createdAt: string;
  dispatchedAt: string;
  endpointId: string;
  errorMessage: string;
  expiresAt: string;
  requestId: string;
  requestedBy: string;
  sha256: string;
  since: string;
  sizeBytes: number;
  status: string;
  until: string;
}
