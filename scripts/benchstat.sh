#!/usr/bin/env bash
set -euo pipefail

# Use a pinned, temporary benchstat tool unless the caller explicitly provides
# a controlled binary through BENCHSTAT_BIN. This leaves no developer-global
# executable or module cache behind.
BENCHSTAT_VERSION="v0.0.0-20260709024250-82a0b07e230d"

if [[ -n "${BENCHSTAT_BIN:-}" ]]; then
  exec "$BENCHSTAT_BIN" "$@"
fi

cache_dir="$(mktemp -d)"
cleanup() {
  chmod -R u+w "$cache_dir" 2>/dev/null || true
  rm -rf "$cache_dir"
}
trap cleanup EXIT

GOFLAGS="${GOFLAGS:-} -modcacherw" \
  GOMODCACHE="$cache_dir/modcache" \
  GOCACHE="$cache_dir/gocache" \
  go run "golang.org/x/perf/cmd/benchstat@${BENCHSTAT_VERSION}" "$@"
