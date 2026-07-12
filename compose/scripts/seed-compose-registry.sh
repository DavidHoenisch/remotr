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

INSERT INTO app_packages (id, name, version, s3_key, sha256, manifest)
VALUES (
  '11111111-1111-4111-8111-111111111111',
  'e2e/test-cli',
  '1.0.0',
  'app-packages/e2e/test-cli/1.0.0/e2e_test-cli-1.0.0.zip',
  '872ac97a9a5c1fb5a3870411d76785782607388d09d85861c2f367456647a4ff',
  '{"schemaVersion":1,"name":"e2e/test-cli","version":"1.0.0","install":{"mode":"binary","files":[{"src":"bin/test-cli","dest":"/usr/local/bin/e2e-test-cli","mode":"0755"}]}}'::jsonb
)
ON CONFLICT (name, version) DO NOTHING;
SQL

printf '%s\n' "$DEBIAN_TOKEN" > "${TOKEN_DIR}/debian.token"
printf '%s\n' "$ARCH_TOKEN" > "${TOKEN_DIR}/arch.token"
chmod 644 "${TOKEN_DIR}/debian.token" "${TOKEN_DIR}/arch.token"

echo "seeded fleet ${FLEET} and compose enrollment tokens in ${TOKEN_DIR}"
