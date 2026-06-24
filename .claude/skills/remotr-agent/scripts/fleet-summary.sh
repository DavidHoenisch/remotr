#!/usr/bin/env bash
# Summarize fleet inventory and compliance. Requires bootstrapped operator CLI.
set -euo pipefail

FLEET="${1:-}"
if [[ -z "$FLEET" ]]; then
  if [[ -n "${REMOTR_FLEET:-}" ]]; then
    FLEET="$REMOTR_FLEET"
  else
    echo "usage: fleet-summary.sh <fleet-name>" >&2
    echo "  or set REMOTR_FLEET" >&2
    exit 2
  fi
fi

echo "== endpoints =="
remotr endpoint list

echo
echo "== state report: ${FLEET} =="
remotr fleet state report --fleet "$FLEET" || ec=$?
ec=${ec:-0}

echo
echo "== cron report: ${FLEET} =="
remotr fleet cron report --fleet "$FLEET" || true

exit "$ec"
