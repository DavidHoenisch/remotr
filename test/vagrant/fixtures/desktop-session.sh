#!/usr/bin/env bash
set -euo pipefail

phase=
report=
while test "$#" -gt 0
do
  case "$1" in
    --phase) phase=${2:-}; shift 2 ;;
    --report) report=${2:-}; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

test -n "$phase"
test -n "$report"
user_a=remotr-desktop-a
user_b=remotr-desktop-b
home_a=/home/$user_a
home_b=/home/$user_b
schema=org.gnome.desktop.session
key=idle-delay

assert_baseline() {
  getent passwd "$user_a" | grep -Fq ":$home_a:/bin/bash"
  getent passwd "$user_b" | grep -Fq ":$home_b:/bin/bash"
  test "$(stat -c '%U:%G:%a' "$home_a")" = "$user_a:$user_a:700"
  test "$(stat -c '%U:%G:%a' "$home_b")" = "$user_b:$user_b:700"
  command -v dbus-run-session >/dev/null
  command -v dconf >/dev/null
  command -v gsettings >/dev/null
  runuser -u "$user_a" -- test ! -r "$home_b"
  runuser -u "$user_b" -- test ! -r "$home_a"
}

case "$phase" in
  exercise)
    assert_baseline
    install -d -o root -g root -m 700 "$(dirname "$report")"

    runuser -u "$user_b" -- env HOME="$home_b" dbus-run-session -- \
      gsettings set "$schema" "$key" 'uint32 654'
    test "$(runuser -u "$user_b" -- env HOME="$home_b" dbus-run-session -- gsettings get "$schema" "$key")" = 'uint32 654'

    runuser -u "$user_a" -- env HOME="$home_a" dbus-run-session -- sh -eu -c '
      gsettings set "$1" "$2" "uint32 321"
      touch "$HOME/.remotr-session-ready"
      while test ! -e "$HOME/.remotr-session-stop"; do sleep 0.1; done
    ' sh "$schema" "$key" &
    session_pid=$!
    for _ in $(seq 1 100)
    do
      test -e "$home_a/.remotr-session-ready" && break
      sleep 0.1
    done
    test -e "$home_a/.remotr-session-ready"
    kill -0 "$session_pid"
    test "$(runuser -u "$user_a" -- env HOME="$home_a" dbus-run-session -- gsettings get "$schema" "$key")" = 'uint32 321'
    test "$(runuser -u "$user_b" -- env HOME="$home_b" dbus-run-session -- gsettings get "$schema" "$key")" = 'uint32 654'
    runuser -u "$user_a" -- touch "$home_a/.remotr-session-stop"
    wait "$session_pid"

    runuser -u "$user_a" -- touch "$home_a/.remotr-snapshot-probe"
    runuser -u "$user_b" -- touch "$home_b/.remotr-snapshot-probe"
    printf '%s\n' \
      'interactive_users=verified' \
      'logged_out_execution=verified' \
      'logged_in_execution=verified' \
      'isolated_homes=verified' \
      'provider_facts=verified' > "$report"
    ;;
  verify-recovery)
    assert_baseline
    test ! -e "$home_a/.remotr-snapshot-probe"
    test ! -e "$home_b/.remotr-snapshot-probe"
    test ! -e "$home_a/.config/dconf/user"
    test ! -e "$home_b/.config/dconf/user"
    install -d -o root -g root -m 700 "$(dirname "$report")"
    printf '%s\n' 'snapshot_recovery=verified' > "$report"
    ;;
  *)
    echo "unsupported phase: $phase" >&2
    exit 2
    ;;
esac
