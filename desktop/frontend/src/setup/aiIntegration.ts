export interface AIProjectRootView {
  directoryName: string;
  id: string;
  status: string;
}

export interface AIIntegrationListRequest {
  projectRootId: string;
  scope: "project" | "user";
}

export interface AIIntegrationInstallRequest {
  agent: string;
  projectRootId: string;
  replace: boolean;
  scope: "project" | "user";
}

export interface AIIntegrationUpgradeRequest
  extends AIIntegrationInstallRequest {
  version: string;
}

export interface AIIntegrationView {
  agent: string;
  bundleVersion: string;
  displayName: string;
  guidance: string;
  installed: boolean;
  runtimeAvailable: boolean;
  runtimeStatus: string;
  scope: string;
  source: string;
  sourceVersion: string;
}

export interface AIIntegrationActionResult {
  integration: AIIntegrationView;
  status: string;
}
