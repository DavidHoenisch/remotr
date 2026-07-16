export interface AppPackageArchiveView {
  fileName: string;
  mode: string;
  name: string;
  sha256: string;
  sizeBytes: number;
  source: "built" | "selected";
  version: string;
}

export interface AppPackageView {
  createdAt: string;
  id: string;
  installMode: string;
  name: string;
  objectKey: string;
  sha256: string;
  version: string;
}

export interface LocalPackageCreateRequest {
  directoryName: string;
  mode: "binary" | "build" | "script";
  name: string;
  version: string;
}

export interface LocalPackageView {
  locationName: string;
  mode: string;
  name: string;
  version: string;
}

export interface AppPackagePublishRequest {
  confirmation: string;
  name: string;
  sha256: string;
  version: string;
}

export interface AppPackageDeleteRequest {
  confirmation: string;
  deleteObject: boolean;
  name: string;
  version: string;
}

export interface AppPackageDeleteResult {
  name: string;
  scope: "catalog_and_object" | "catalog_only";
  version: string;
}
