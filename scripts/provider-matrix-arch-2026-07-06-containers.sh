#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
"$root/scripts/verify-package-provider-fixtures.sh"
qualification_runtime=$(mktemp -d)
trap 'rm -rf "$qualification_runtime"' EXIT INT TERM
file_provider_test="$qualification_runtime/remotr-file-provider.test"
download_provider_test="$qualification_runtime/remotr-download-provider.test"
(
  cd "$root"
  CGO_ENABLED=0 go test -mod=vendor -c -o "$file_provider_test" ./internal/applicators/files
  CGO_ENABLED=0 go test -mod=vendor -c -o "$download_provider_test" ./internal/applicators/downloads
)

image_name=arch-2026-07-06
image="remotr-provider-${image_name}:local"
docker build --file "$root/test/provider-matrix/containers/Dockerfile.${image_name}" --tag "$image" "$root"
docker run --rm "$image" sh -eu -c '
  . /etc/os-release
  test "$ID" = arch
  test "${REMOTR_PROVIDER_IMAGE_RELEASE}" = 2026-07-06
  command -v pacman
  test "$(uname -m)" = x86_64
  pacman --version >/dev/null
'
docker run --rm \
  --volume "$file_provider_test:/usr/local/lib/remotr-file-provider.test:ro" \
  --volume "$download_provider_test:/usr/local/lib/remotr-download-provider.test:ro" \
  "$image" \
  sh -eu -c '
    /usr/local/lib/remotr-file-provider.test -test.count=1 -test.v
    /usr/local/lib/remotr-download-provider.test -test.count=1 -test.v
  '
