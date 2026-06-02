#!/bin/sh
set -eu

SERVER_URL="${REMOTR_SERVER_URL:-https://remotr-server:8443}"
CA_PATH="${REMOTR_TLS_CA:-/certs/ca.crt}"
STATE_DIR="${REMOTR_STATE_DIR:-/var/lib/remotr}"

wait_for_server() {
  until wget -qO- --no-check-certificate "${SERVER_URL}/healthz" >/dev/null 2>&1; do
    sleep 1
  done
}

read_enroll_token() {
  if [ -n "${REMOTR_ENROLL_TOKEN:-}" ]; then
    printf '%s' "$REMOTR_ENROLL_TOKEN"
    return
  fi
  if [ -n "${REMOTR_ENROLL_TOKEN_FILE:-}" ] && [ -f "$REMOTR_ENROLL_TOKEN_FILE" ]; then
    tr -d ' \n\r' < "$REMOTR_ENROLL_TOKEN_FILE"
    return
  fi
  echo "agent entrypoint: enrollment token required (REMOTR_ENROLL_TOKEN or REMOTR_ENROLL_TOKEN_FILE)" >&2
  exit 1
}

wait_for_server

if [ ! -f "${STATE_DIR}/state.json" ]; then
  TOKEN="$(read_enroll_token)"
  remotr-agent enroll \
    --server-url "$SERVER_URL" \
    --ca "$CA_PATH" \
    --state-dir "$STATE_DIR" \
    --token "$TOKEN" \
    --no-sync
fi

# Compose dev only: host-side e2e tests read credentials from the bind mount.
if [ "${REMOTR_COMPOSE_E2E:-}" = "1" ] && [ -f "${STATE_DIR}/state.json" ]; then
  chmod a+rx "${STATE_DIR}"
  chmod a+r "${STATE_DIR}/agent.crt" "${STATE_DIR}/ca.crt" "${STATE_DIR}/state.json" 2>/dev/null || true
  chmod a+r "${STATE_DIR}/agent.key" 2>/dev/null || true
fi

exec remotr-agent
