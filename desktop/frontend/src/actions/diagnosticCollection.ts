export interface DiagnosticCapabilities {
  collectors: string[];
  maxTimeSpanSeconds: number;
}

export interface DiagnosticCollectionRequest {
  collectors: string[];
  endpointId: string;
  since: string;
  until: string;
}

export interface DiagnosticCollectionResult
  extends DiagnosticCollectionRequest {
  createdAt?: string;
  expiresAt?: string;
  requestId: string;
  status: string;
}
