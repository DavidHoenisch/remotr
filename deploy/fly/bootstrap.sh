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
#   REMOTR_IMAGE           Docker image (default: docker.io/$REMOTR_DOCKER_USER/remotr-server:$TAG)
#   REMOTR_DOCKER_USER     Docker Hub user for default image (see deploy/fly/defaults.env)
#   REMOTR_IMAGE_TAG       Image tag when REMOTR_IMAGE unset (default: latest)
#   REMOTR_BUILD_FROM_SOURCE  Set to 1 to build deploy/fly/Dockerfile on Fly instead of pulling Hub
#   REMOTR_REPO            Git remote when cloning (bootstrap via curl)
#   REMOTR_REF             Git ref when cloning (default: master)
#   REMOTR_YES             Set to 1 to skip confirmation prompts
#   REMOTR_DATABASE_URL    Skip Neon create; use this Postgres URL instead
#   REMOTR_NEON_REUSE      Set to 1 to reuse an existing Neon project by name
#   REMOTR_SKIP_OPERATOR   Set to 1 to skip remotr CLI bootstrap + enroll token
#   REMOTR_FLY_SKIP_IPV4   Set to 1 to skip dedicated IPv4 (~$2/mo; IPv6-only may fail on some networks)

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
    printf '  Fly org:       %s\n' "$REMOTR_FLY_ORG"
    printf '  Neon project:  %s (%s)\n' "$REMOTR_NEON_PROJECT" "$REMOTR_NEON_REGION"
    printf '  Fleet:         %s\n' "$REMOTR_FLEET"
    if [[ "${REMOTR_BUILD_FROM_SOURCE:-}" == "1" ]]; then
      printf '  Deploy:        build from source on Fly\n'
    else
      printf '  Image:         %s\n' "$REMOTR_IMAGE"
    fi
    if [[ "${REMOTR_FLY_SKIP_IPV4:-}" == "1" ]]; then
      printf '  Fly IPs:       dedicated IPv6 only (IPv4 skipped)\n'
    else
      printf '  Fly IPs:       dedicated IPv6 + IPv4 (~$2/mo for v4)\n'
    fi
    printf '\n'
  } >"$tty"
}

