export interface FleetUpgradeRequest {
  fleet: string;
  version: string;
}

export interface FleetUpgradeResult {
  acceptedEndpoints: number;
  fleet: string;
  status: "requested";
  version: string;
}
