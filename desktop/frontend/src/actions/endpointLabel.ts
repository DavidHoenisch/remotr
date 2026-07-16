export interface EndpointLabelView {
  key: string;
  value: string;
}

export interface EndpointLabelSetRequest {
  endpointId: string;
  key: string;
  value: string;
}

export interface EndpointLabelRemoveRequest {
  endpointId: string;
  key: string;
}

export interface EndpointLabelResult {
  effect: "added" | "removed" | "replaced";
  endpointId: string;
  key: string;
  value: string;
  labels: EndpointLabelView[];
}

function runeLength(value: string): number {
  return Array.from(value).length;
}

export function validEndpointLabelKey(key: string): boolean {
  const trimmed = key.trim();
  return (
    trimmed.length > 0 &&
    runeLength(key) <= 64 &&
    !key.startsWith(".") &&
    !/[ =\n\r\t]/u.test(key)
  );
}

export function validEndpointLabelValue(value: string): boolean {
  return runeLength(value) <= 512;
}
