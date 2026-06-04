#!/usr/bin/env bash
# Reset demo recording state (operator credentials + config) for VHS tapes.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RECORD_STATE="${ROOT}/demo/record/state"
RECORD_CONFIG="${ROOT}/demo/record/config"
OPERATOR_STATE="${ROOT}/demo/operator-state"

rm -rf "$RECORD_STATE" "$RECORD_CONFIG"
mkdir -p "$RECORD_STATE" "$RECORD_CONFIG"

cp -a "$OPERATOR_STATE/." "$RECORD_STATE/"

cat >"$RECORD_CONFIG/config.yaml" <<EOF
server_url: https://demo.remotr.example:8443
state_dir: ${RECORD_STATE}
ca: ${RECORD_STATE}/ca.crt
fleet: engineering
EOF
chmod 600 "$RECORD_CONFIG/config.yaml"
