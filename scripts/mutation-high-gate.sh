#!/usr/bin/env bash
set -euo pipefail

MEWT_BIN="${MEWT:-mewt}"
TARGET_FILE="${MUTATION_TARGET_FILE:-test/mutation/critical-targets.txt}"
EVIDENCE_FILE="${MUTATION_EVIDENCE_FILE:-artifacts/mutation/high-gate.json}"
REUSE_OUTCOMES="${MUTATION_REUSE_OUTCOMES:-false}"

remove_db=false
if [[ -n "${MUTATION_DB:-}" ]]; then
  mutation_db="$MUTATION_DB"
else
  mutation_db_dir="$(mktemp -d)"
  mutation_db="$mutation_db_dir/mewt.sqlite"
  remove_db=true
fi

if [[ ! -x "$MEWT_BIN" ]] && ! command -v "$MEWT_BIN" >/dev/null 2>&1; then
  echo "mutation-high-gate: set MEWT to the verified Mewt 3.0.1 executable" >&2
  exit 1
fi
if [[ "$($MEWT_BIN --version)" != "mewt 3.0.1" ]]; then
  echo "mutation-high-gate: expected mewt 3.0.1" >&2
  exit 1
fi

mapfile -t targets < <(sed -E '/^[[:space:]]*(#|$)/d' "$TARGET_FILE")
if (( ${#targets[@]} == 0 )); then
  echo "mutation-high-gate: no critical targets" >&2
  exit 1
fi
if ! git diff --quiet -- "${targets[@]}"; then
  echo "mutation-high-gate: critical source targets must be clean before mutation" >&2
  exit 1
fi

verify_restored() {
  if ! git diff --quiet -- "${targets[@]}"; then
    echo "mutation-high-gate: Mewt left a critical target modified" >&2
    return 1
  fi
  if [[ "$remove_db" == true ]]; then
    rm -rf "$mutation_db_dir"
  fi
}
trap verify_restored EXIT

if [[ "$REUSE_OUTCOMES" != true ]]; then
  "$MEWT_BIN" mutate --db "$mutation_db" "${targets[@]}"
fi
mkdir -p "$(dirname "$EVIDENCE_FILE")"
evidence_tmp="${EVIDENCE_FILE}.tmp"
printf '{"schemaVersion":1,"tool":"mewt@3.0.1","blockingSeverity":"high","targets":[' > "$evidence_tmp"

total=0
first=true
for target in "${targets[@]}"; do
  mapfile -t ids < <(
    "$MEWT_BIN" print mutants --db "$mutation_db" --target "$target" --severity high |
      sed -nE 's/.*\[[[:upper:]]+ ([0-9]+)\].*/\1/p'
  )
  if (( ${#ids[@]} == 0 )); then
    echo "mutation-high-gate: $target has no generated high-severity mutants" >&2
    exit 1
  fi
  if [[ "$REUSE_OUTCOMES" != true ]]; then
    existing_results="$($MEWT_BIN results --db "$mutation_db" --target "$target" 2>/dev/null || true)"
    pending_ids=()
    for id in "${ids[@]}"; do
      if ! grep -Eq "TestFail[[:space:]]+\| \[[[:upper:]]+ ${id}\]" <<<"$existing_results"; then
        pending_ids+=("$id")
      fi
    done
    if (( ${#pending_ids[@]} > 0 )); then
      ids_file="$(mktemp)"
      printf '%s\n' "${pending_ids[@]}" > "$ids_file"
      "$MEWT_BIN" test --db "$mutation_db" --ids-file "$ids_file"
      rm -f "$ids_file"
    fi
  fi

  results="$($MEWT_BIN results --db "$mutation_db" --target "$target")"
  for id in "${ids[@]}"; do
    if ! grep -Eq "TestFail[[:space:]]+\| \[[[:upper:]]+ ${id}\]" <<<"$results"; then
      echo "mutation-high-gate: unexplained high-severity mutant $id survived for $target" >&2
      exit 1
    fi
  done
  if [[ "$first" == false ]]; then
    printf ',' >> "$evidence_tmp"
  fi
  first=false
  printf '{"target":"%s","selected":%d,"caught":%d}' "$target" "${#ids[@]}" "${#ids[@]}" >> "$evidence_tmp"
  total=$((total + ${#ids[@]}))
  echo "mutation-high-gate: $target caught ${#ids[@]}/${#ids[@]}"
done

printf '],"selected":%d,"caught":%d,"unexplainedSurvivors":0}\n' "$total" "$total" >> "$evidence_tmp"
mv "$evidence_tmp" "$EVIDENCE_FILE"
verify_restored
trap - EXIT
if [[ "$remove_db" == true ]]; then
  rm -rf "$mutation_db_dir"
fi
echo "mutation-high-gate: caught $total/$total adopted high-severity mutants"
