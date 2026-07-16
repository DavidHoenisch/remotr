#!/bin/sh
set -eu

fail() {
	printf 'desktop Flatpak build: %s\n' "$1" >&2
	exit 1
}

binary=""
version=""
architecture=""
output=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--binary)
			[ "$#" -ge 2 ] || fail "--binary requires a path"
			binary=$2
			shift 2
			;;
		--version)
			[ "$#" -ge 2 ] || fail "--version requires a value"
			version=$2
			shift 2
			;;
		--architecture)
			[ "$#" -ge 2 ] || fail "--architecture requires a value"
			architecture=$2
			shift 2
			;;
		--output)
			[ "$#" -ge 2 ] || fail "--output requires a path"
			output=$2
			shift 2
			;;
		*) fail "unknown argument: $1" ;;
	esac
done

[ "$(uname -s)" = "Linux" ] || fail "only Linux package builds are supported"
[ "$(uname -m)" = "x86_64" ] || fail "linux/amd64 Flatpak builds require a native x86_64 host"
[ "$architecture" = "amd64" ] || fail "only linux/amd64 Flatpak release assets are advertised"
[ -x "$binary" ] || fail "binary is not executable: $binary"
[ -n "$output" ] || fail "--output is required"
case "$output" in
	*.flatpak) ;;
	*) fail "--output must end in .flatpak" ;;
esac
case "$version" in
	[0-9]*) ;;
	*) fail "version must begin with a digit" ;;
esac
case "$version" in
	*[!0-9A-Za-z.+~_-]*) fail "version contains unsupported Flatpak artifact characters" ;;
esac

expected_identity="Remotr Desktop $version"
if ! actual_identity=$("$binary" --version 2>&1); then
	fail "could not read the embedded binary identity"
fi
[ "$actual_identity" = "$expected_identity" ] ||
	fail "embedded identity is '$actual_identity'; expected '$expected_identity'"

command -v flatpak >/dev/null 2>&1 || fail "flatpak is required"
command -v flatpak-builder >/dev/null 2>&1 || fail "flatpak-builder is required"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
manifest="$repo_root/desktop/build/linux/flatpak/io.github.davidhoenisch.remotr.desktop.json"
[ -f "$manifest" ] || fail "Flatpak manifest does not exist: $manifest"

stage=$(mktemp -d "${TMPDIR:-/tmp}/remotr-desktop-flatpak.XXXXXX")
cleanup() {
	rm -rf "$stage"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

flatpak-builder \
	--user \
	--force-clean \
	--disable-rofiles-fuse \
	--state-dir="$stage/state" \
	--arch=x86_64 \
	--default-branch=stable \
	--repo="$stage/repo" \
	"$stage/build" \
	"$manifest"
flatpak build-bundle \
	--arch=x86_64 \
	--runtime-repo=https://dl.flathub.org/repo/flathub.flatpakrepo \
	"$stage/repo" \
	"$stage/remotr-desktop.flatpak" \
	io.github.davidhoenisch.remotr.desktop \
	stable

mkdir -p "$(dirname -- "$output")"
install -m 0644 "$stage/remotr-desktop.flatpak" "$output"
printf 'desktop Flatpak build: wrote unsigned GitHub release asset %s\n' "$output"
