#!/usr/bin/env bash
set -euo pipefail

# Usage: scripts/fuzz-all.sh [duration] [package ...]
#        scripts/fuzz-all.sh --seed-corpora [package ...]
#
# Without packages, every native Fuzz* function committed to the repository is
# exercised. With packages, the same source discovery is limited to the named
# packages, which keeps affected-package PR fuzzing bounded.

cd "$(dirname "$0")/.."

mode="fuzz"
if [[ "${1:-}" == "--seed-corpora" ]]; then
  mode="seed-corpora"
  fuzztime="seed corpora"
  shift
else
  fuzztime="${1:-30s}"
  shift || true
fi
selected_packages=("$@")

die() {
  echo "fuzz discovery: $*" >&2
  exit 1
}

declare -A discovered
targets=()

while IFS= read -r source; do

  source_dir="${source%/*}"
  [[ "$source_dir" != "$source" ]] || source_dir="."

  # A nested go.mod owns its own test surface. Root fuzz discovery must not
  # hand that package to the root module's vendored `go test` invocation.
  ancestor="$source_dir"
  nested_module=false
  while [[ "$ancestor" != "." ]]; do
    if [[ -f "$ancestor/go.mod" ]]; then
      nested_module=true
      break
    fi
    parent="${ancestor%/*}"
    [[ "$parent" != "$ancestor" ]] || parent="."
    ancestor="$parent"
  done
  [[ "$nested_module" == false ]] || continue

  package="./${source_dir}"
  [[ "$package" == "./." ]] && package="."

  while IFS=$'\t' read -r target_package target; do
    [[ -z "$target" ]] && continue
    key="${target_package}:${target}"
    [[ -z "${discovered[$key]:-}" ]] || die "duplicate native target ${key}"
    discovered["$key"]=1
    targets+=("$key")
  done < <(
    awk -v package="$package" '
      /^[[:space:]]*func[[:space:]]+Fuzz[A-Za-z0-9_]+[[:space:]]*\(/ {
        name = $0
        sub(/^[[:space:]]*func[[:space:]]+/, "", name)
        sub(/[[:space:]]*\(.*/, "", name)
        print package "\t" name
      }
    ' "$source"
  )
done < <(git ls-files -- '*_test.go' ':!vendor/**')

((${#targets[@]} > 0)) || die "no native Fuzz* targets discovered"

selected=()
for entry in "${targets[@]}"; do
  package="${entry%%:*}"
  if ((${#selected_packages[@]} == 0)); then
    selected+=("$entry")
    continue
  fi
  for requested in "${selected_packages[@]}"; do
    if [[ "$package" == "$requested" ]]; then
      selected+=("$entry")
      break
    fi
  done
done

((${#selected[@]} > 0)) || die "package selection matched no discovered native fuzz target"

for entry in "${selected[@]}"; do
  package="${entry%%:*}"
  target="${entry##*:}"

  # Go reports an unmatched -fuzz expression as a successful warning. Listing
  # first turns that case into a hard error and also catches build-tag drift.
  listing="$(go test -mod=vendor "$package" -list "^${target}$" 2>&1)" \
    || die "cannot list ${entry}: ${listing}"
  grep -Fxq "$target" <<<"$listing" \
    || die "discovered ${entry}, but go test found no matching fuzz target"

  if [[ "$mode" == "seed-corpora" ]]; then
    # In ordinary Go test mode a Fuzz function runs its seed corpus. Running it
    # by its discovered name makes that promise explicit and auditable.
    echo "==> fuzz seed corpus ${target} (${package})"
    go test -mod=vendor "$package" -run "^${target}$" -count=1
  else
    echo "==> fuzz ${target} (${package}, ${fuzztime})"
    go test -mod=vendor "$package" -run '^$' -fuzz="^${target}$" \
      -fuzztime="$fuzztime" -count=1
  fi
done

echo "completed ${#selected[@]} discovered fuzz target(s) (${fuzztime})"
