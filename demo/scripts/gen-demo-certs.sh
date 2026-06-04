#!/usr/bin/env bash
# Generate demo operator TLS material and refresh POST_v1_admin_bootstrap.json.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STATE_DIR="${ROOT}/demo/operator-state"
FIXTURES="${ROOT}/demo/fixtures/http"
TMP="${ROOT}/demo/.gen-tmp"

rm -rf "$TMP"
mkdir -p "$STATE_DIR" "$FIXTURES" "$TMP"

openssl genrsa -out "$TMP/ca.key" 2048 2>/dev/null
openssl req -x509 -new -nodes -key "$TMP/ca.key" -sha256 -days 3650 \
  -subj "/CN=Remotr Demo CA" -out "$TMP/ca.crt" 2>/dev/null

openssl genrsa -out "$TMP/operator.key" 2048 2>/dev/null
openssl req -new -key "$TMP/operator.key" \
  -subj "/CN=demo-operator" -out "$TMP/operator.csr" 2>/dev/null
openssl x509 -req -in "$TMP/operator.csr" -CA "$TMP/ca.crt" -CAkey "$TMP/ca.key" \
  -CAcreateserial -out "$TMP/operator.crt" -days 3650 -sha256 2>/dev/null

install -m 600 "$TMP/operator.crt" "$STATE_DIR/operator.crt"
install -m 600 "$TMP/operator.key" "$STATE_DIR/operator.key"
install -m 600 "$TMP/ca.crt" "$STATE_DIR/ca.crt"
printf '%s\n' '{"operator_id":"demo-operator"}' >"$STATE_DIR/state.json"
chmod 600 "$STATE_DIR/state.json"

go run -mod=vendor "${ROOT}/demo/scripts/write-bootstrap-fixture.go" \
  "$FIXTURES/POST_v1_admin_bootstrap.json" \
  "$TMP/operator.crt" "$TMP/operator.key" "$TMP/ca.crt"

rm -rf "$TMP"
echo "demo operator state: $STATE_DIR"
echo "bootstrap fixture: $FIXTURES/POST_v1_admin_bootstrap.json"
