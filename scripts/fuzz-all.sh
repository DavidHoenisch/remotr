#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

fuzztime="${1:-30s}"

targets=(
  "./internal/models:FuzzParseState"
  "./internal/identity:FuzzEndpointIDFromCert"
  "./internal/identity:FuzzFingerprintFromCertRoundTrip"
  "./internal/pki:FuzzIssueEndpointCredential"
  "./internal/configrepo:FuzzFleetArtifact"
  "./internal/configrepo:FuzzFleetArtifactPathTraversal"
  "./internal/store/postgres:FuzzUUIDFromString"
  "./internal/server:FuzzHandleSync"
  "./internal/server:FuzzHandleEnroll"
)

for entry in "${targets[@]}"; do
  pkg="${entry%%:*}"
  name="${entry##*:}"
  echo "==> fuzz ${name} (${pkg}, ${fuzztime})"
  go test -mod=vendor "${pkg}" -fuzz="^${name}\$" -fuzztime="${fuzztime}" -count=1
done

echo "all fuzz targets completed (${fuzztime} each)"
