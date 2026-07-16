import type { ChangeRequestDetailView } from "./ChangeRequestDetail";

export interface ChangeExecutionWindowInput {
  durationMinutes: number;
  startMinuteUtc: number;
  weekdays: number[];
}

export interface ChangeAuthorizationRequest {
  attemptLimit: number;
  changeRequestId: string;
  confirmation: string;
  executionWindows: ChangeExecutionWindowInput[];
  justification: string;
  maxConcurrency: number;
  validFrom: string;
  validUntil: string;
}

export interface ChangeLifecycleRequest {
  action: "pause" | "resume" | "revoke";
  changeRequestId: string;
  confirmation: string;
}

export interface ChangeBaselinePromotionRequest {
  acknowledgeExceptions: boolean;
  changeRequestId: string;
  confirmation: string;
  resourceAddress: string;
}

export interface BaselineAdoptionRequest {
  confirmation: string;
  fleet: string;
  planId: string;
}

export interface BaselineAdoptionPreview {
  artifactDigest: string;
  fleet: string;
  planId: string;
  releaseRef: string;
  resourceAddresses: string[];
  resourceCount: number;
  targetCount: number;
}

export interface RolloutAuthorizationView {
  attemptLimit: number;
  authorizedAt: string;
  authorizedBy: string;
  changeRequestId: string;
  executionWindows: ChangeExecutionWindowInput[];
  fleet: string;
  id: string;
  justification: string;
  maxConcurrency: number;
  validFrom: string;
  validUntil: string;
}

export interface BaselineAuthorizationView {
  authorizedAt: string;
  authorizedBy: string;
  changeRequestId: string;
  desiredHash: string;
  fleet: string;
  id: string;
  provider: string;
  resourceAddress: string;
  risk: string;
}

export interface ChangeActionResult {
  action: string;
  affectedEvidence: string[];
  authorization?: RolloutAuthorizationView;
  baseline?: BaselineAuthorizationView;
  changeRequest: ChangeRequestDetailView;
}

export interface ChangeControlBindings {
  authorizeChangeRequest: (
    request: ChangeAuthorizationRequest,
  ) => Promise<ChangeActionResult>;
  changeRequestLifecycle: (
    request: ChangeLifecycleRequest,
  ) => Promise<ChangeActionResult>;
  chooseBaselineAdoptionPlan: (
    fleet: string,
  ) => Promise<BaselineAdoptionPreview>;
  createBaselineAdoption: (
    request: BaselineAdoptionRequest,
  ) => Promise<ChangeActionResult>;
  promoteChangeBaseline: (
    request: ChangeBaselinePromotionRequest,
  ) => Promise<ChangeActionResult>;
}
