#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
"$root/scripts/verify-package-provider-fixtures.sh"
run_environment() {
  image_name=$1
  name=$2
  release=$3
  image="remotr-provider-${image_name}:local"
  docker build --file "$root/test/provider-matrix/containers/Dockerfile.${image_name}" --tag "$image" "$root"
  docker run --rm "$image" sh -eu -c '
    . /etc/os-release
    test "$ID" = "$1"
    test "$VERSION_ID" = "$2"
    command -v apt-get
    command -v dpkg
    apt-get --version >/dev/null
    dpkg --version >/dev/null
  ' sh "$name" "$release"
}

run_environment debian-12 debian 12
run_environment ubuntu-24.04 ubuntu 24.04

arch_image="remotr-provider-arch-2026-07-06:local"
docker build --file "$root/test/provider-matrix/containers/Dockerfile.arch-2026-07-06" --tag "$arch_image" "$root"
docker run --rm "$arch_image" sh -eu -c '
  . /etc/os-release
  test "$ID" = "arch"
  command -v pacman
  pacman --version >/dev/null
  ! command -v yay
'
