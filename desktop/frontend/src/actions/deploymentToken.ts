export interface DeploymentTokenView {
  createdAt: string;
  expiresAt: string;
  fleet: string;
  id: string;
  label: string;
  lastUsedAt: string;
  revokedAt: string;
  status: string;
}

export interface DeploymentTokenCreateRequest {
  fleet: string;
  label: string;
  ttlSeconds: number;
}

export interface DeploymentTokenCreateResult {
  metadata: DeploymentTokenView;
  token: string;
}

export interface DeploymentTokenRevokeRequest {
  confirmation: string;
  label: string;
}

export interface DeploymentTokenSaveResult {
  path?: string;
  sizeBytes?: number;
  status: "canceled" | "saved";
}
