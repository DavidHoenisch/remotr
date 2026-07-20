#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
"$root/scripts/verify-package-provider-fixtures.sh"
qualification_runtime=$(mktemp -d)
trap 'rm -rf "$qualification_runtime"' EXIT INT TERM
file_provider_test="$qualification_runtime/remotr-file-provider.test"
download_provider_test="$qualification_runtime/remotr-download-provider.test"
directory_provider_test="$qualification_runtime/remotr-directory-provider.test"
link_provider_test="$qualification_runtime/remotr-link-provider.test"
known_host_provider_test="$qualification_runtime/remotr-known-host-provider.test"
registry_provider_test="$qualification_runtime/remotr-registry-provider.test"
secret_api_test="$qualification_runtime/remotr-secret-api.test"
cron_provider_test="$qualification_runtime/remotr-cron-provider.test"
(
  cd "$root"
  CGO_ENABLED=0 go test -mod=vendor -c -o "$file_provider_test" ./internal/applicators/files
  CGO_ENABLED=0 go test -mod=vendor -c -o "$download_provider_test" ./internal/applicators/downloads
  CGO_ENABLED=0 go test -mod=vendor -tags=providerintegration -c -o "$directory_provider_test" ./internal/applicators/directories
  CGO_ENABLED=0 go test -mod=vendor -c -o "$link_provider_test" ./internal/applicators/links
  CGO_ENABLED=0 go test -mod=vendor -c -o "$known_host_provider_test" ./internal/applicators/knownhosts
  CGO_ENABLED=0 go test -mod=vendor -c -o "$registry_provider_test" ./internal/resourceregistry
  CGO_ENABLED=0 go test -mod=vendor -c -o "$secret_api_test" ./internal/server
  CGO_ENABLED=0 go test -mod=vendor -tags=providerintegration -c -o "$cron_provider_test" ./internal/applicators/endpointschedules/cron
)

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
    test "$(uname -m)" = x86_64
    apt-get --version >/dev/null
    dpkg --version >/dev/null
  ' sh "$name" "$release"
  if test "$name" = ubuntu
  then
    docker run --rm \
      --volume "$file_provider_test:/usr/local/lib/remotr-file-provider.test:ro" \
      --volume "$download_provider_test:/usr/local/lib/remotr-download-provider.test:ro" \
      --volume "$directory_provider_test:/usr/local/lib/remotr-directory-provider.test:ro" \
      --volume "$link_provider_test:/usr/local/lib/remotr-link-provider.test:ro" \
      --volume "$known_host_provider_test:/usr/local/lib/remotr-known-host-provider.test:ro" \
      --volume "$registry_provider_test:/usr/local/lib/remotr-registry-provider.test:ro" \
      --volume "$secret_api_test:/usr/local/lib/remotr-secret-api.test:ro" \
      --volume "$cron_provider_test:/usr/local/lib/remotr-cron-provider.test:ro" \
      --tmpfs /qualification/root/mounted:rw,noexec,nosuid,nodev \
      "$image" \
      sh -eu -c '
        /usr/local/lib/remotr-file-provider.test -test.count=1 -test.v
        /usr/local/lib/remotr-download-provider.test -test.count=1 -test.v
        /usr/local/lib/remotr-directory-provider.test -test.count=1 -test.v
        /usr/local/lib/remotr-link-provider.test -test.count=1 -test.v
        /usr/local/lib/remotr-known-host-provider.test -test.count=1 -test.v
        /usr/local/lib/remotr-registry-provider.test -test.count=1 -test.v -test.run "^TestRegistryDownloadProviderResolvesScopedAuthentication$"
        /usr/local/lib/remotr-secret-api.test -test.count=1 -test.v -test.run "^TestResolveSecretAuthorizesEndpointArtifactResourceAndPurpose$"
        /usr/local/lib/remotr-cron-provider.test -test.count=1 -test.v -test.run "^TestCronProviderUbuntuContainer$"
      '
  fi
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
