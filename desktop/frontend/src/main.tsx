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
      clearEnrollmentToken={bridge.clearEnrollmentToken}
      copyEnrollmentToken={bridge.copyEnrollmentToken}
      createEnrollmentToken={bridge.createEnrollmentToken}
      loadAssetInventory={bridge.loadAssetInventory}
      loadAuditExportInfo={bridge.loadAuditExportInfo}
      loadDiagnosticCapabilities={bridge.getDiagnosticCapabilities}
      loadDiagnosticRequest={bridge.loadDiagnosticRequest}
      loadFirewallReport={bridge.loadFirewallReport}
      loadFleetOperationalReports={bridge.loadFleetOperationalReports}
      removeEndpointLabel={bridge.removeEndpointLabel}
      removeEndpoint={bridge.removeEndpoint}
      requestDiagnosticCollection={bridge.requestDiagnosticCollection}
      requestEndpointAgentUpgrade={bridge.requestEndpointAgentUpgrade}
      requestFleetAgentUpgrade={bridge.requestFleetAgentUpgrade}
      requestGitSync={bridge.requestGitSync}
      saveAssetInventory={bridge.saveAssetInventory}
      saveDiagnosticBundle={bridge.saveDiagnosticBundle}
      saveFirewallReport={bridge.saveFirewallReport}
      setEndpointLabel={bridge.setEndpointLabel}
    />
  </StrictMode>,
);
