#!/bin/sh
set -eu

usage() {
	echo "usage: $0 --check|--wails-tags" >&2
	exit 2
}

mode="${1:-}"
case "$mode" in
	--check|--wails-tags) ;;
	*) usage ;;
esac

if [ "$(uname -s)" != "Linux" ]; then
	echo "Remotr Desktop build and development commands support Linux only." >&2
	exit 1
fi

if ! command -v pkg-config >/dev/null 2>&1; then
	echo "Linux desktop prerequisites are missing: install pkg-config, GTK 3 development files, and WebKitGTK 4.1 or 4.0 development files." >&2
	exit 1
fi

if ! pkg-config --exists gtk+-3.0; then
	echo "GTK 3 development files are missing. Install libgtk-3-dev (Debian/Ubuntu), gtk3-devel (Fedora), or gtk3 (Arch)." >&2
	exit 1
fi

tags=""
if pkg-config --exists webkit2gtk-4.1; then
	tags="webkit2_41"
elif ! pkg-config --exists webkit2gtk-4.0; then
	echo "WebKitGTK 4.1 or 4.0 development files are missing. Install libwebkit2gtk-4.1-dev (Debian/Ubuntu), webkit2gtk4.1-devel (Fedora), or webkit2gtk-4.1 (Arch)." >&2
	exit 1
fi

if [ "$mode" = "--wails-tags" ]; then
	printf '%s\n' "$tags"
fi
