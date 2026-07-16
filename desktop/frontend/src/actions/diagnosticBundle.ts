export interface DiagnosticBundleSaveResult {
  path?: string;
  sizeBytes?: number;
  status: "canceled" | "saved";
}
