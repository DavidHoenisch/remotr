export interface EndpointRemovalRequest {
  confirmation: string;
  endpointId: string;
}

export interface EndpointRemovalResult {
  affectedEvidence: string[];
  credentialStatus: "not_enrolled";
  endpointId: string;
  status: "removed";
}
