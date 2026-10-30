#!/bin/sh
# Real scanner evidence, with a local synthetic signature database and no network.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
(cd "$root" && CGO_ENABLED=0 go test -mod=vendor -tags=providercontainer -c -o "$work/cronexec.test" ./internal/agent/cronexec)
docker build --file "$root/test/provider-matrix/containers/Dockerfile.clamav-debian-12" --tag remotr-clamav-debian-12:local "$root"
docker run --rm --network none \
  --env REMOTR_CLAMAV_FIXTURE=1 \
  --volume "$work/cronexec.test:/cronexec-test:ro" \
  --volume "$root/test/provider-matrix/fixtures/clamav/setup.py:/fixture-setup.py:ro" \
  remotr-clamav-debian-12:local sh -ec '
    python3 /fixture-setup.py
    clamd
    /cronexec-test -test.run "^TestScheduledClamAVScansPrivateHomes$" -test.v
  '
