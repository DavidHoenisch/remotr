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
    async activateSecretVersion() {
      throw new Error("No Secret fixture was configured.");
    },
    async buildLocalPackage() {
      throw new Error("No local package fixture was configured.");
    },
    async authorizeChangeRequest() {
      throw new Error("No Change-control fixture was configured.");
    },
    async changeRequestLifecycle() {
      throw new Error("No Change-control fixture was configured.");
    },
    async chooseBaselineAdoptionPlan() {
      throw new Error("No baseline-adoption fixture was configured.");
    },
    async chooseAppPackageArchive() {
      throw new Error("No application package fixture was configured.");
    },
    async chooseLocalPackageSource() {
      throw new Error("No local package fixture was configured.");
    },
    async clearDeploymentToken() {},
    async clearEnrollmentToken() {},
    async copyDeploymentToken() {},
    async copyEnrollmentToken() {},
    async createEnrollmentToken(request) {
      return {
        ...defaultEnrollmentTokenResult,
        fleet: request.fleet,
      };
    },
    async createDeploymentToken(request) {
      return {
        metadata: {
          createdAt: "",
          expiresAt: "2032-03-05T05:05:07Z",
          fleet: request.fleet,
          id: "",
          label: request.label,
          lastUsedAt: "",
          revokedAt: "",
          status: "active",
        },
        token: "fixture-deployment-token",
      };
    },
    async createBaselineAdoption() {
      throw new Error("No baseline-adoption fixture was configured.");
    },
    async createLocalPackage() {
      throw new Error("No local package fixture was configured.");
    },
    async deleteAppPackage() {
      throw new Error("No application package fixture was configured.");
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
    async listDeploymentTokens() {
      return [];
    },
    async listAppPackages() {
      return [];
    },
    async listSecretVersions() {
      return [];
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
    async loadActivityPage() {
      return {
        events: [],
        nextCursor: "",
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
    async loadChangeRequestDetail() {
      throw new Error("No Change request detail fixture was configured.");
    },
    async loadAppPackage() {
      throw new Error("No application package fixture was configured.");
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
    async loadDeploymentToken(label) {
      return {
        createdAt: "2032-03-01T05:05:07Z",
        expiresAt: "2032-03-05T05:05:07Z",
        fleet: "production",
        id: `deployment-${label}`,
        label,
        lastUsedAt: "",
        revokedAt: "",
        status: "active",
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
    async promoteChangeBaseline() {
      throw new Error("No baseline-promotion fixture was configured.");
    },
    async publishAppPackage() {
      throw new Error("No application package fixture was configured.");
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
    async revokeDeploymentToken(request) {
      return {
        createdAt: "2032-03-01T05:05:07Z",
        expiresAt: "2032-03-05T05:05:07Z",
        fleet: "production",
        id: `deployment-${request.label}`,
        label: request.label,
        lastUsedAt: "",
        revokedAt: "2032-03-04T05:05:07Z",
        status: "revoked",
      };
    },
    async revokeSecretVersion() {
      throw new Error("No Secret fixture was configured.");
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
    async saveDeploymentToken(label) {
      return {
        path: `/tmp/${label}.token`,
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
    async uploadSecretVersion() {
      throw new Error("No Secret fixture was configured.");
    },
  };
}
