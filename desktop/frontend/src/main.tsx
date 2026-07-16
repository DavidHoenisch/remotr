import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { createWailsBridge } from "./bridge/desktopBridge";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Remotr desktop root element is missing");
}

const bridge = createWailsBridge();

createRoot(root).render(
  <StrictMode>
    <App
      clearDeploymentToken={bridge.clearDeploymentToken}
      clearEnrollmentToken={bridge.clearEnrollmentToken}
      copyDeploymentToken={bridge.copyDeploymentToken}
      copyEnrollmentToken={bridge.copyEnrollmentToken}
      createDeploymentToken={bridge.createDeploymentToken}
      createEnrollmentToken={bridge.createEnrollmentToken}
      loadAssetInventory={bridge.loadAssetInventory}
      loadAuditExportInfo={bridge.loadAuditExportInfo}
      listDeploymentTokens={bridge.listDeploymentTokens}
      loadDiagnosticCapabilities={bridge.getDiagnosticCapabilities}
      loadDiagnosticRequest={bridge.loadDiagnosticRequest}
      loadDeploymentToken={bridge.loadDeploymentToken}
      loadFirewallReport={bridge.loadFirewallReport}
      loadFleetOperationalReports={bridge.loadFleetOperationalReports}
      removeEndpointLabel={bridge.removeEndpointLabel}
      removeEndpoint={bridge.removeEndpoint}
      requestDiagnosticCollection={bridge.requestDiagnosticCollection}
      requestEndpointAgentUpgrade={bridge.requestEndpointAgentUpgrade}
      requestFleetAgentUpgrade={bridge.requestFleetAgentUpgrade}
      requestGitSync={bridge.requestGitSync}
      revokeDeploymentToken={bridge.revokeDeploymentToken}
      saveAssetInventory={bridge.saveAssetInventory}
      saveDiagnosticBundle={bridge.saveDiagnosticBundle}
      saveDeploymentToken={bridge.saveDeploymentToken}
      saveFirewallReport={bridge.saveFirewallReport}
      setEndpointLabel={bridge.setEndpointLabel}
    />
  </StrictMode>,
);
