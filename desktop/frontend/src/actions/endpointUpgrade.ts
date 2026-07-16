export interface EndpointUpgradeRequest {
  endpointId: string;
  version: string;
}

export interface EndpointUpgradeResult {
  affectedEvidence: string[];
  endpointId: string;
  status: "requested";
  version: string;
}

export interface EndpointUpgradeEvidence {
  desiredAgentVersion: string;
  reportedAgentVersion: string;
}

const semanticVersion = /^(?:v)?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/u;

export function validAgentVersion(version: string): boolean {
  return semanticVersion.test(version);
}
