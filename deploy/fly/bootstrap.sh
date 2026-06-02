#!/usr/bin/env bash
# Remotr Fly.io + Neon bootstrap installer.
#
#   curl -fsSL .../bootstrap.sh | bash          # interactive (prompt on /dev/tty)
#   bash <(curl -fsSL .../bootstrap.sh)         # interactive (stdin stays your terminal)
#   REMOTR_YES=1 curl -fsSL .../bootstrap.sh | bash
#
# Or from a clone:
#   ./deploy/fly/bootstrap.sh
#
# Requires: fly (flyctl), neon (neonctl), git, curl, jq, openssl
# Optional: psql or Docker (for schema migration)
#
# Environment:
#   REMOTR_APP_NAME        Fly app name (default: remotr-<random>)
#   REMOTR_FLY_REGION      Fly region (default: iad)
#   REMOTR_FLY_ORG         Fly organization slug (optional)
#   REMOTR_NEON_PROJECT    Neon project name (default: <app-name>)
#   REMOTR_NEON_REGION     Neon region id (default: aws-us-east-1)
#   REMOTR_FLEET           Initial fleet name (default: default)
#   REMOTR_IMAGE           Pre-built image (skips local Docker build)
#   REMOTR_REPO            Git remote when cloning (bootstrap via curl)
#   REMOTR_REF             Git ref when cloning (default: master)
#   REMOTR_YES             Set to 1 to skip confirmation prompts
#   REMOTR_SKIP_OPERATOR   Set to 1 to skip remotr CLI bootstrap + enroll token

set -euo pipefail

REMOTR_APP_NAME="${REMOTR_APP_NAME:-remotr-$(openssl rand -hex 3)}"
REMOTR_FLY_REGION="${REMOTR_FLY_REGION:-iad}"
REMOTR_FLEET="${REMOTR_FLEET:-default}"
REMOTR_NEON_PROJECT="${REMOTR_NEON_PROJECT:-${REMOTR_APP_NAME}}"
REMOTR_NEON_REGION="${REMOTR_NEON_REGION:-aws-us-east-1}"
REMOTR_REPO="${REMOTR_REPO:-https://github.com/DavidHoenisch/remotr.git}"
REMOTR_REF="${REMOTR_REF:-master}"
STATE_DIR="${REMOTR_STATE_DIR:-${HOME}/.config/remotr/${REMOTR_APP_NAME}}"

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

confirm() {
  if [[ "${REMOTR_YES:-}" == "1" ]]; then
    return 0
  fi

  local prompt=$1 reply
  local tty=/dev/tty

  if [[ ! -r "$tty" ]] || [[ ! -w "$tty" ]]; then
    die "no terminal — use: bash <(curl -fsSL .../bootstrap.sh)  OR  REMOTR_YES=1 curl ... | bash"
  fi

  {
    printf '\n'
    printf '%s\n' "$prompt"
    printf 'Type yes to continue: '
  } >"$tty"

  if ! read -r reply <"$tty"; then
    die "could not read from terminal — try: bash <(curl -fsSL .../bootstrap.sh)"
  fi

  case "${reply}" in
    y|Y|yes|YES) return 0 ;;
    *) die "aborted" ;;
  esac
}

show_plan() {
  local tty=/dev/tty
  [[ -w "$tty" ]] || return 0
  {
    printf '\n'
    printf 'Remotr Fly.io bootstrap plan\n'
    printf '  Fly app:       %s (%s)\n' "$REMOTR_APP_NAME" "$REMOTR_FLY_REGION"
    printf '  Neon project:  %s (%s)\n' "$REMOTR_NEON_PROJECT" "$REMOTR_NEON_REGION"
    printf '  Fleet:         %s\n' "$REMOTR_FLEET"
    printf '\n'
  } >"$tty"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1 ($2)"
}

fly_cmd() {
  if command -v flyctl >/dev/null 2>&1; then
    echo flyctl
  elif command -v fly >/dev/null 2>&1; then
    echo fly
  else
    die "missing Fly CLI (install: https://fly.io/docs/hands-on/install-flyctl/)"
  fi
}

neon_cmd() {
  if command -v neon >/dev/null 2>&1; then
    echo neon
  elif command -v neonctl >/dev/null 2>&1; then
    echo neonctl
  else
    die "missing Neon CLI (install: npm install -g neonctl)"
  fi
}

