#!/usr/bin/env bash
# Rotate Tigris (S3) credentials for a Remotr Fly app without printing secrets.
#
# Blue-green flow (default):
#   1. Create a new Tigris access key scoped to the app bucket
#   2. Import AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY into Fly via stdin
#   3. Wait for the app to redeploy and verify bucket access
#   4. Delete the previous access key
#
# Requires: fly (flyctl), tigris, jq
# Prerequisite: tigris login (OAuth) with access to the bucket's organization
#
# Usage:
#   REMOTR_APP_NAME=remotr-b8108f ./deploy/fly/rotate-tigris-credentials.sh
#   REMOTR_YES=1 REMOTR_APP_NAME=remotr-b8108f ./deploy/fly/rotate-tigris-credentials.sh
#
# Environment:
#   REMOTR_APP_NAME              Fly app name (required)
#   REMOTR_TIGRIS_BUCKET         Bucket name (default: resolve from Fly app)
#   REMOTR_TIGRIS_ORG            Tigris organization (default: auto-detect from bucket)
#   REMOTR_TIGRIS_ROTATE_IN_PLACE Set to 1 to rotate the current key secret in place
#   REMOTR_KEEP_OLD_KEY          Set to 1 to skip deleting the superseded access key
#   REMOTR_SKIP_VERIFY           Set to 1 to skip post-deploy S3 access check
#   REMOTR_STATE_DIR             Stamp file directory (default: ~/.config/remotr/<app>)
#   REMOTR_YES                   Skip confirmation prompt

set -euo pipefail
umask 077

if [[ "$-" == *x* ]]; then
  printf 'error: shell tracing (set -x) is enabled — disable before rotating credentials\n' >&2
  exit 1
fi

REMOTR_APP_NAME="${REMOTR_APP_NAME:-}"
REMOTR_STATE_DIR="${REMOTR_STATE_DIR:-}"

SECRET_IMPORT_FILE=""
TIGRIS_JSON_FILE=""

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

scrub_file() {
  local f=$1
  [[ -z "$f" || ! -e "$f" ]] && return 0
  if command -v shred >/dev/null 2>&1; then
    shred -u "$f" 2>/dev/null || rm -f "$f"
  else
    rm -f "$f"
  fi
}

cleanup() {
  scrub_file "${SECRET_IMPORT_FILE:-}"
  scrub_file "${TIGRIS_JSON_FILE:-}"
}
trap cleanup EXIT

confirm() {
  if [[ "${REMOTR_YES:-}" == "1" ]]; then
    return 0
  fi
  local prompt=$1 reply tty=/dev/tty
  if [[ ! -r "$tty" ]] || [[ ! -w "$tty" ]]; then
    die "no terminal — set REMOTR_YES=1 for non-interactive use"
  fi
  {
    printf '\n%s\n' "$prompt"
    printf 'Type yes to continue: '
  } >"$tty"
  read -r reply <"$tty" || die "could not read confirmation"
  case "${reply}" in
    y|Y|yes|YES) ;;
    *) die "aborted" ;;
  esac
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
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

tigris_cmd() {
  command -v tigris >/dev/null 2>&1 || die "missing Tigris CLI (install: https://www.tigrisdata.com/docs/cli/)"
  echo tigris
}

