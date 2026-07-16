import type {
  ApplicationInfo,
  DesktopBridge,
} from "../bridge/desktopBridge";
import type { ActionAcknowledgement } from "../actions/useActionController";

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

export function createBridgeFixture(
  applicationInfo: Partial<ApplicationInfo> = {},
): DesktopBridge {
  const info = { ...defaultApplicationInfo, ...applicationInfo };

  return {
    async getApplicationInfo() {
      return { ...info };
    },
    async requestGitSync() {
      return {
        ...defaultGitSyncResult,
        affectedEvidence: [...defaultGitSyncResult.affectedEvidence],
      };
    },
  };
}
