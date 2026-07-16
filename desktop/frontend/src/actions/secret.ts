export interface SecretUploadRequest {
  name: string;
  scopeId: string;
  scopeType: "fleet" | "endpoint";
}

export interface SecretLifecycleRequest {
  confirmation: string;
  name: string;
  version: string;
}

export interface SecretRolloutView {
  changeRequestId: string;
  effectiveHash: string;
  fleet: string;
  purpose: string;
  resourceAddress: string;
  risk: string;
}

export interface SecretVersionView {
  activatedAt: string;
  activatedBy: string;
  activationGeneration: number;
  createdAt: string;
  createdBy: string;
  endpointCopyStatus: string;
  fingerprint: string;
  name: string;
  resolutionBlocked: boolean;
  revokedAt: string;
  revokedBy: string;
  rollouts: SecretRolloutView[];
  scopeId: string;
  scopeType: "fleet" | "endpoint";
  status: "inactive" | "active" | "activation_planned" | "revoked";
  version: string;
}

