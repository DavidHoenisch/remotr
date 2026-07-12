#!/bin/sh
set -eu

CERT_DIR="${CERT_DIR:-/certs}"
mkdir -p "$CERT_DIR"

if [ -f "$CERT_DIR/ca.crt" ]; then
  echo "certs already generated, skipping"
  exit 0
fi

# Remotr CA and server TLS only. Endpoint credentials come from enrollment (CSR).
openssl genrsa -out "$CERT_DIR/ca.key" 4096
openssl req -x509 -new -nodes -key "$CERT_DIR/ca.key" -sha256 -days 3650 \
  -out "$CERT_DIR/ca.crt" -subj "/CN=Remotr Test CA"

openssl genrsa -out "$CERT_DIR/server.key" 2048
openssl req -new -key "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" \
  -subj "/CN=remotr-server"
printf 'subjectAltName=DNS:remotr-server,DNS:localhost\n' > "$CERT_DIR/server.ext"
openssl x509 -req -in "$CERT_DIR/server.csr" -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" \
  -CAcreateserial -out "$CERT_DIR/server.crt" -days 825 -sha256 -extfile "$CERT_DIR/server.ext"

chmod 644 "$CERT_DIR"/*.crt
chmod 644 "$CERT_DIR"/*.key

echo "generated CA and server certs in $CERT_DIR"