ensure_repo_root() {
  if [[ -n "${REMOTR_REPO_ROOT:-}" && -f "${REMOTR_REPO_ROOT}/go.mod" ]]; then
    return 0
  fi

  local src="${BASH_SOURCE[0]:-}"
  if [[ "$src" == */deploy/fly/bootstrap.sh ]]; then
    local candidate
    candidate=$(cd "$(dirname "$src")/../.." && pwd)
    if [[ -f "$candidate/go.mod" ]]; then
      REMOTR_REPO_ROOT=$candidate
      return 0
    fi
  fi

  local tmp
  tmp=$(mktemp -d)
  log "cloning ${REMOTR_REPO} (${REMOTR_REF})"
  git clone --depth 1 --branch "$REMOTR_REF" "$REMOTR_REPO" "$tmp"
  export REMOTR_REPO_ROOT=$tmp
  export REMOTR_BOOTSTRAP_CLONED=1
  export REMOTR_APP_NAME REMOTR_FLY_REGION REMOTR_FLEET REMOTR_NEON_PROJECT REMOTR_NEON_REGION
  export REMOTR_REPO REMOTR_REF
  [[ -n "${REMOTR_FLY_ORG:-}" ]] && export REMOTR_FLY_ORG
  [[ -n "${REMOTR_IMAGE:-}" ]] && export REMOTR_IMAGE
  [[ -n "${REMOTR_STATE_DIR:-}" ]] && export REMOTR_STATE_DIR
  [[ -n "${REMOTR_YES:-}" ]] && export REMOTR_YES
  [[ -n "${REMOTR_SKIP_OPERATOR:-}" ]] && export REMOTR_SKIP_OPERATOR
  # Re-exec from disk so we are not running a stdin pipe script (fixes prompts + deploy paths).
  exec bash "$tmp/deploy/fly/bootstrap.sh" "$@"
}

check_prerequisites() {
  need_cmd git "https://git-scm.com/"
  need_cmd curl "https://curl.se/"
  need_cmd jq "https://jqlang.org/"
  need_cmd openssl "https://www.openssl.org/"

  FLY=$(fly_cmd)
  NEON=$(neon_cmd)

  if ! "$FLY" auth whoami >/dev/null 2>&1; then
    die "Fly CLI not authenticated — run: $FLY auth login"
  fi
  if ! "$NEON" me >/dev/null 2>&1; then
    die "Neon CLI not authenticated — run: $NEON auth"
  fi

  if ! command -v psql >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then
    die "need psql or Docker to apply sql/schema.sql"
  fi
}

run_psql() {
  if command -v psql >/dev/null 2>&1; then
    psql "$@"
    return
  fi
  docker run --rm -i postgres:17-alpine psql "$@"
}

normalize_db_url() {
  local url=$1
  url=${url/postgresql:/postgres:}
  if [[ "$url" != *sslmode=* ]]; then
    if [[ "$url" == *"?"* ]]; then
      url="${url}&sslmode=require"
    else
      url="${url}?sslmode=require"
    fi
  fi
  printf '%s' "$url"
}

create_neon_database() {
  log "creating Neon project ${REMOTR_NEON_PROJECT}"
  local json
  json=$("$NEON" projects create \
    --name "$REMOTR_NEON_PROJECT" \
    --region-id "$REMOTR_NEON_REGION" \
    --database remotr \
    --output json)

  NEON_PROJECT_ID=$(echo "$json" | jq -r '.project.id')
  [[ -n "$NEON_PROJECT_ID" && "$NEON_PROJECT_ID" != null ]] || die "failed to parse Neon project id"

  DATABASE_URL=$(echo "$json" | jq -r '.connection_uris[0].connection_uri')
  [[ -n "$DATABASE_URL" && "$DATABASE_URL" != null ]] || die "failed to parse Neon connection URI"
  DATABASE_URL=$(normalize_db_url "$DATABASE_URL")

  log "Neon project id: ${NEON_PROJECT_ID}"
}

apply_schema() {
  log "applying Postgres schema"
  run_psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "${REMOTR_REPO_ROOT}/sql/schema.sql"
  run_psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<SQL
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('${REMOTR_FLEET}', 'auto')
ON CONFLICT (fleet) DO NOTHING;
SQL
}

generate_certificates() {
  CERT_DIR=$(mktemp -d)
  SERVER_HOST="${REMOTR_APP_NAME}.fly.dev"

  log "generating Remotr CA and server TLS certificates for ${SERVER_HOST}"

  openssl genrsa -out "${CERT_DIR}/ca.key" 4096 2>/dev/null
  openssl req -x509 -new -nodes -key "${CERT_DIR}/ca.key" -sha256 -days 3650 \
    -out "${CERT_DIR}/ca.crt" -subj "/CN=Remotr CA" 2>/dev/null

  openssl genrsa -out "${CERT_DIR}/server.key" 2048 2>/dev/null
  openssl req -new -key "${CERT_DIR}/server.key" -out "${CERT_DIR}/server.csr" \
    -subj "/CN=${SERVER_HOST}" 2>/dev/null
  cat > "${CERT_DIR}/server.ext" <<EOF
subjectAltName=DNS:${SERVER_HOST},DNS:localhost
EOF
  openssl x509 -req -in "${CERT_DIR}/server.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial -out "${CERT_DIR}/server.crt" -days 825 -sha256 -extfile "${CERT_DIR}/server.ext" 2>/dev/null

  WEBHOOK_SECRET=$(openssl rand -hex 24)
}

