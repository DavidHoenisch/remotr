#!/usr/bin/env bash
set -euo pipefail

health_url=
ack_url=
token_file=
report=
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
    --report)
      report=$2
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
test -n "$report"
test "$rollback_after" -ge 1

state_dir=$(mktemp -d /run/remotr-network-recovery.XXXXXX)
route=$(ip -4 route show default | head -n 1)
test -n "$route"
watchdog_pid=
resolver_backup=
control_interface=

control_authority=${health_url#*://}
control_authority=${control_authority%%/*}
control_host=${control_authority%:*}
control_port=${control_authority##*:}
test -n "$control_host"
test "$control_host" != "$control_authority"
test "$control_port" -ge 1
test "$control_port" -le 65535
control_interface=$(ip -4 route get "$control_host" | awk 'NR == 1 { for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }')
test -n "$control_interface"

restore_route() {
  ip -4 route replace $route
}

restore_control_route() {
  ip -4 route del blackhole "$control_host"/32 2>/dev/null || true
}

restore_resolver() {
  if test -n "$resolver_backup" && test -r "$resolver_backup"
  then
    cat "$resolver_backup" > /etc/resolv.conf
  fi
}

restore_firewall() {
  nft delete table inet remotr_vm_recovery 2>/dev/null || true
}

restore_profile() {
  if test -n "$control_interface"
  then
    ip link set dev "$control_interface" up || true
    restore_route || true
  fi
}

wait_for_watchdog() {
  wait "$watchdog_pid"
  watchdog_pid=
}

health_fails() {
  ! curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null
}

cleanup() {
  status=$?
  if test -n "$watchdog_pid"
  then
    kill "$watchdog_pid" 2>/dev/null || true
    wait "$watchdog_pid" 2>/dev/null || true
  fi
  restore_profile || true
  restore_firewall || true
  restore_resolver || true
  restore_control_route || true
  restore_route || true
  rm -rf "$state_dir"
  exit "$status"
}
trap cleanup EXIT INT TERM

curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null

(
  sleep "$rollback_after"
  restore_control_route
  touch "$state_dir/rolled-back"
) &
watchdog_pid=$!

ip -4 route replace blackhole "$control_host"/32
if ! health_fails
then
  echo "control-path health probe unexpectedly succeeded during route loss" >&2
  exit 1
fi

wait_for_watchdog
test -f "$state_dir/rolled-back"
curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null

resolver_backup="$state_dir/resolv.conf"
cat /etc/resolv.conf > "$resolver_backup"
getent ahostsv4 deb.debian.org >/dev/null
(
  sleep "$rollback_after"
  restore_resolver
  touch "$state_dir/dns-rolled-back"
) &
watchdog_pid=$!
printf 'nameserver 127.0.0.1\noptions attempts:1 timeout:1\n' > /etc/resolv.conf
if getent ahostsv4 deb.debian.org >/dev/null 2>&1
then
  echo "DNS probe unexpectedly succeeded during resolver loss" >&2
  exit 1
fi
wait_for_watchdog
test -f "$state_dir/dns-rolled-back"
cmp -s "$resolver_backup" /etc/resolv.conf
getent ahostsv4 deb.debian.org >/dev/null

nft add table inet remotr_vm_recovery
nft 'add chain inet remotr_vm_recovery output { type filter hook output priority -300; policy accept; }'
nft add rule inet remotr_vm_recovery output ip daddr "$control_host" tcp dport "$control_port" drop
(
  sleep "$rollback_after"
  restore_firewall
  touch "$state_dir/firewall-rolled-back"
) &
watchdog_pid=$!
if ! health_fails
then
  echo "control-path health probe unexpectedly succeeded during firewall loss" >&2
  exit 1
fi
wait_for_watchdog
test -f "$state_dir/firewall-rolled-back"
curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null

(
  sleep "$rollback_after"
  restore_profile
  touch "$state_dir/profile-rolled-back"
) &
watchdog_pid=$!
ip link set dev "$control_interface" down
if ! health_fails
then
  echo "control-path health probe unexpectedly succeeded during profile loss" >&2
  exit 1
fi
wait_for_watchdog
test -f "$state_dir/profile-rolled-back"
curl --fail --silent --show-error --connect-timeout 3 --max-time 5 "$health_url" >/dev/null

printf '%s\n' \
  'route_recovery=verified' \
  'dns_recovery=verified' \
  'firewall_recovery=verified' \
  'profile_recovery=verified' > "$report"
chmod 600 "$report"

curl_config="$state_dir/curl.conf"
printf 'header = "Authorization: Bearer %s"\nrequest = "POST"\n' "$(cat "$token_file")" > "$curl_config"
chmod 600 "$curl_config"
curl --fail --silent --show-error --connect-timeout 3 --max-time 5 --config "$curl_config" "$ack_url" >/dev/null
