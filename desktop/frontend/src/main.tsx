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
      activateSecretVersion={bridge.activateSecretVersion}
      authorizeChangeRequest={bridge.authorizeChangeRequest}
      buildLocalPackage={bridge.buildLocalPackage}
      changeRequestLifecycle={bridge.changeRequestLifecycle}
      chooseBaselineAdoptionPlan={bridge.chooseBaselineAdoptionPlan}
      chooseAppPackageArchive={bridge.chooseAppPackageArchive}
      chooseLocalPackageSource={bridge.chooseLocalPackageSource}
      clearDeploymentToken={bridge.clearDeploymentToken}
      clearEnrollmentToken={bridge.clearEnrollmentToken}
      copyDeploymentToken={bridge.copyDeploymentToken}
      copyEnrollmentToken={bridge.copyEnrollmentToken}
      createDeploymentToken={bridge.createDeploymentToken}
      createEnrollmentToken={bridge.createEnrollmentToken}
      createBaselineAdoption={bridge.createBaselineAdoption}
      createLocalPackage={bridge.createLocalPackage}
      deleteAppPackage={bridge.deleteAppPackage}
      loadAssetInventory={bridge.loadAssetInventory}
      loadAuditExportInfo={bridge.loadAuditExportInfo}
      loadActivityPage={bridge.loadActivityPage}
      loadChangeRequestDetail={bridge.loadChangeRequestDetail}
      listAppPackages={bridge.listAppPackages}
      listSecretVersions={bridge.listSecretVersions}
      listDeploymentTokens={bridge.listDeploymentTokens}
      loadDiagnosticCapabilities={bridge.getDiagnosticCapabilities}
      loadDiagnosticRequest={bridge.loadDiagnosticRequest}
      loadAppPackage={bridge.loadAppPackage}
      loadDeploymentToken={bridge.loadDeploymentToken}
      loadFirewallReport={bridge.loadFirewallReport}
      loadFleetOperationalReports={bridge.loadFleetOperationalReports}
      promoteChangeBaseline={bridge.promoteChangeBaseline}
      publishAppPackage={bridge.publishAppPackage}
      removeEndpointLabel={bridge.removeEndpointLabel}
      removeEndpoint={bridge.removeEndpoint}
      requestDiagnosticCollection={bridge.requestDiagnosticCollection}
      requestEndpointAgentUpgrade={bridge.requestEndpointAgentUpgrade}
      requestFleetAgentUpgrade={bridge.requestFleetAgentUpgrade}
      requestGitSync={bridge.requestGitSync}
      revokeDeploymentToken={bridge.revokeDeploymentToken}
      revokeSecretVersion={bridge.revokeSecretVersion}
      saveAssetInventory={bridge.saveAssetInventory}
      saveDiagnosticBundle={bridge.saveDiagnosticBundle}
      saveDeploymentToken={bridge.saveDeploymentToken}
      saveFirewallReport={bridge.saveFirewallReport}
      setEndpointLabel={bridge.setEndpointLabel}
      uploadSecretVersion={bridge.uploadSecretVersion}
    />
  </StrictMode>,
);
