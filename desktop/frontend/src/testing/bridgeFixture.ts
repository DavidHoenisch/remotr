import type {
  ApplicationInfo,
  DesktopBridge,
} from "../bridge/desktopBridge";
import type { ActionAcknowledgement } from "../actions/useActionController";
import type { EnrollmentTokenResult } from "../actions/enrollmentToken";

const defaultApplicationInfo: ApplicationInfo = {
  name: "Remotr Desktop",
  version: "test",
};

const defaultGitSyncResult: ActionAcknowledgement = {
  acceptedAt: "2032-03-04T05:06:07Z",
  action: "git_sync",
  affectedEvidence: ["release_ref", "activity"],
  summary: "Server accepted Git sync for the test profile.",
  target: "config-repo",
};

const defaultEnrollmentTokenResult: EnrollmentTokenResult = {
  expiresAt: "2032-03-05T05:05:07Z",
  fleet: "production",
  token: "fixture-enrollment-token",
};

export function createBridgeFixture(
  applicationInfo: Partial<ApplicationInfo> = {},
): DesktopBridge {
  const info = { ...defaultApplicationInfo, ...applicationInfo };

  return {
    async clearEnrollmentToken() {},
    async copyEnrollmentToken() {},
    async createEnrollmentToken(request) {
      return {
        ...defaultEnrollmentTokenResult,
        fleet: request.fleet,
      };
    },
    async getApplicationInfo() {
      return { ...info };
    },
    async removeEndpointLabel(request) {
      return {
        effect: "removed",
        endpointId: request.endpointId,
        key: request.key,
        labels: [],
        value: "",
      };
    },
    async requestEndpointAgentUpgrade(request) {
      return {
        affectedEvidence: [
          "desired_agent_version",
          "reported_agent_version",
          "activity",
        ],
        endpointId: request.endpointId,
        status: "requested",
        version: request.version,
      };
    },
    async requestGitSync() {
      return {
        ...defaultGitSyncResult,
        affectedEvidence: [...defaultGitSyncResult.affectedEvidence],
      };
    },
    async setEndpointLabel(request) {
      return {
        effect: "added",
        endpointId: request.endpointId,
        key: request.key,
        labels: [{ key: request.key, value: request.value }],
        value: request.value,
      };
    },
  };
}