write_fly_config() {
  FLY_CONFIG=$(mktemp)
  sed \
    -e "s/^app = .*/app = \"${REMOTR_APP_NAME}\"/" \
    -e "s/^primary_region = .*/primary_region = \"${REMOTR_FLY_REGION}\"/" \
    "${REMOTR_REPO_ROOT}/deploy/fly/fly.toml" > "$FLY_CONFIG"
}

create_fly_app() {
  log "creating Fly app ${REMOTR_APP_NAME} in ${REMOTR_FLY_REGION}"
  if [[ -n "${REMOTR_FLY_ORG:-}" ]]; then
    "$FLY" apps create "$REMOTR_APP_NAME" --org "$REMOTR_FLY_ORG" 2>/dev/null || true
  else
    "$FLY" apps create "$REMOTR_APP_NAME" 2>/dev/null || true
  fi

  if ! "$FLY" volumes list -a "$REMOTR_APP_NAME" 2>/dev/null | grep -q remotr_data; then
    "$FLY" volumes create remotr_data --size 1 --region "$REMOTR_FLY_REGION" -y -a "$REMOTR_APP_NAME"
  fi
}

set_fly_secrets() {
  log "setting Fly secrets"
  "$FLY" secrets set \
    REMOTR_DATABASE_URL="$DATABASE_URL" \
    REMOTR_GIT_WEBHOOK_SECRET="$WEBHOOK_SECRET" \
    REMOTR_CA_CERT="$(cat "${CERT_DIR}/ca.crt")" \
    REMOTR_CA_KEY="$(cat "${CERT_DIR}/ca.key")" \
    REMOTR_TLS_CERT="$(cat "${CERT_DIR}/server.crt")" \
    REMOTR_TLS_KEY="$(cat "${CERT_DIR}/server.key")" \
    REMOTR_TLS_CLIENT_CA="$(cat "${CERT_DIR}/ca.crt")" \
    -a "$REMOTR_APP_NAME"
}

deploy_fly() {
  log "deploying to Fly.io"
  cd "$REMOTR_REPO_ROOT"
  if [[ -n "${REMOTR_IMAGE:-}" ]]; then
    "$FLY" deploy --config "$FLY_CONFIG" --image "$REMOTR_IMAGE" -a "$REMOTR_APP_NAME" --remote-only
  else
    "$FLY" deploy --config "$FLY_CONFIG" -a "$REMOTR_APP_NAME" --remote-only
  fi
}

wait_for_server() {
  log "waiting for https://${REMOTR_APP_NAME}.fly.dev/healthz"
  local i
  for i in $(seq 1 60); do
    if curl -kfsS "https://${REMOTR_APP_NAME}.fly.dev/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done
  warn "health check did not pass yet — continuing (server may still be starting)"
}

wait_for_bootstrap_token() {
  log "waiting for operator bootstrap token (check Fly logs / volume)"
  local token="" i
  for i in $(seq 1 90); do
    token=$("$FLY" ssh console -a "$REMOTR_APP_NAME" -C "cat /var/lib/remotr/bootstrap.token" 2>/dev/null | tr -d ' \n\r' || true)
    if [[ -n "$token" ]]; then
      BOOTSTRAP_TOKEN=$token
      return 0
    fi
    sleep 2
  done
  die "timed out waiting for bootstrap token — try: $FLY logs -a $REMOTR_APP_NAME"
}

install_remotr_cli() {
  if command -v remotr >/dev/null 2>&1; then
    return 0
  fi
  if ! command -v go >/dev/null 2>&1; then
    warn "remotr CLI not found and Go not installed — install from GitHub Releases (docs/guides/installing-cli.md)"
    return 1
  fi
  log "installing remotr CLI with go install"
  (cd "$REMOTR_REPO_ROOT" && go install -mod=vendor ./cmd/remotr)
  export PATH="${PATH}:$(go env GOPATH)/bin"
  command -v remotr >/dev/null 2>&1
}

