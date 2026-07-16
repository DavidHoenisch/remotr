#!/bin/sh
set -eu

fail() {
	printf 'desktop Flatpak smoke: %s\n' "$1" >&2
	exit 1
}

bundle=""
expected_version=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--package)
			[ "$#" -ge 2 ] || fail "--package requires a path"
			bundle=$2
			shift 2
			;;
		--version)
			[ "$#" -ge 2 ] || fail "--version requires a value"
			expected_version=$2
			shift 2
			;;
		*) fail "unknown argument: $1" ;;
	esac
done

[ "$(uname -s)" = "Linux" ] || fail "only Linux package smoke is supported"
[ "$(uname -m)" = "x86_64" ] || fail "linux/amd64 Flatpak smoke requires a native x86_64 host"
[ -f "$bundle" ] || fail "Flatpak bundle does not exist: $bundle"
[ -n "$expected_version" ] || fail "--version is required"
command -v flatpak >/dev/null 2>&1 || fail "flatpak is required"
command -v xvfb-run >/dev/null 2>&1 || fail "xvfb-run is required"
command -v xwininfo >/dev/null 2>&1 || fail "xwininfo is required"

application_id=io.github.davidhoenisch.remotr.desktop
attempts=${REMOTR_DESKTOP_SMOKE_ATTEMPTS:-100}
interval=${REMOTR_DESKTOP_SMOKE_INTERVAL:-0.1}
case "$attempts" in
	*[!0-9]* | 0 | "") fail "REMOTR_DESKTOP_SMOKE_ATTEMPTS must be a positive integer" ;;
esac

test_root=$(mktemp -d "${TMPDIR:-/tmp}/remotr-desktop-flatpak-smoke.XXXXXX")
permitted_sentinel_value=flatpak-config-sentinel
forbidden_canary_value=REMOTR_FLATPAK_SECRET_CANARY_MUST_NOT_LEAK
installed=0
cleanup() {
	if [ "$installed" -eq 1 ]; then
		flatpak uninstall --user --noninteractive --delete-data "$application_id" >/dev/null 2>&1 || true
	fi
	rm -rf "$test_root"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
mkdir -p "$test_root/home/.config/remotr" "$test_root/home/.ssh" "$test_root/data" "$test_root/cache" "$test_root/config"
export HOME="$test_root/home"
export XDG_DATA_HOME="$test_root/data"
export XDG_CACHE_HOME="$test_root/cache"
export XDG_CONFIG_HOME="$test_root/config"
printf '%s\n' "$permitted_sentinel_value" >"$HOME/.config/remotr/flatpak-smoke-config"
printf '%s\n' "$forbidden_canary_value" >"$HOME/.ssh/remotr-flatpak-forbidden-canary"

flatpak install --user --noninteractive "$bundle" >/dev/null
installed=1
flatpak info --user "$application_id" >/dev/null || fail "installed Flatpak identity is unavailable"
flatpak run --user --command=sh "$application_id" -c '
test -x /app/bin/remotr-desktop
test -f /app/share/applications/io.github.davidhoenisch.remotr.desktop.desktop
test -f /app/share/metainfo/io.github.davidhoenisch.remotr.desktop.metainfo.xml
test -f /app/share/icons/hicolor/256x256/apps/io.github.davidhoenisch.remotr.desktop.png
test -r "$HOME/.config/remotr/flatpak-smoke-config"
test ! -r "$HOME/.ssh/remotr-flatpak-forbidden-canary"
' || fail "installed Flatpak payload is incomplete"

expected_identity="Remotr Desktop $expected_version"
if ! actual_identity=$(flatpak run --user "$application_id" --version 2>&1); then
	fail "could not read embedded identity from installed Flatpak"
fi
[ "$actual_identity" = "$expected_identity" ] ||
	fail "embedded identity is '$actual_identity'; expected '$expected_identity'"

log_file="$test_root/launch.log"
if ! xvfb-run -a sh -c '
application_id=$1
log_file=$2
attempts=$3
interval=$4
window_title=$5

unset WAYLAND_DISPLAY
flatpak run --user --nosocket=wayland --socket=x11 --env=GDK_BACKEND=x11 --env=WEBKIT_DISABLE_COMPOSITING_MODE=1 "$application_id" >"$log_file" 2>&1 &
desktop_pid=$!
stop_desktop() {
	if kill -0 "$desktop_pid" 2>/dev/null; then
		kill "$desktop_pid" 2>/dev/null || true
	fi
	wait "$desktop_pid" 2>/dev/null || true
}
trap stop_desktop EXIT
trap "exit 1" HUP INT TERM

attempt=1
while [ "$attempt" -le "$attempts" ]; do
	sleep "$interval"
	if ! kill -0 "$desktop_pid" 2>/dev/null; then
		wait "$desktop_pid"
		status=$?
		printf "desktop Flatpak smoke: process exited before opening the Remotr Desktop window (status %s)\n" "$status" >&2
		cat "$log_file" >&2
		exit 1
	fi
	if xwininfo -root -tree 2>/dev/null | grep -F "\"$window_title\"" >/dev/null; then
		exit 0
	fi
	attempt=$((attempt + 1))
done

printf "desktop Flatpak smoke: did not observe a Remotr Desktop window after %s attempts\n" "$attempts" >&2
cat "$log_file" >&2
exit 1
' remotr-flatpak-smoke "$application_id" "$log_file" "$attempts" "$interval" "Remotr Desktop"; then
	fail "installed Flatpak window launch verification failed"
fi

if grep -a -F "$forbidden_canary_value" "$log_file" "$bundle" >/dev/null 2>&1; then
	fail "forbidden secret canary leaked into the package or launch log"
fi

flatpak uninstall --user --noninteractive --delete-data "$application_id" >/dev/null
installed=0
if flatpak info --user "$application_id" >/dev/null 2>&1; then
	fail "Flatpak remains installed after removal"
fi
if [ "$(cat "$HOME/.config/remotr/flatpak-smoke-config")" != "$permitted_sentinel_value" ]; then
	fail "permitted host configuration sentinel changed during the lifecycle"
fi
if [ "$(cat "$HOME/.ssh/remotr-flatpak-forbidden-canary")" != "$forbidden_canary_value" ]; then
	fail "forbidden host secret canary changed during the lifecycle"
fi

printf 'desktop Flatpak smoke: unsigned GitHub release asset install/launch/remove smoke passed for linux/amd64 Flatpak %s\n' "$expected_version"
