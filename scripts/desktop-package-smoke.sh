#!/bin/sh
set -eu

fail() {
	printf 'desktop DEB smoke: %s\n' "$1" >&2
	exit 1
}

deb=""
expected_version=""
native_smoke=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--package)
			[ "$#" -ge 2 ] || fail "--package requires a path"
			deb=$2
			shift 2
			;;
		--version)
			[ "$#" -ge 2 ] || fail "--version requires a value"
			expected_version=$2
			shift 2
			;;
		--native-smoke)
			[ "$#" -ge 2 ] || fail "--native-smoke requires a path"
			native_smoke=$2
			shift 2
			;;
		*) fail "unknown argument: $1" ;;
	esac
done

[ "$(uname -s)" = "Linux" ] || fail "only Linux package smoke is supported"
[ "$(dpkg --print-architecture)" = "amd64" ] || fail "linux/amd64 DEB smoke requires a native amd64 host"
[ -f "$deb" ] || fail "DEB package does not exist: $deb"
[ -x "$native_smoke" ] || fail "native smoke harness is not executable: $native_smoke"
[ -n "$expected_version" ] || fail "--version is required"
command -v dpkg-deb >/dev/null 2>&1 || fail "dpkg-deb is required"
command -v docker >/dev/null 2>&1 || fail "Docker is required for isolated native package-manager lifecycle smoke"
command -v desktop-file-validate >/dev/null 2>&1 || fail "desktop-file-validate is required"
command -v appstreamcli >/dev/null 2>&1 || fail "appstreamcli is required"

[ "$(dpkg-deb --field "$deb" Package)" = "remotr-desktop" ] || fail "package identity is not remotr-desktop"
actual_version=$(dpkg-deb --field "$deb" Version)
[ "$actual_version" = "$expected_version" ] ||
	fail "package version is '$actual_version'; expected '$expected_version'"
[ "$(dpkg-deb --field "$deb" Architecture)" = "amd64" ] || fail "package architecture is not amd64"

deb_dir=$(CDPATH= cd -- "$(dirname -- "$deb")" && pwd)
deb="$deb_dir/$(basename -- "$deb")"
if ! container_output=$(docker run --rm --platform linux/amd64 \
	-v "$deb:/tmp/remotr-desktop.deb:ro" \
	debian:13-slim@sha256:020c0d20b9880058cbe785a9db107156c3c75c2ac944a6aa7ab59f2add76a7bd sh -eu -c '
dpkg --force-depends --install /tmp/remotr-desktop.deb
dpkg-query --show remotr-desktop >/dev/null
test -x /usr/bin/remotr-desktop
test -f /usr/share/applications/remotr-desktop.desktop
test -f /usr/share/metainfo/io.github.davidhoenisch.remotr.desktop.metainfo.xml
test -f /usr/share/icons/hicolor/256x256/apps/remotr-desktop.png
dpkg --force-depends --purge remotr-desktop
test ! -e /usr/bin/remotr-desktop
test ! -e /usr/share/applications/remotr-desktop.desktop
test ! -e /usr/share/metainfo/io.github.davidhoenisch.remotr.desktop.metainfo.xml
test ! -e /usr/share/icons/hicolor/256x256/apps/remotr-desktop.png
if dpkg-query --show remotr-desktop >/dev/null 2>&1; then
	exit 1
fi
printf "container package-manager install/remove passed\n"
' 2>&1); then
	printf '%s\n' "$container_output" >&2
	fail "native linux/amd64 package-manager install/remove failed"
fi
printf '%s\n' "$container_output"
case "$container_output" in
	*"container package-manager install/remove passed"*) ;;
	*) fail "container package-manager lifecycle did not report completion" ;;
esac

install_root=$(mktemp -d "${TMPDIR:-/tmp}/remotr-desktop-install.XXXXXX")
cleanup() {
	rm -rf "$install_root"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

dpkg-deb --extract "$deb" "$install_root"

installed_binary="$install_root/usr/bin/remotr-desktop"
installed_desktop="$install_root/usr/share/applications/remotr-desktop.desktop"
installed_metainfo="$install_root/usr/share/metainfo/io.github.davidhoenisch.remotr.desktop.metainfo.xml"
installed_icon="$install_root/usr/share/icons/hicolor/256x256/apps/remotr-desktop.png"
for installed_path in "$installed_binary" "$installed_desktop" "$installed_metainfo" "$installed_icon"; do
	[ -e "$installed_path" ] || fail "installed payload is missing $installed_path"
done

desktop-file-validate "$installed_desktop"
appstreamcli validate --no-net "$installed_metainfo" >/dev/null
"$native_smoke" --binary "$installed_binary" --version "$expected_version"

printf 'desktop DEB smoke: unsigned development snapshot install/launch/remove smoke passed for linux/amd64 DEB %s\n' "$expected_version"
