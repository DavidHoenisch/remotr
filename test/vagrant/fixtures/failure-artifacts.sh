#!/usr/bin/env bash
set -euo pipefail

canary_file=

while test "$#" -gt 0
do
  case "$1" in
    --canary-file)
      canary_file=$2
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

test "$(id -u)" = 0
test -r "$canary_file"
canary=$(cat "$canary_file")

printf '%s\n' \
  'provider=fixture' \
  'state_transition=preflight' \
  'safe_argv=/usr/bin/false --fixture-failure' \
  'state_transition=apply' \
  "token=$canary" \
  'state_transition=failed'
exit 42
