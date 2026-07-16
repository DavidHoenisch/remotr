import {
  createContext,
  type PropsWithChildren,
  useContext,
} from "react";

import {
  GetApplicationInfo,
  RequestGitSync,
} from "../../wailsjs/go/main/App";
import type { ActionAcknowledgement } from "../actions/useActionController";

export interface ApplicationInfo {
  name: string;
  version: string;
}

export interface DesktopBridge {
  getApplicationInfo(): Promise<ApplicationInfo>;
  requestGitSync(): Promise<ActionAcknowledgement>;
}

interface GeneratedGitSyncResult {
  acceptedAt: string;
  action: string;
  affectedEvidence: string[];
  summary: string;
  target: string;
}

export interface GeneratedBindings {
  GetApplicationInfo(): Promise<ApplicationInfo>;
  RequestGitSync(): Promise<GeneratedGitSyncResult>;
}

const generatedBindings: GeneratedBindings = {
  GetApplicationInfo,
  RequestGitSync,
};

export function createWailsBridge(
  bindings: GeneratedBindings = generatedBindings,
): DesktopBridge {
  return {
    async getApplicationInfo() {
      const info = await bindings.GetApplicationInfo();

      return { name: info.name, version: info.version };
    },
    async requestGitSync() {
      const result = await bindings.RequestGitSync();

      return {
        acceptedAt: result.acceptedAt,
        action: result.action,
        affectedEvidence: [...result.affectedEvidence],
        summary: result.summary,
        target: result.target,
      };
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
