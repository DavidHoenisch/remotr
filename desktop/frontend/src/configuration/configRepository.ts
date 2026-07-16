export interface ConfigWorkingTreeView {
  directoryName: string;
  id: string;
  status: string;
}

export interface ConfigRepositoryInitRequest {
  fleet: string;
  remediationPolicy: "auto" | "report";
}

export interface ConfigRepositoryInitResult {
  fleet: string;
  status: string;
  workingTree: ConfigWorkingTreeView;
}

export interface ConfigValidationFinding {
  message: string;
  path: string;
}

export interface ConfigValidationDiagnostic {
  code: string;
  message: string;
  path: string;
}

export interface ConfigValidationView {
  diagnostics: ConfigValidationDiagnostic[];
  issues: ConfigValidationFinding[];
  ok: string[];
  valid: boolean;
  workingTreeId: string;
}

export interface ConfigFleetDiscoverRequest {
  fleet: string;
  workingTreeId: string;
}

export interface ConfigFleetDiscoveryView {
  applications: string[];
  capabilityRequirements: string[];
  crons: string[];
  diagnostics: ConfigValidationDiagnostic[];
  fleet: string;
  manifest: string;
  modules: string[];
  resourceKinds: string[];
  workingTreeId: string;
}

export interface ConfigRenderRequest {
  scope: "endpoint" | "fleet";
  targetId: string;
  workingTreeId: string;
}

export interface ConfigRenderedArtifactView {
  artifactType: string;
  content: string;
  digest: string;
  targetId: string;
  targetType: string;
}

export interface ConfigRenderView {
  artifacts: ConfigRenderedArtifactView[];
  workingTreeId: string;
}

export interface ConfigRenderSaveRequest {
  artifactType: string;
  digest: string;
  targetId: string;
  targetType: string;
  workingTreeId: string;
}

export interface ConfigRenderSaveResult {
  fileName: string;
  status: string;
}

export interface ConfigHubSnippetView {
  author: string;
  category: string;
  description: string;
  distros: string[];
  featured: boolean;
  id: string;
  tags: string[];
  title: string;
}

export interface ConfigHubImportRequest {
  entryId: string;
  outPath: string;
  workingTreeId: string;
}

export interface ConfigHubImportResult {
  entryId: string;
  outPath: string;
  status: string;
}
