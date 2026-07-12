#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <output-file> [go-package ...]" >&2
  exit 2
fi

output="$1"
shift
count="${BENCH_COUNT:-10}"
pattern="${BENCH_PATTERN:-.}"

if [[ $# -eq 0 ]]; then
  set -- \
    ./internal/models \
    ./internal/configrepo \
    ./internal/configcompose \
    ./internal/apppackages \
    ./internal/agent/engine \
    ./internal/agent/sync \
    ./internal/registry \
    ./test/acceptance
fi

mkdir -p "$(dirname "$output")"
go test -mod=vendor -run='^$' -bench="$pattern" -benchmem -count="$count" "$@" > "$output"
