import type {
  ApplicationInfo,
  DesktopBridge,
} from "../bridge/desktopBridge";

const defaultApplicationInfo: ApplicationInfo = {
  name: "Remotr Desktop",
  version: "test",
};

export function createBridgeFixture(
  applicationInfo: Partial<ApplicationInfo> = {},
): DesktopBridge {
  const info = { ...defaultApplicationInfo, ...applicationInfo };

  return {
    async getApplicationInfo() {
      return { ...info };
    },
  };
}
