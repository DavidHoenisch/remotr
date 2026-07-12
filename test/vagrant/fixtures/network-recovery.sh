#!/usr/bin/env bash
set -euo pipefail

health_url=
ack_url=
token_file=
rollback_after=8

while test "$#" -gt 0
do
  case "$1" in
    --health-url)
      health_url=$2
      shift 2
      ;;
    --ack-url)
      ack_url=$2
      shift 2
      ;;
    --token-file)
      token_file=$2
      shift 2
      ;;
    --rollback-after)
      rollback_after=$2
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

test "$(id -u)" = 0
test -n "$health_url"
test -n "$ack_url"
test -r "$token_file"
test "$rollback_after" -ge 1

state_dir=$(mktemp -d /run/remotr-network-recovery.XXXXXX)
route=$(ip -4 route show default | head -n 1)
test -n "$route"
watchdog_pid=

restore_route() {
  ip -4 route replace $route
}

cleanup() {
  status=$?
  if test -n "$watchdog_pid"
  then
    kill "$watchdog_pid" 2>/dev/null || true
    wait "$watchdog_pid" 2>/dev/null || true
  fi
  restore_route || true
  rm -rf "$state_dir"
  exit "$status"
}
trap cleanup EXIT INT TERM

curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null

(
  sleep "$rollback_after"
  restore_route
  touch "$state_dir/rolled-back"
) &
watchdog_pid=$!

ip -4 route replace blackhole default
if curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null
then
  echo "control-path health probe unexpectedly succeeded during route loss" >&2
  exit 1
fi

wait "$watchdog_pid"
watchdog_pid=
test -f "$state_dir/rolled-back"

curl_config="$state_dir/curl.conf"
printf 'header = "Authorization: Bearer %s"\nrequest = "POST"\n' "$(cat "$token_file")" > "$curl_config"
chmod 600 "$curl_config"
curl --fail --silent --show-error --connect-timeout 3 --max-time 5 --config "$curl_config" "$ack_url" >/dev/null