mask_key_id() {
  local id=$1
  if [[ ${#id} -le 12 ]]; then
    printf 'tid_***'
    return
  fi
  printf '%s...%s' "${id:0:8}" "${id: -4}"
}

fly_app_env() {
  local app=$1 var=$2
  "$FLY" ssh console --app "$app" -C "printenv ${var}" 2>/dev/null | tr -d '\r\n'
}

resolve_bucket_name() {
  if [[ -n "${REMOTR_TIGRIS_BUCKET:-}" ]]; then
    printf '%s' "$REMOTR_TIGRIS_BUCKET"
    return 0
  fi
  local bucket
  bucket=$(fly_app_env "$REMOTR_APP_NAME" BUCKET_NAME)
  if [[ -n "$bucket" ]]; then
    printf '%s' "$bucket"
    return 0
  fi
  bucket=$("$FLY" storage status --app "$REMOTR_APP_NAME" --yes 2>/dev/null \
    | sed -n 's/^ Name.*│ *//p' | head -1 | tr -d ' \r\n')
  [[ -n "$bucket" ]] || die "could not resolve bucket — set REMOTR_TIGRIS_BUCKET"
  printf '%s' "$bucket"
}

bucket_in_active_org() {
  local bucket=$1
  "$TIGRIS" buckets list --json --yes 2>/dev/null \
    | jq -e --arg b "$bucket" '
        (.buckets // . // [])
        | if type == "array" then . else [.] end
        | any(.[]; (.name // .Name // "") == $b)
      ' >/dev/null
}

select_tigris_org_for_bucket() {
  local bucket=$1 org
  if [[ -n "${REMOTR_TIGRIS_ORG:-}" ]]; then
    "$TIGRIS" orgs select "$REMOTR_TIGRIS_ORG" --yes >/dev/null 2>&1 \
      || die "could not select Tigris org: ${REMOTR_TIGRIS_ORG}"
    bucket_in_active_org "$bucket" || die "bucket ${bucket} not found in org ${REMOTR_TIGRIS_ORG}"
    return 0
  fi

  if bucket_in_active_org "$bucket"; then
    return 0
  fi

  local orgs_json org_name
  orgs_json=$("$TIGRIS" orgs list --json --yes 2>/dev/null) \
    || die "tigris orgs list failed — run: tigris login"
  while IFS= read -r org_name; do
    [[ -z "$org_name" ]] && continue
    "$TIGRIS" orgs select "$org_name" --yes >/dev/null 2>&1 || continue
    if bucket_in_active_org "$bucket"; then
      log "using Tigris org: ${org_name}"
      return 0
    fi
  done < <(printf '%s' "$orgs_json" | jq -r '
    (.organizations // . // [])
    | if type == "array" then . else [.] end
    | .[]
    | .name // .Name // empty
  ')

  die "bucket ${bucket} not found in any Tigris org — set REMOTR_TIGRIS_ORG"
}

ensure_tigris_session() {
  "$TIGRIS" whoami --json --yes >/dev/null 2>&1 \
    || die "Tigris CLI not authenticated — run: tigris login"
}

parse_access_key_json() {
  local file=$1
  NEW_ACCESS_KEY_ID=$(jq -r '
    .accessKeyId // .access_key_id // .id // .keyId // .AccessKeyId // empty
  ' "$file")
  NEW_SECRET_ACCESS_KEY=$(jq -r '
    .secretAccessKey // .secret_access_key // .secret // .SecretAccessKey // empty
  ' "$file")
  [[ -n "$NEW_ACCESS_KEY_ID" && -n "$NEW_SECRET_ACCESS_KEY" ]] \
    || die "could not parse access key response from Tigris CLI"
}

run_tigris_json() {
  local rc
  TIGRIS_JSON_FILE=$(mktemp)
  set +e
  "$TIGRIS" "$@" --json --yes >"$TIGRIS_JSON_FILE" 2>/dev/null
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    die "tigris command failed: tigris $*"
  fi
}

create_access_key() {
  local name=$1
  run_tigris_json access-keys create "$name"
  parse_access_key_json "$TIGRIS_JSON_FILE"
  scrub_file "$TIGRIS_JSON_FILE"
  TIGRIS_JSON_FILE=""
}

rotate_access_key_secret() {
  local key_id=$1
  run_tigris_json access-keys rotate "$key_id"
  parse_access_key_json "$TIGRIS_JSON_FILE"
  NEW_ACCESS_KEY_ID=$key_id
  scrub_file "$TIGRIS_JSON_FILE"
  TIGRIS_JSON_FILE=""
}

assign_bucket_editor() {
  local key_id=$1 bucket=$2
  "$TIGRIS" access-keys assign "$key_id" --bucket "$bucket" --role Editor --yes >/dev/null 2>&1 \
    || die "failed to assign Editor on bucket ${bucket} for $(mask_key_id "$key_id")"
}

write_secret_import_file() {
  local endpoint=${1:-https://t3.storage.dev}
  SECRET_IMPORT_FILE=$(mktemp)
  {
    printf 'AWS_ACCESS_KEY_ID=%s\n' "$NEW_ACCESS_KEY_ID"
    printf 'AWS_SECRET_ACCESS_KEY=%s\n' "$NEW_SECRET_ACCESS_KEY"
    printf 'AWS_ENDPOINT_URL_S3=%s\n' "$endpoint"
    printf 'AWS_REGION=auto\n'
    printf 'BUCKET_NAME=%s\n' "$BUCKET_NAME"
  } >"$SECRET_IMPORT_FILE"
}

import_fly_secrets() {
  log "updating Fly secrets on ${REMOTR_APP_NAME} (credentials not logged)"
  "$FLY" secrets import --app "$REMOTR_APP_NAME" <"$SECRET_IMPORT_FILE" >/dev/null
  scrub_file "$SECRET_IMPORT_FILE"
  SECRET_IMPORT_FILE=""
}

wait_for_fly_deploy() {
  log "waiting for ${REMOTR_APP_NAME} to redeploy"
  local i
  for i in $(seq 1 60); do
    if curl -kfsS "https://${REMOTR_APP_NAME}.fly.dev/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done
  die "timed out waiting for Fly app health after secret update"
}

verify_bucket_access() {
  [[ "${REMOTR_SKIP_VERIFY:-}" == "1" ]] && return 0
  log "verifying S3 access to ${BUCKET_NAME}"
  AWS_ACCESS_KEY_ID=$NEW_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY=$NEW_SECRET_ACCESS_KEY \
    "$TIGRIS" credentials test --bucket "$BUCKET_NAME" --json --yes >/dev/null 2>&1 \
    || die "new credentials failed bucket access check"
}

verify_fly_runtime_key() {
  [[ "${REMOTR_SKIP_VERIFY:-}" == "1" ]] && return 0
  local live_id
  live_id=$(fly_app_env "$REMOTR_APP_NAME" AWS_ACCESS_KEY_ID)
  [[ "$live_id" == "$NEW_ACCESS_KEY_ID" ]] \
    || die "Fly app AWS_ACCESS_KEY_ID does not match the rotated key yet"
}

delete_access_key() {
  local key_id=$1
  [[ -z "$key_id" ]] && return 0
  [[ "$key_id" == "$NEW_ACCESS_KEY_ID" ]] && return 0
  log "revoking superseded access key $(mask_key_id "$key_id")"
  "$TIGRIS" access-keys delete "$key_id" --yes >/dev/null 2>&1 \
    || warn "could not delete old access key $(mask_key_id "$key_id") — remove it manually in Tigris"
}

write_rotation_stamp() {
  local stamp_dir=${REMOTR_STATE_DIR:-${HOME}/.config/remotr/${REMOTR_APP_NAME}}
  local stamp_file=${stamp_dir}/s3-credential-rotation.json
  mkdir -p "$stamp_dir"
  chmod 700 "$stamp_dir" 2>/dev/null || true
  jq -n \
    --arg rotated_at "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" \
    --arg fly_app "$REMOTR_APP_NAME" \
    --arg bucket "$BUCKET_NAME" \
    --arg access_key_id "$NEW_ACCESS_KEY_ID" \
    --arg mode "${ROTATION_MODE}" \
    '{
      rotated_at: $rotated_at,
      fly_app: $fly_app,
      bucket: $bucket,
      access_key_id: $access_key_id,
      rotation_mode: $mode
    }' >"$stamp_file"
  chmod 600 "$stamp_file"
  log "rotation stamp: ${stamp_file}"
}

main() {
  [[ -n "$REMOTR_APP_NAME" ]] || die "set REMOTR_APP_NAME"

  need_cmd jq
  FLY=$(fly_cmd)
  TIGRIS=$(tigris_cmd)

  "$FLY" auth whoami >/dev/null 2>&1 || die "Fly CLI not authenticated — run: $FLY auth login"
  ensure_tigris_session

  BUCKET_NAME=$(resolve_bucket_name)
  OLD_ACCESS_KEY_ID=$(fly_app_env "$REMOTR_APP_NAME" AWS_ACCESS_KEY_ID)
  ENDPOINT=$(fly_app_env "$REMOTR_APP_NAME" AWS_ENDPOINT_URL_S3)
  [[ -z "$ENDPOINT" ]] && ENDPOINT=https://t3.storage.dev

  select_tigris_org_for_bucket "$BUCKET_NAME"

  if [[ "${REMOTR_TIGRIS_ROTATE_IN_PLACE:-}" == "1" ]]; then
    ROTATION_MODE=in_place
    [[ -n "$OLD_ACCESS_KEY_ID" ]] || die "could not read current AWS_ACCESS_KEY_ID from Fly app"
    log "plan: rotate secret in place for $(mask_key_id "$OLD_ACCESS_KEY_ID") on ${REMOTR_APP_NAME} (bucket ${BUCKET_NAME})"
  else
    ROTATION_MODE=blue_green
    log "plan: create new Tigris key, update Fly app ${REMOTR_APP_NAME}, revoke old key (bucket ${BUCKET_NAME})"
    if [[ -n "$OLD_ACCESS_KEY_ID" ]]; then
      log "current key: $(mask_key_id "$OLD_ACCESS_KEY_ID")"
    fi
  fi

  confirm "Rotate Tigris credentials for ${REMOTR_APP_NAME}?"

  if [[ "$ROTATION_MODE" == "in_place" ]]; then
    rotate_access_key_secret "$OLD_ACCESS_KEY_ID"
  else
    local key_name="remotr-${REMOTR_APP_NAME}-$(date -u +%Y%m%d%H%M%S)"
    create_access_key "$key_name"
    assign_bucket_editor "$NEW_ACCESS_KEY_ID" "$BUCKET_NAME"
  fi

  log "new key: $(mask_key_id "$NEW_ACCESS_KEY_ID")"

  write_secret_import_file "$ENDPOINT"
  import_fly_secrets
  wait_for_fly_deploy
  verify_bucket_access
  verify_fly_runtime_key

  if [[ "$ROTATION_MODE" == "blue_green" && "${REMOTR_KEEP_OLD_KEY:-}" != "1" ]]; then
    delete_access_key "$OLD_ACCESS_KEY_ID"
  fi

  write_rotation_stamp
  log "Tigris credential rotation complete for ${REMOTR_APP_NAME}"
}

main "$@"
