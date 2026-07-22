import { describe, expect, it } from "vitest";

import { endpointDeliveryStatus } from "./endpointDelivery";

describe("endpointDeliveryStatus", () => {
  it.each([
    ["unmanaged", { unmanaged: true, capabilityBlockedTargetRef: "release-2" }],
    ["capability_blocked", { capabilityBlockedTargetRef: "release-2" }],
    [
      "offered",
      { activeReleaseRef: "release-1", offeredReleaseRef: "release-2" },
    ],
    [
      "current",
      { activeReleaseRef: "release-2", targetReleaseRef: "release-2" },
    ],
    ["not_reported", { activeReleaseRef: "release-1" }],
  ] as const)("returns %s from independent delivery evidence", (want, evidence) => {
    expect(endpointDeliveryStatus(evidence)).toBe(want);
  });
});
