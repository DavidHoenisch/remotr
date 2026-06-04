#!/usr/bin/env bash
# Reset recording state for bootstrap VHS (config only, no operator credentials).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RECORD_STATE="${ROOT}/demo/record/state"
RECORD_CONFIG="${ROOT}/demo/record/config"
CA="${ROOT}/demo/operator-state/ca.crt"

rm -rf "$RECORD_STATE" "$RECORD_CONFIG"
mkdir -p "$RECORD_STATE" "$RECORD_CONFIG"
install -m 600 "$CA" "$RECORD_STATE/ca.crt"

cat >"$RECORD_CONFIG/config.yaml" <<EOF
server_url: https://demo.remotr.example:8443
state_dir: ${RECORD_STATE}
ca: ${RECORD_STATE}/ca.crt
fleet: engineering
EOF
chmod 600 "$RECORD_CONFIG/config.yaml"
