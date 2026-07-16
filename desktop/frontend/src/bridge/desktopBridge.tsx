import {
  createContext,
  type PropsWithChildren,
  useContext,
} from "react";

import { GetApplicationInfo } from "../../wailsjs/go/main/App";

export interface ApplicationInfo {
  name: string;
  version: string;
}

export interface DesktopBridge {
  getApplicationInfo(): Promise<ApplicationInfo>;
}

export interface GeneratedBindings {
  GetApplicationInfo(): Promise<ApplicationInfo>;
}

const generatedBindings: GeneratedBindings = { GetApplicationInfo };

export function createWailsBridge(
  bindings: GeneratedBindings = generatedBindings,
): DesktopBridge {
  return {
    async getApplicationInfo() {
      const info = await bindings.GetApplicationInfo();

      return { name: info.name, version: info.version };
    },
  };
}

const DesktopBridgeContext = createContext<DesktopBridge | undefined>(undefined);

export function BridgeProvider({
  bridge,
  children,
}: PropsWithChildren<{ bridge: DesktopBridge }>) {
  return (
    <DesktopBridgeContext.Provider value={bridge}>
      {children}
    </DesktopBridgeContext.Provider>
  );
}

export function useDesktopBridge(): DesktopBridge {
  const bridge = useContext(DesktopBridgeContext);
  if (bridge === undefined) {
    throw new Error("useDesktopBridge must be used within BridgeProvider");
  }

  return bridge;
}
