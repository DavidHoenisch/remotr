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
    async getDiagnosticCapabilities() {
      return {
        collectors: ["system_info", "network_state"],
        maxTimeSpanSeconds: 7 * 24 * 60 * 60,
      };
    },
    async loadAssetInventory() {
      return {
        omittedEndpointIds: [],
        rows: [],
        section: {
          snapshot: { loadedAt: "2032-03-04T05:06:07Z" },
          state: "empty",
        },
      };
    },
    async loadAuditExportInfo() {
      return {
        exportPath: "/v1/admin/audit-export/events",
        pathKey: "siem-v1",
      };
    },
    async loadDiagnosticRequest(requestId) {
      return {
        collectors: ["system_info"],
        completedAt: "",
        createdAt: "2032-03-04T05:06:07Z",
        dispatchedAt: "",
        endpointId: "endpoint-alpha",
        errorMessage: "",
        expiresAt: "2032-03-05T05:06:07Z",
        requestId,
        requestedBy: "operator-fixture",
        sha256: "",
        since: "2032-03-03T05:06:07Z",
        sizeBytes: 0,
        status: "pending",
        until: "2032-03-04T05:06:07Z",
      };
    },
    async loadFirewallReport(endpointId) {
      const section = {
        snapshot: { loadedAt: "2032-03-04T05:06:07Z" },
        state: "empty",
      };
      return {
        audit: [],
        endpointId,
        live: {
          backend: "",
          defaultZone: "",
          ruleset: "",
          rulesetTruncated: false,
          zones: [],
        },
        sections: { audit: section, live: section },
      };
    },
    async loadFleetOperationalReports(fleet) {
      const section = {
        snapshot: { loadedAt: "2032-03-04T05:06:07Z" },
        state: "empty",
      };
      return {
        fleet,
        schedules: [],
        sections: { schedules: section, state: section },
        states: [],
      };
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
    async removeEndpoint(request) {
      return {
        affectedEvidence: ["inventory", "activity"],
        credentialStatus: "not_enrolled",
        endpointId: request.endpointId,
        status: "removed",
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
    async requestDiagnosticCollection(request) {
      return {
        ...request,
        collectors: [...request.collectors],
        requestId: "diagnostic-fixture",
        status: "pending",
      };
    },
    async requestFleetAgentUpgrade(request) {
      return {
        acceptedEndpoints: 1,
        fleet: request.fleet,
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
    async saveAssetInventory(format) {
      return {
        path: `/tmp/remotr-inventory.${format}`,
        sizeBytes: 128,
        status: "saved",
      };
    },
    async saveDiagnosticBundle(requestId) {
      return {
        path: `/tmp/${requestId}.tar.gz`,
        sizeBytes: 128,
        status: "saved",
      };
    },
    async saveFirewallReport() {
      return { status: "canceled" };
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
