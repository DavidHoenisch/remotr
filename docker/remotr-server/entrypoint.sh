#!/bin/sh
set -eu

# Fly.io (and similar) inject PEM material via secrets env vars. remotr-server
# expects absolute file paths — materialize inline PEM to /run/remotr/certs first.
CERT_DIR=/run/remotr/certs

is_pem() {
  case "$1" in
    -----BEGIN*) return 0 ;;
    *) return 1 ;;
  esac
}

write_pem() {
  var=$1
  file=$2
  mode=$3
  eval "val=\${$var:-}"
  if [ -z "$val" ] || ! is_pem "$val"; then
    return 0
  fi
  mkdir -p "$CERT_DIR"
  printf '%s' "$val" >"$file"
  chmod "$mode" "$file"
  export "$var=$file"
}

write_pem REMOTR_CA_CERT "$CERT_DIR/ca.crt" 0644
write_pem REMOTR_CA_KEY "$CERT_DIR/ca.key" 0600
write_pem REMOTR_TLS_CERT "$CERT_DIR/server.crt" 0644
write_pem REMOTR_TLS_KEY "$CERT_DIR/server.key" 0600
write_pem REMOTR_TLS_CLIENT_CA "$CERT_DIR/client-ca.crt" 0644

exec /usr/local/bin/remotr-server "$@"
