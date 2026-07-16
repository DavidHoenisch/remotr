import {
  createContext,
  type PropsWithChildren,
  useContext,
} from "react";

import {
  ClearEnrollmentToken,
  CopyEnrollmentToken,
  CreateEnrollmentToken,
  GetApplicationInfo,
  RequestGitSync,
} from "../../wailsjs/go/main/App";
import type { ActionAcknowledgement } from "../actions/useActionController";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "../actions/enrollmentToken";

export interface ApplicationInfo {
  name: string;
  version: string;
}

export interface DesktopBridge {
  clearEnrollmentToken(): Promise<void>;
  copyEnrollmentToken(): Promise<void>;
  createEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<EnrollmentTokenResult>;
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

interface GeneratedEnrollmentTokenResult {
  expiresAt: string;
  fleet: string;
  token: string;
}

export interface GeneratedBindings {
  ClearEnrollmentToken(): Promise<void>;
  CopyEnrollmentToken(): Promise<void>;
  CreateEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<GeneratedEnrollmentTokenResult>;
  GetApplicationInfo(): Promise<ApplicationInfo>;
  RequestGitSync(): Promise<GeneratedGitSyncResult>;
}

const generatedBindings: GeneratedBindings = {
  ClearEnrollmentToken,
  CopyEnrollmentToken,
  CreateEnrollmentToken,
  GetApplicationInfo,
  RequestGitSync,
};

export function createWailsBridge(
  bindings: GeneratedBindings = generatedBindings,
): DesktopBridge {
  return {
    async clearEnrollmentToken() {
      await bindings.ClearEnrollmentToken();
    },
    async copyEnrollmentToken() {
      await bindings.CopyEnrollmentToken();
    },
    async createEnrollmentToken(request) {
      const result = await bindings.CreateEnrollmentToken({ ...request });

      return {
        expiresAt: result.expiresAt,
        fleet: result.fleet,
        token: result.token,
      };
    },
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
