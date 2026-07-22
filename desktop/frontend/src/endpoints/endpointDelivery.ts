export interface EndpointDeliveryEvidence {
  activeReleaseRef?: string;
  capabilityBlockedTargetRef?: string;
  offeredReleaseRef?: string;
  releaseRef?: string;
  targetReleaseRef?: string;
  unmanaged?: boolean;
}

export type EndpointDeliveryStatus =
  | "capability_blocked"
  | "current"
  | "not_reported"
  | "offered"
  | "unmanaged";

export function endpointDeliveryStatus(
  endpoint: EndpointDeliveryEvidence,
): EndpointDeliveryStatus {
  if (endpoint.unmanaged) {
    return "unmanaged";
  }
  if (endpoint.capabilityBlockedTargetRef) {
    return "capability_blocked";
  }
  const active = endpoint.activeReleaseRef || endpoint.releaseRef;
  if (endpoint.offeredReleaseRef && endpoint.offeredReleaseRef !== active) {
    return "offered";
  }
  if (endpoint.targetReleaseRef && active === endpoint.targetReleaseRef) {
    return "current";
  }
  return "not_reported";
}
