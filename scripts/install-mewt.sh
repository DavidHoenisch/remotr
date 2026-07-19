#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <installation-directory>" >&2
  exit 2
fi

destination="$1"
version="3.0.1"
archive="mewt-x86_64-unknown-linux-gnu.tar.xz"
digest="4e4b589b1744bc30b2cbd9ca21f8e2ab2527bce56dbea856621ed804451f4703"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

curl --fail --location --output "$scratch/$archive" \
  "https://github.com/trailofbits/mewt/releases/download/v${version}/${archive}"
printf '%s *%s\n' "$digest" "$scratch/$archive" | sha256sum --check --status
mkdir -p "$destination"
tar -xf "$scratch/$archive" -C "$destination" --strip-components=1

binary="$destination/mewt"
if [[ ! -x "$binary" ]] || [[ "$($binary --version)" != "mewt $version" ]]; then
  echo "install-mewt: verified archive did not contain mewt $version" >&2
  exit 1
fi
printf '%s\n' "$binary"