load_image_defaults() {
  if [[ -f "${REMOTR_REPO_ROOT}/deploy/fly/defaults.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "${REMOTR_REPO_ROOT}/deploy/fly/defaults.env"
    set +a
  fi
  REMOTR_DOCKER_USER="${REMOTR_DOCKER_USER:-dh1689}"
  REMOTR_IMAGE_TAG="${REMOTR_IMAGE_TAG:-latest}"
  if [[ -z "${REMOTR_IMAGE:-}" ]]; then
    REMOTR_IMAGE="docker.io/${REMOTR_DOCKER_USER}/remotr-server:${REMOTR_IMAGE_TAG}"
  fi
}

fly_org_slugs() {
  if [[ -n "${FLY_ORGS_JSON:-}" ]]; then
    printf '%s' "$FLY_ORGS_JSON" | jq -r 'keys[]'
    return
  fi
  "$FLY" orgs list -j | jq -r 'keys[]'
}

load_fly_orgs() {
  [[ -n "${FLY_ORGS_JSON:-}" ]] && return 0
  FLY_ORGS_JSON=$("$FLY" orgs list -j)
}

validate_fly_org() {
  local org=$1
  load_fly_orgs
  if printf '%s' "$FLY_ORGS_JSON" | jq -e --arg o "$org" 'has($o)' >/dev/null; then
    return 0
  fi
  die "unknown Fly org: ${org} (run: $FLY orgs list)"
}

resolve_fly_org() {
  load_fly_orgs

  if [[ -n "${REMOTR_FLY_ORG:-}" ]]; then
    validate_fly_org "$REMOTR_FLY_ORG"
    return 0
  fi

  local orgs
  orgs=$(printf '%s' "$FLY_ORGS_JSON" | jq -r 'keys[]')
  local count
  count=$(printf '%s\n' "$orgs" | sed '/^$/d' | wc -l | tr -d ' ')

  if [[ "$count" -eq 1 ]]; then
    REMOTR_FLY_ORG=$(printf '%s\n' "$orgs" | head -1)
    log "using sole Fly org: ${REMOTR_FLY_ORG}"
    return 0
  fi

  if [[ "${REMOTR_YES:-}" == "1" ]]; then
    die "set REMOTR_FLY_ORG (available: $(printf '%s\n' "$orgs" | tr '\n' ' '))"
  fi

  local tty=/dev/tty
  if [[ ! -r "$tty" ]] || [[ ! -w "$tty" ]]; then
    die "set REMOTR_FLY_ORG — multiple Fly orgs available"
  fi

  {
    printf '\n'
    printf 'Select Fly organization (or set REMOTR_FLY_ORG):\n'
    while IFS= read -r slug; do
      [[ -z "$slug" ]] && continue
      local name
      name=$(printf '%s' "$FLY_ORGS_JSON" | jq -r --arg s "$slug" '.[$s]')
      printf '  %s — %s\n' "$slug" "$name"
    done <<< "$orgs"
    printf 'Org slug: '
  } >"$tty"

  local reply
  if ! read -r reply <"$tty"; then
    die "could not read Fly org from terminal"
  fi
  reply=${reply// /}
  [[ -z "$reply" ]] && die "Fly org is required (REMOTR_FLY_ORG)"
  validate_fly_org "$reply"
  REMOTR_FLY_ORG=$reply
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
  if [[ -n "${REMOTR_IMAGE:-}" ]] && [[ "${REMOTR_BUILD_FROM_SOURCE:-}" != "1" ]]; then
    export REMOTR_IMAGE
  fi
  [[ -n "${REMOTR_BUILD_FROM_SOURCE:-}" ]] && export REMOTR_BUILD_FROM_SOURCE
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

neon_json() {
  "$NEON" --no-color --output json "$@"
}

neon_project_id_by_name() {
  neon_json projects list | jq -r --arg name "$REMOTR_NEON_PROJECT" '
    (if type == "array" then . else .projects // [] end)
    | .[] | select(.name == $name) | .id' | head -1
}

neon_connection_uri() {
  local project_id=$1
  neon_json connection-string \
    --project-id "$project_id" \
    --database-name remotr | tr -d '\n\r'
}

use_existing_neon_project() {
  local project_id
  project_id=$(neon_project_id_by_name)
  [[ -n "$project_id" && "$project_id" != null ]] || return 1
  log "reusing Neon project ${project_id} (${REMOTR_NEON_PROJECT})"
  NEON_PROJECT_ID=$project_id
  DATABASE_URL=$(normalize_db_url "$(neon_connection_uri "$project_id")")
  [[ -n "$DATABASE_URL" ]] || die "could not resolve connection string for Neon project ${project_id}"
}

create_neon_database() {
  if [[ -n "${REMOTR_DATABASE_URL:-}" ]]; then
    log "using REMOTR_DATABASE_URL from environment (skipping Neon project create)"
    DATABASE_URL=$(normalize_db_url "$REMOTR_DATABASE_URL")
    NEON_PROJECT_ID=${REMOTR_NEON_PROJECT:-existing}
    return 0
  fi

  if [[ "${REMOTR_NEON_REUSE:-}" == "1" ]] && use_existing_neon_project; then
    return 0
  fi

  log "creating Neon project ${REMOTR_NEON_PROJECT}"
  local out rc
  set +e
  out=$(neon_json projects create \
    --name "$REMOTR_NEON_PROJECT" \
    --region-id "$REMOTR_NEON_REGION" \
    --database remotr 2>&1)
  rc=$?
  set -e

  if echo "$out" | jq -e '.project.id' >/dev/null 2>&1; then
    NEON_PROJECT_ID=$(echo "$out" | jq -r '.project.id')
    DATABASE_URL=$(echo "$out" | jq -r '.connection_uris[0].connection_uri')
    DATABASE_URL=$(normalize_db_url "$DATABASE_URL")
    log "Neon project id: ${NEON_PROJECT_ID}"
    return 0
  fi

  if [[ "${REMOTR_NEON_REUSE:-}" != "1" ]] && use_existing_neon_project; then
    warn "Neon create failed; reusing existing project named ${REMOTR_NEON_PROJECT}"
    return 0
  fi

  printf '\n%s\n' "$out" >&2
  die "Neon project create failed (output above is not JSON).

Common fixes:
  - Pick a supported region: REMOTR_NEON_REGION=aws-us-east-1 (or aws-us-east-2)
  - Reuse an existing project: REMOTR_NEON_REUSE=1
  - Bring your own database: REMOTR_DATABASE_URL='postgres://...'
  - Check Neon org limits: ${NEON} me"
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
  FLY_CONFIG="${REMOTR_REPO_ROOT}/deploy/fly/.fly-bootstrap.toml"
  sed \
    -e "s/^app = .*/app = \"${REMOTR_APP_NAME}\"/" \
    -e "s/^primary_region = .*/primary_region = \"${REMOTR_FLY_REGION}\"/" \
    "${REMOTR_REPO_ROOT}/deploy/fly/fly.toml" > "$FLY_CONFIG"

  if [[ "${REMOTR_BUILD_FROM_SOURCE:-}" == "1" ]]; then
    cat >> "$FLY_CONFIG" <<'EOF'

[build]
  dockerfile = "deploy/fly/Dockerfile"
EOF
  fi
}

create_fly_app() {
  log "creating Fly app ${REMOTR_APP_NAME} in ${REMOTR_FLY_REGION} (org: ${REMOTR_FLY_ORG})"
  "$FLY" apps create "$REMOTR_APP_NAME" --org "$REMOTR_FLY_ORG" 2>/dev/null || true

  if ! "$FLY" volumes list -a "$REMOTR_APP_NAME" 2>/dev/null | grep -q remotr_data; then
    "$FLY" volumes create remotr_data --size 1 --region "$REMOTR_FLY_REGION" -y -a "$REMOTR_APP_NAME"
  fi
}

fly_has_ip() {
  local kind=$1
  "$FLY" ips list -a "$REMOTR_APP_NAME" -j 2>/dev/null | jq -e --arg k "$kind" 'any(.[]; .Type == $k)' >/dev/null
}

ensure_fly_ips() {
  # Raw TCP on 443/8443 (mTLS in-app) cannot use Fly's free shared IPv4. Without any
  # dedicated IP, fly.dev DNS is not published and the app is unreachable publicly.
  log "ensuring Fly dedicated IPs (required for TCP/mTLS passthrough)"

  if ! fly_has_ip v6; then
    log "allocating dedicated IPv6 (free)"
    "$FLY" ips allocate-v6 -a "$REMOTR_APP_NAME"
  fi

  if [[ "${REMOTR_FLY_SKIP_IPV4:-}" == "1" ]]; then
    warn "REMOTR_FLY_SKIP_IPV4=1 — no IPv4; some networks cannot resolve or reach IPv6-only hosts"
    return 0
  fi

  if ! fly_has_ip v4; then
    log "allocating dedicated IPv4 (~\$2/mo — set REMOTR_FLY_SKIP_IPV4=1 to skip)"
    "$FLY" ips allocate-v4 -a "$REMOTR_APP_NAME" -y
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
  cd "$REMOTR_REPO_ROOT"
  if [[ "${REMOTR_BUILD_FROM_SOURCE:-}" == "1" ]]; then
    log "deploying to Fly.io (build from source on Fly)"
    "$FLY" deploy --config "$FLY_CONFIG" -a "$REMOTR_APP_NAME" --remote-only
    return
  fi
  log "deploying to Fly.io (image: ${REMOTR_IMAGE})"
  "$FLY" deploy --config "$FLY_CONFIG" --image "$REMOTR_IMAGE" -a "$REMOTR_APP_NAME" --remote-only
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
  if [[ -n "${FLY_CONFIG:-}" && -f "${FLY_CONFIG:-}" && "${FLY_CONFIG:-}" == *".fly-bootstrap.toml" ]]; then
    rm -f "$FLY_CONFIG"
  fi
  if [[ "${REMOTR_BOOTSTRAP_CLONED:-}" == "1" && "${REMOTR_BOOTSTRAP_KEEP_CLONE:-}" != "1" ]]; then
    rm -rf "${REMOTR_REPO_ROOT:-}"
  fi
}
trap cleanup EXIT

main() {
  ensure_repo_root
  load_image_defaults
  check_prerequisites
  resolve_fly_org

  show_plan
  log "Remotr bootstrap → Fly.io (${REMOTR_APP_NAME}) + Neon (${REMOTR_NEON_PROJECT})"
  confirm "Deploy Remotr to Fly.io and create a Neon Postgres project?"

  create_neon_database
  apply_schema
  generate_certificates
  write_fly_config
  create_fly_app
  ensure_fly_ips
  set_fly_secrets
  deploy_fly
  wait_for_server
  wait_for_bootstrap_token
  configure_operator
  save_local_artifacts
  print_summary
}

main "$@"
