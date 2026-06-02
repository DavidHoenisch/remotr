#!/bin/sh
set -eu

OUT_DIR="${OUT_DIR:-/runtime}"
TOKEN_DIR="${OUT_DIR}/enroll-tokens"
DEBIAN_TOKEN="${REMOTR_COMPOSE_DEBIAN_ENROLL_TOKEN:-e2e-compose-debian-enroll}"
ARCH_TOKEN="${REMOTR_COMPOSE_ARCH_ENROLL_TOKEN:-e2e-compose-arch-enroll}"
FLEET="${REMOTR_COMPOSE_FLEET:-test-fleet}"

mkdir -p "$TOKEN_DIR"

psql -h postgres -U remotr -d remotr -v ON_ERROR_STOP=1 <<SQL
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('${FLEET}', 'auto')
ON CONFLICT (fleet) DO NOTHING;

INSERT INTO enrollment_tokens (token, fleet, expires_at)
VALUES
  ('${DEBIAN_TOKEN}', '${FLEET}', now() + interval '7 days'),
  ('${ARCH_TOKEN}', '${FLEET}', now() + interval '7 days')
ON CONFLICT (token) DO NOTHING;
SQL

printf '%s\n' "$DEBIAN_TOKEN" > "${TOKEN_DIR}/debian.token"
printf '%s\n' "$ARCH_TOKEN" > "${TOKEN_DIR}/arch.token"
chmod 644 "${TOKEN_DIR}/debian.token" "${TOKEN_DIR}/arch.token"

echo "seeded fleet ${FLEET} and compose enrollment tokens in ${TOKEN_DIR}"
