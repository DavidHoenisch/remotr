#!/bin/sh
set -eu

fail() {
	printf 'desktop native smoke: %s\n' "$1" >&2
	exit 1
}

binary=""
expected_version=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--binary)
			[ "$#" -ge 2 ] || fail "--binary requires a path"
			binary=$2
			shift 2
			;;
		--version)
			[ "$#" -ge 2 ] || fail "--version requires an expected version"
			expected_version=$2
			shift 2
			;;
		*)
			fail "unknown argument: $1"
			;;
	esac
done

[ "$(uname -s)" = "Linux" ] || fail "Remotr Desktop native smoke supports Linux only"
[ -n "$binary" ] || fail "--binary is required"
[ -n "$expected_version" ] || fail "--version is required"
[ -x "$binary" ] || fail "binary is not executable: $binary"

expected_identity="Remotr Desktop $expected_version"
if ! actual_identity=$("$binary" --version 2>&1); then
	fail "could not read embedded identity from $binary"
fi
[ "$actual_identity" = "$expected_identity" ] ||
	fail "embedded identity is '$actual_identity'; expected '$expected_identity'"

command -v xvfb-run >/dev/null 2>&1 || fail "xvfb-run is required for headless native launch"
command -v xwininfo >/dev/null 2>&1 || fail "xwininfo is required to verify the native window"

attempts=${REMOTR_DESKTOP_SMOKE_ATTEMPTS:-100}
interval=${REMOTR_DESKTOP_SMOKE_INTERVAL:-0.1}
case "$attempts" in
	*[!0-9]* | 0 | "") fail "REMOTR_DESKTOP_SMOKE_ATTEMPTS must be a positive integer" ;;
esac

log_file=$(mktemp "${TMPDIR:-/tmp}/remotr-desktop-smoke.XXXXXX")
cleanup() {
	rm -f "$log_file"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

if ! xvfb-run -a sh -c '
binary=$1
log_file=$2
window_title=$3
attempts=$4
interval=$5

"$binary" >"$log_file" 2>&1 &
desktop_pid=$!

stop_desktop() {
	if kill -0 "$desktop_pid" 2>/dev/null; then
		kill "$desktop_pid" 2>/dev/null || true
		attempt=0
		while kill -0 "$desktop_pid" 2>/dev/null && [ "$attempt" -lt 20 ]; do
			sleep 0.05
			attempt=$((attempt + 1))
		done
		if kill -0 "$desktop_pid" 2>/dev/null; then
			kill -KILL "$desktop_pid" 2>/dev/null || true
		fi
	fi
	wait "$desktop_pid" 2>/dev/null || true
}
trap stop_desktop EXIT
trap "exit 1" HUP INT TERM

attempt=1
while [ "$attempt" -le "$attempts" ]; do
	sleep "$interval"
	if ! kill -0 "$desktop_pid" 2>/dev/null; then
		wait "$desktop_pid" 2>/dev/null
		status=$?
		printf "desktop native smoke: process exited before opening the Remotr Desktop window (status %s)\n" "$status" >&2
		cat "$log_file" >&2
		exit 1
	fi
	if xwininfo -root -tree 2>/dev/null | grep -F "\"$window_title\"" >/dev/null; then
		printf "desktop native smoke: native launch smoke passed for %s\n" "$window_title"
		exit 0
	fi
	attempt=$((attempt + 1))
done

printf "desktop native smoke: did not observe a Remotr Desktop window after %s attempts\n" "$attempts" >&2
cat "$log_file" >&2
exit 1
' remotr-desktop-smoke "$binary" "$log_file" "Remotr Desktop" "$attempts" "$interval"; then
	fail "Remotr Desktop window launch verification failed"
fi

printf 'desktop native smoke: embedded identity %s\n' "$expected_identity"