configure_operator() {
  SERVER_URL="https://${REMOTR_APP_NAME}.fly.dev"
  mkdir -p "$STATE_DIR"
  cp "${CERT_DIR}/ca.crt" "${STATE_DIR}/ca.crt"

  if [[ "${REMOTR_SKIP_OPERATOR:-}" == "1" ]]; then
    warn "REMOTR_SKIP_OPERATOR=1 — skipping operator bootstrap"
    return 0
  fi

  if ! install_remotr_cli; then
    warn "skipping operator bootstrap (install remotr CLI manually)"
    return 0
  fi

  log "bootstrapping operator credentials"
  remotr bootstrap \
    --server-url "$SERVER_URL" \
    --ca "${STATE_DIR}/ca.crt" \
    --token "$BOOTSTRAP_TOKEN" \
    --state-dir "$STATE_DIR"

  log "creating enrollment token for fleet ${REMOTR_FLEET}"
  ENROLL_OUTPUT=$(remotr enroll token create \
    --server-url "$SERVER_URL" \
    --fleet "$REMOTR_FLEET" \
    --state-dir "$STATE_DIR" 2>&1) || die "enroll token create failed: ${ENROLL_OUTPUT}"
  ENROLL_TOKEN=$(echo "$ENROLL_OUTPUT" | sed -n 's/^enrollment token (one-time): //p')
}

save_local_artifacts() {
  mkdir -p "$STATE_DIR"
  cp "${CERT_DIR}/ca.crt" "${STATE_DIR}/ca.crt"
  chmod 600 "${STATE_DIR}/ca.crt" 2>/dev/null || true

  SUMMARY_FILE="${STATE_DIR}/fly-bootstrap.txt"
  cat > "$SUMMARY_FILE" <<EOF
Remotr Fly bootstrap summary
============================
Server URL:     https://${REMOTR_APP_NAME}.fly.dev
Fly app:        ${REMOTR_APP_NAME}
Fly region:     ${REMOTR_FLY_REGION}
Neon project:   ${NEON_PROJECT_ID}
Database:       remotr (Neon)
Fleet:          ${REMOTR_FLEET}
Operator state: ${STATE_DIR}
CA certificate: ${STATE_DIR}/ca.crt
Git webhook:    POST https://${REMOTR_APP_NAME}.fly.dev/v1/webhooks/git
Webhook header: X-Remotr-Git-Webhook-Secret: ${WEBHOOK_SECRET}
EOF
  if [[ -n "${ENROLL_TOKEN:-}" ]]; then
    cat >> "$SUMMARY_FILE" <<EOF
Enrollment token (one-time): ${ENROLL_TOKEN}
EOF
  fi
  if [[ -n "${BOOTSTRAP_TOKEN:-}" && "${REMOTR_SKIP_OPERATOR:-}" == "1" ]]; then
    cat >> "$SUMMARY_FILE" <<EOF
Bootstrap token (one-time): ${BOOTSTRAP_TOKEN}
EOF
  fi
}

print_summary() {
  cat <<EOF

Remotr is deployed on Fly.io
  Server:    https://${REMOTR_APP_NAME}.fly.dev
  Fly app:   ${REMOTR_APP_NAME}
  Neon DB:   ${NEON_PROJECT_ID} / remotr
  Fleet:     ${REMOTR_FLEET}

Operator files: ${STATE_DIR}
  CA cert:   ${STATE_DIR}/ca.crt
  Summary:   ${STATE_DIR}/fly-bootstrap.txt

Next steps
  1. Enroll an agent:
       remotr-agent enroll \\
         --server-url https://${REMOTR_APP_NAME}.fly.dev \\
         --ca ${STATE_DIR}/ca.crt \\
         --token <enrollment-token>

  2. Point a Git config repo at the server (set REMOTR_GIT_REMOTE_URL secret) and call:
       curl -X POST https://${REMOTR_APP_NAME}.fly.dev/v1/webhooks/git \\
         -H "X-Remotr-Git-Webhook-Secret: ${WEBHOOK_SECRET}"

  3. List endpoints:
       remotr endpoint list --server-url https://${REMOTR_APP_NAME}.fly.dev --state-dir ${STATE_DIR}

Docs: deploy/fly/README.md
EOF
}

cleanup() {
  if [[ -n "${CERT_DIR:-}" && -d "${CERT_DIR:-}" ]]; then
    rm -rf "$CERT_DIR"
  fi
  if [[ -n "${FLY_CONFIG:-}" && -f "${FLY_CONFIG:-}" ]]; then
    rm -f "$FLY_CONFIG"
  fi
  if [[ "${REMOTR_BOOTSTRAP_CLONED:-}" == "1" && "${REMOTR_BOOTSTRAP_KEEP_CLONE:-}" != "1" ]]; then
    rm -rf "${REMOTR_REPO_ROOT:-}"
  fi
}
trap cleanup EXIT

main() {
  ensure_repo_root
  check_prerequisites

  show_plan
  log "Remotr bootstrap → Fly.io (${REMOTR_APP_NAME}) + Neon (${REMOTR_NEON_PROJECT})"
  confirm "Deploy Remotr to Fly.io and create a Neon Postgres project?"

  create_neon_database
  apply_schema
  generate_certificates
  write_fly_config
  create_fly_app
  set_fly_secrets
  deploy_fly
  wait_for_server
  wait_for_bootstrap_token
  configure_operator
  save_local_artifacts
  print_summary
}

main "$@"
