#!/usr/bin/env bash
set -euo pipefail

# Usage: scripts/fuzz-affected.sh <base-revision> [duration]
# Runs short active fuzzing only for changed packages that own native Fuzz*
# targets. Other changed packages still run their seed corpora through `make
# test`; they are not treated as a no-match failure here.

base_revision=${1:?usage: fuzz-affected.sh <base-revision> [duration]}
duration=${2:-10s}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

declare -A changed_packages
while IFS= read -r path
do
  case "$path" in
    *.go)
      package="./${path%/*}"
      test "$package" = "./." && package=.
      changed_packages["$package"]=1
      ;;
  esac
done < <(git diff --name-only "$base_revision...HEAD" -- ':!vendor/**')

declare -A fuzz_packages
while IFS= read -r source
do
  package="./${source%/*}"
  test "$package" = "./." && package=.
  if rg -q '^[[:space:]]*func[[:space:]]+Fuzz[A-Za-z0-9_]+[[:space:]]*\(' "$source"
  then
    fuzz_packages["$package"]=1
  fi
done < <(rg --files -g '*_test.go' -g '!vendor/**')

selected=()
for package in "${!changed_packages[@]}"
do
  if test -n "${fuzz_packages[$package]:-}"
  then
    selected+=("$package")
  fi
done

if ((${#selected[@]} == 0))
then
  echo "no changed package owns a native fuzz target"
  exit 0
fi

IFS=$'\n' selected=($(sort <<<"${selected[*]}"))
unset IFS
exec ./scripts/fuzz-all.sh "$duration" "${selected[@]}"
