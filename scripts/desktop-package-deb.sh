#!/bin/sh
set -eu

fail() {
	printf 'desktop DEB build: %s\n' "$1" >&2
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
[ "$architecture" = "amd64" ] || fail "only linux/amd64 DEB snapshots are advertised"
[ "$(dpkg --print-architecture)" = "amd64" ] || fail "linux/amd64 DEB snapshots require a native amd64 host"
[ -x "$binary" ] || fail "binary is not executable: $binary"
[ -n "$output" ] || fail "--output is required"
case "$output" in
	*.deb) ;;
	*) fail "--output must end in .deb" ;;
esac
case "$version" in
	[0-9]*) ;;
	*) fail "version must begin with a digit" ;;
esac
case "$version" in
	*[!0-9A-Za-z.+~_-]*) fail "version contains unsupported DEB characters" ;;
esac

expected_identity="Remotr Desktop $version"
if ! actual_identity=$("$binary" --version 2>&1); then
	fail "could not read the embedded binary identity"
fi
[ "$actual_identity" = "$expected_identity" ] ||
	fail "embedded identity is '$actual_identity'; expected '$expected_identity'"

command -v dpkg-deb >/dev/null 2>&1 || fail "dpkg-deb is required"
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(dirname -- "$script_dir")
linux_assets="$repo_root/desktop/build/linux"

stage=$(mktemp -d "${TMPDIR:-/tmp}/remotr-desktop-deb.XXXXXX")
cleanup() {
	rm -rf "$stage"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

install -D -m 0755 "$binary" "$stage/usr/bin/remotr-desktop"
install -D -m 0644 "$linux_assets/remotr-desktop.desktop" "$stage/usr/share/applications/remotr-desktop.desktop"
install -D -m 0644 "$linux_assets/io.github.davidhoenisch.remotr.desktop.metainfo.xml" "$stage/usr/share/metainfo/io.github.davidhoenisch.remotr.desktop.metainfo.xml"
install -D -m 0644 "$linux_assets/icons/hicolor/256x256/apps/remotr-desktop.png" "$stage/usr/share/icons/hicolor/256x256/apps/remotr-desktop.png"
install -D -m 0644 "$linux_assets/debian/copyright" "$stage/usr/share/doc/remotr-desktop/copyright"
install -d -m 0755 "$stage/DEBIAN"

installed_size=$(du -sk "$stage/usr" | awk '{print $1}')
sed \
	-e "s/@VERSION@/$version/g" \
	-e "s/@ARCHITECTURE@/$architecture/g" \
	-e "s/@INSTALLED_SIZE@/$installed_size/g" \
	"$linux_assets/debian/control.in" >"$stage/DEBIAN/control"

mkdir -p "$(dirname -- "$output")"
dpkg-deb --root-owner-group --build "$stage" "$output" >/dev/null
printf 'desktop DEB build: wrote unsigned development snapshot %s\n' "$output"
