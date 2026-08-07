#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
export REMOTR_BOOTSTRAP_SOURCE_ONLY=1
# shellcheck source=bootstrap.sh
source "$ROOT/deploy/fly/bootstrap.sh"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
CALLS="$TMP/calls"
touch "$CALLS"
FAKE="$TMP/fly"
cat >"$FAKE" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$CALLS"
case "$1 $2" in
  "redis status")
    if [[ -f "$(dirname "$CALLS")/exists" ]]; then
      printf 'Private URL = redis://:bootstrap-secret@fly-cache.upstash.io:6379\n'
      exit 0
    fi
    exit 1
    ;;
  "redis create") touch "$(dirname "$CALLS")/exists"; exit 0 ;;
esac
EOF
chmod +x "$FAKE"
export CALLS FLY="$FAKE" REMOTR_APP_NAME=remotr-test REMOTR_FLY_ORG=personal REMOTR_FLY_REGION=iad

REMOTR_SKIP_REDIS=1 create_redis
[[ "$REDIS_BACKEND" == memory && -z "$REDIS_URL" ]]

unset REMOTR_SKIP_REDIS
export REMOTR_REDIS_URL='rediss://:external-secret@example.invalid:6380'
create_redis
[[ "$REDIS_BACKEND" == redis && "$REDIS_URL" == "$REMOTR_REDIS_URL" ]]
! grep -q external-secret "$CALLS"

unset REMOTR_REDIS_URL
create_redis
[[ "$REDIS_URL" == redis://:bootstrap-secret@fly-cache.upstash.io:6379 ]]
grep -q '^redis status remotr-test-sync-cache$' "$CALLS"
grep -q '^redis create --name remotr-test-sync-cache --org personal --region iad --plan pay-as-you-go --no-replicas --disable-eviction$' "$CALLS"

# A rerun reuses the database and does not issue a second create.
create_redis
[[ $(grep -c '^redis create' "$CALLS") == 1 ]]

printf 'bootstrap Redis tests passed\n'
