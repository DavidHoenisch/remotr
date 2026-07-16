export interface ConnectionProfile {
  caPath: string;
  defaultFleet: string;
  name: string;
  serverUrl: string;
  stateDir: string;
}

export interface ConnectionView {
  operatorId: string;
  profileName: string;
  roles: string[];
  serverUrl: string;
}

export interface SetupApplicationView {
  architecture: string;
  name: string;
  platform: string;
  version: string;
}

export interface SetupMaintenanceView {
  application: SetupApplicationView;
  desktopProfilesPath: string;
  profiles: ConnectionProfile[];
  standardConfigPath: string;
}

export interface DesktopDoctorCheck {
  detail: string;
  guidance: string;
  name: string;
  status: "fail" | "ok" | "warn";
}

export interface DesktopDoctorReport {
  checks: DesktopDoctorCheck[];
  healthy: boolean;
  operatorId: string;
  profileName: string;
  roles: string[];
}

export interface DesktopUpdateStatus {
  currentVersion: string;
  guidance: string;
  installSupported: boolean;
  latestVersion: string;
  platform: string;
  updateAvailable: boolean;
}
