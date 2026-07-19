#!/usr/bin/env bash
set -euo pipefail

MEWT_BIN="${MEWT:-mewt}"
EXPECTED_VERSION="mewt 3.0.1"

targets=(
  internal/capabilitydoc/validation.go
  internal/artifactrequirements/set.go
  internal/artifactvariant/compatibility.go
  internal/server/legacy_capability_profiles.go
  internal/server/composition.go
  internal/server/server.go
)

if [[ ! -x "$MEWT_BIN" ]] && ! command -v "$MEWT_BIN" >/dev/null 2>&1; then
  echo "mutation-capability-selection: set MEWT to the verified Mewt 3.0.1 executable" >&2
  exit 1
fi
if [[ "$($MEWT_BIN --version)" != "$EXPECTED_VERSION" ]]; then
  echo "mutation-capability-selection: expected $EXPECTED_VERSION" >&2
  exit 1
fi
if ! git diff --quiet -- "${targets[@]}"; then
  echo "mutation-capability-selection: target source files must be clean before mutation" >&2
  exit 1
fi

high_ids() {
  "$MEWT_BIN" print mutants --target "$1" --severity high |
    sed -nE 's/.*\[[[:upper:]]+ ([0-9]+)\].*/\1/p'
}

scoped_high_ids() {
  local target="$1"
  shift
  local id start end bounds low high
  while read -r id start end; do
    [[ -n "$id" ]] || continue
    end="${end:-$start}"
    for bounds in "$@"; do
      low="${bounds%%:*}"
      high="${bounds##*:}"
      if (( end >= low && start <= high )); then
        echo "$id"
        break
      fi
    done
  done < <(
    "$MEWT_BIN" print mutants --target "$target" --severity high |
      sed -nE 's/.*\[[[:upper:]]+ ([0-9]+)\] Lines? ([0-9]+)(-([0-9]+))?.*/\1 \2 \4/p'
  )
}

total=0
run_group() {
  local target="$1"
  local expected="$2"
  shift 2
  local ids=("$@")
  if (( ${#ids[@]} != expected )); then
    echo "mutation-capability-selection: $target selected ${#ids[@]} mutants; expected $expected; review the versioned scope" >&2
    exit 1
  fi

  local csv results id
  csv="$(IFS=,; echo "${ids[*]}")"
  "$MEWT_BIN" test --ids "$csv"
  results="$($MEWT_BIN results --target "$target")"
  for id in "${ids[@]}"; do
    if ! grep -Eq "TestFail[[:space:]]+\\| \\[[[:upper:]]+ ${id}\\]" <<<"$results"; then
      echo "mutation-capability-selection: mutant $id for $target was not caught" >&2
      exit 1
    fi
  done
  total=$((total + ${#ids[@]}))
  echo "mutation-capability-selection: $target caught ${#ids[@]}/${#ids[@]}"
}

"$MEWT_BIN" mutate "${targets[@]}"

mapfile -t ids < <(high_ids internal/capabilitydoc/validation.go)
run_group internal/capabilitydoc/validation.go 28 "${ids[@]}"
mapfile -t ids < <(high_ids internal/artifactrequirements/set.go)
run_group internal/artifactrequirements/set.go 27 "${ids[@]}"
mapfile -t ids < <(high_ids internal/artifactvariant/compatibility.go)
run_group internal/artifactvariant/compatibility.go 15 "${ids[@]}"
mapfile -t ids < <(high_ids internal/server/legacy_capability_profiles.go)
run_group internal/server/legacy_capability_profiles.go 6 "${ids[@]}"
mapfile -t ids < <(scoped_high_ids internal/server/composition.go 226:255)
run_group internal/server/composition.go 8 "${ids[@]}"
mapfile -t ids < <(scoped_high_ids internal/server/server.go 304:417 464:487 516:575 646:784)
run_group internal/server/server.go 59 "${ids[@]}"

if (( total != 143 )); then
  echo "mutation-capability-selection: selected $total mutants; expected 143" >&2
  exit 1
fi
if ! git diff --quiet -- "${targets[@]}"; then
  echo "mutation-capability-selection: Mewt left a target source file modified" >&2
  exit 1
fi

echo "mutation-capability-selection: caught 143/143 focused high-severity mutants"
