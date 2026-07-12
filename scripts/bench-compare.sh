#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <before.txt> <after.txt> <output-directory>" >&2
  exit 2
fi

before="$1"
after="$2"
out_dir="$3"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "$out_dir"

compare() {
  local name="$1"
  local filter="$2"
  local output="$out_dir/$name.txt"
  "$script_dir/benchstat.sh" -filter "$filter" "$before" "$after" > "$output"
  if [[ ! -s "$output" ]]; then
    printf 'No %s measurements were emitted by this benchmark pair.\n' "$name" > "$output"
  fi
}

compare latency '.unit:sec/op'
compare allocations '.unit:(B/op OR allocs/op)'
compare payload '.unit:payload_bytes'
compare storage '.unit:storage_bytes'
