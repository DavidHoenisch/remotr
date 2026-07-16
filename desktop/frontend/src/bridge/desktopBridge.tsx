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
  RemoveEndpointLabel,
  RequestEndpointAgentUpgrade,
  RequestFleetAgentUpgrade,
  RequestGitSync,
  SetEndpointLabel,
} from "../../wailsjs/go/main/App";
import type { ActionAcknowledgement } from "../actions/useActionController";
import type {
  EnrollmentTokenRequest,
  EnrollmentTokenResult,
} from "../actions/enrollmentToken";
import type {
  EndpointLabelRemoveRequest,
  EndpointLabelResult,
  EndpointLabelSetRequest,
} from "../actions/endpointLabel";
import type {
  EndpointUpgradeRequest,
  EndpointUpgradeResult,
} from "../actions/endpointUpgrade";
import type {
  FleetUpgradeRequest,
  FleetUpgradeResult,
} from "../actions/fleetUpgrade";

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
  removeEndpointLabel(
    request: EndpointLabelRemoveRequest,
  ): Promise<EndpointLabelResult>;
  requestEndpointAgentUpgrade(
    request: EndpointUpgradeRequest,
  ): Promise<EndpointUpgradeResult>;
  requestFleetAgentUpgrade(
    request: FleetUpgradeRequest,
  ): Promise<FleetUpgradeResult>;
  requestGitSync(): Promise<ActionAcknowledgement>;
  setEndpointLabel(
    request: EndpointLabelSetRequest,
  ): Promise<EndpointLabelResult>;
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

interface GeneratedEndpointLabelResult {
  effect: string;
  endpointId: string;
  key: string;
  value: string;
  labels: Array<{ key: string; value: string }>;
}

interface GeneratedEndpointUpgradeResult {
  affectedEvidence: string[];
  endpointId: string;
  status: string;
  version: string;
}

interface GeneratedFleetUpgradeResult {
  acceptedEndpoints: number;
  fleet: string;
  status: string;
  version: string;
}

function adaptEndpointLabelEffect(
  effect: string,
): EndpointLabelResult["effect"] {
  if (effect === "added" || effect === "removed" || effect === "replaced") {
    return effect;
  }
  throw new Error("The native bridge returned an unknown Label effect.");
}

export interface GeneratedBindings {
  ClearEnrollmentToken(): Promise<void>;
  CopyEnrollmentToken(): Promise<void>;
  CreateEnrollmentToken(
    request: EnrollmentTokenRequest,
  ): Promise<GeneratedEnrollmentTokenResult>;
  GetApplicationInfo(): Promise<ApplicationInfo>;
  RemoveEndpointLabel(
    request: EndpointLabelRemoveRequest,
  ): Promise<GeneratedEndpointLabelResult>;
  RequestEndpointAgentUpgrade(
    request: EndpointUpgradeRequest,
  ): Promise<GeneratedEndpointUpgradeResult>;
  RequestFleetAgentUpgrade(
    request: FleetUpgradeRequest,
  ): Promise<GeneratedFleetUpgradeResult>;
  RequestGitSync(): Promise<GeneratedGitSyncResult>;
  SetEndpointLabel(
    request: EndpointLabelSetRequest,
  ): Promise<GeneratedEndpointLabelResult>;
}

const generatedBindings: GeneratedBindings = {
  ClearEnrollmentToken,
  CopyEnrollmentToken,
  CreateEnrollmentToken,
  GetApplicationInfo,
  RemoveEndpointLabel,
  RequestEndpointAgentUpgrade,
  RequestFleetAgentUpgrade,
  RequestGitSync,
  SetEndpointLabel,
};

function adaptEndpointLabelResult(
  result: GeneratedEndpointLabelResult,
): EndpointLabelResult {
  return {
    effect: adaptEndpointLabelEffect(result.effect),
    endpointId: result.endpointId,
    key: result.key,
    labels: result.labels.map((label) => ({ ...label })),
    value: result.value,
  };
}

function adaptEndpointUpgradeResult(
  result: GeneratedEndpointUpgradeResult,
): EndpointUpgradeResult {
  if (result.status !== "requested") {
    throw new Error("The native bridge returned an unknown upgrade state.");
  }
  return {
    affectedEvidence: [...result.affectedEvidence],
    endpointId: result.endpointId,
    status: "requested",
    version: result.version,
  };
}

function adaptFleetUpgradeResult(
  result: GeneratedFleetUpgradeResult,
): FleetUpgradeResult {
  if (result.status !== "requested") {
    throw new Error(
      "The native bridge returned an unknown Fleet upgrade state.",
    );
  }
  return {
    acceptedEndpoints: result.acceptedEndpoints,
    fleet: result.fleet,
    status: "requested",
    version: result.version,
  };
}

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
    async removeEndpointLabel(request) {
      return adaptEndpointLabelResult(
        await bindings.RemoveEndpointLabel({ ...request }),
      );
    },
    async requestEndpointAgentUpgrade(request) {
      return adaptEndpointUpgradeResult(
        await bindings.RequestEndpointAgentUpgrade({ ...request }),
      );
    },
    async requestFleetAgentUpgrade(request) {
      return adaptFleetUpgradeResult(
        await bindings.RequestFleetAgentUpgrade({ ...request }),
      );
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
    async setEndpointLabel(request) {
      return adaptEndpointLabelResult(
        await bindings.SetEndpointLabel({ ...request }),
      );
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
