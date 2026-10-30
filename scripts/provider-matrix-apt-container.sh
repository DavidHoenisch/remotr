#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <image-name> <distribution> <release>" >&2
  exit 2
fi

image_name=$1
distribution=$2
release=$3
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image="remotr-provider-${image_name}:local"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM

"$root/scripts/verify-package-provider-fixtures.sh"

(
  cd "$root"
  CGO_ENABLED=0 go test -mod=vendor -tags=providercontainer -c \
    -o "$work/apt-provider.test" ./internal/applicators/packages/apt
)

docker build \
  --file "$root/test/provider-matrix/containers/Dockerfile.${image_name}" \
  --tag "$image" \
  "$root"

docker run --rm --platform linux/amd64 \
  --env "REMOTR_PROVIDER_DISTRO=$distribution" \
  --env "REMOTR_PROVIDER_RELEASE=$release" \
  --volume "$work/apt-provider.test:/provider-test:ro" \
  --volume "$root/test/provider-matrix/fixtures/core-packages:/fixtures:ro" \
  "$image" \
  /provider-test -test.run '^TestProviderContainerAPTContract$' -test.v

# Exercise the server-dispatched Debian upgrade builtin through the real cron
# runner against a local repository containing an independently versioned fixture.
if [ "$distribution" = debian ]; then
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=providercontainer -c \
      -o "$work/cronexec.test" ./internal/agent/cronexec
  )
  docker run --rm --platform linux/amd64 \
    --env "REMOTR_PROVIDER_DISTRO=$distribution" \
    --volume "$work/cronexec.test:/cronexec-test:ro" \
    --volume "$root/test/provider-matrix/fixtures/core-packages:/fixtures:ro" \
    "$image" sh -ec '
      rm -f /etc/apt/sources.list.d/*
      printf "deb [trusted=yes] file:/fixtures/apt stable main\n" > /etc/apt/sources.list
      dpkg -i /fixtures/apt/pool/main/r/remotr-fixture/remotr-fixture_1.0.0-1_amd64.deb
      /cronexec-test -test.run "^Test(SkippedCronResourceDoesNotReportSuccess|ScheduledUpgradeExecutesRealAPT)$" -test.v
    '
fi
