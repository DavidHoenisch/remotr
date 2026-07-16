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
      addRBACRule={bridge.addRBACRule}
      activateSecretVersion={bridge.activateSecretVersion}
      authorizeChangeRequest={bridge.authorizeChangeRequest}
      bootstrapProfile={bridge.bootstrapProfile}
      buildLocalPackage={bridge.buildLocalPackage}
      checkDesktopUpdate={bridge.checkDesktopUpdate}
      changeRequestLifecycle={bridge.changeRequestLifecycle}
      chooseBaselineAdoptionPlan={bridge.chooseBaselineAdoptionPlan}
      chooseAppPackageArchive={bridge.chooseAppPackageArchive}
      chooseLocalPackageSource={bridge.chooseLocalPackageSource}
      clearDeploymentToken={bridge.clearDeploymentToken}
      clearEnrollmentToken={bridge.clearEnrollmentToken}
      connectProfile={bridge.connectProfile}
      copyDeploymentToken={bridge.copyDeploymentToken}
      copyEnrollmentToken={bridge.copyEnrollmentToken}
      createDeploymentToken={bridge.createDeploymentToken}
      createEnrollmentToken={bridge.createEnrollmentToken}
      createBaselineAdoption={bridge.createBaselineAdoption}
      createLocalPackage={bridge.createLocalPackage}
      createRBACRole={bridge.createRBACRole}
      deleteAppPackage={bridge.deleteAppPackage}
      deleteRBACRole={bridge.deleteRBACRole}
      getRBACRole={bridge.getRBACRole}
      loadAssetInventory={bridge.loadAssetInventory}
      loadAuditExportInfo={bridge.loadAuditExportInfo}
      loadActivityPage={bridge.loadActivityPage}
      loadChangeRequestDetail={bridge.loadChangeRequestDetail}
      listAppPackages={bridge.listAppPackages}
      listSecretVersions={bridge.listSecretVersions}
      listDeploymentTokens={bridge.listDeploymentTokens}
      listRBACOperators={bridge.listRBACOperators}
      listRBACRoles={bridge.listRBACRoles}
      loadDiagnosticCapabilities={bridge.getDiagnosticCapabilities}
      loadDiagnosticRequest={bridge.loadDiagnosticRequest}
      loadAppPackage={bridge.loadAppPackage}
      loadDeploymentToken={bridge.loadDeploymentToken}
      loadFirewallReport={bridge.loadFirewallReport}
      loadFleetOperationalReports={bridge.loadFleetOperationalReports}
      loadSetupMaintenance={bridge.loadSetupMaintenance}
      openRemotrDocumentation={bridge.openRemotrDocumentation}
      promoteChangeBaseline={bridge.promoteChangeBaseline}
      publishAppPackage={bridge.publishAppPackage}
      removeEndpointLabel={bridge.removeEndpointLabel}
      removeEndpoint={bridge.removeEndpoint}
      removeRBACRule={bridge.removeRBACRule}
      requestDiagnosticCollection={bridge.requestDiagnosticCollection}
      requestEndpointAgentUpgrade={bridge.requestEndpointAgentUpgrade}
      requestFleetAgentUpgrade={bridge.requestFleetAgentUpgrade}
      requestGitSync={bridge.requestGitSync}
      runDesktopDoctor={bridge.runDesktopDoctor}
      revokeDeploymentToken={bridge.revokeDeploymentToken}
      revokeSecretVersion={bridge.revokeSecretVersion}
      saveAssetInventory={bridge.saveAssetInventory}
      saveDiagnosticBundle={bridge.saveDiagnosticBundle}
      saveDeploymentToken={bridge.saveDeploymentToken}
      saveFirewallReport={bridge.saveFirewallReport}
      saveProfile={bridge.saveProfile}
      setEndpointLabel={bridge.setEndpointLabel}
      setOperatorRoles={bridge.setOperatorRoles}
      stampOperatorCredential={bridge.stampOperatorCredential}
      uploadSecretVersion={bridge.uploadSecretVersion}
    />
  </StrictMode>,
);
