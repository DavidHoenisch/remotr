#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image='remotr-provider-arch-2026-07-06:local'
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

"$root/scripts/verify-package-provider-fixtures.sh"

(
  cd "$root"
  CGO_ENABLED=0 go test -mod=vendor -tags=providercontainer -c \
    -o "$work/aur-provider.test" ./internal/applicators/packages/aur
)

docker build \
  --file "$root/test/provider-matrix/containers/Dockerfile.arch-2026-07-06" \
  --tag "$image" \
  "$root"

docker run --rm --platform linux/amd64 \
  --env REMOTR_PROVIDER_RELEASE=2026-07-06 \
  --volume "$work/aur-provider.test:/provider-test:ro" \
  --volume "$root/test/provider-matrix/fixtures/core-packages:/fixtures:ro" \
  --volume "$root/test/provider-matrix/fixtures/core-packages/aur/yay-controlled-fixture:/usr/local/bin/yay:ro" \
  "$image" \
  /provider-test -test.run '^TestProviderContainerAURContract$' -test.v
