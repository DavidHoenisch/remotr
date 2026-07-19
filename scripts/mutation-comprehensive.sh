#!/usr/bin/env bash
set -euo pipefail

MEWT_BIN="${MEWT:-mewt}"
TARGET_FILE="${MUTATION_TARGET_FILE:-test/mutation/critical-targets.txt}"
EVIDENCE_DIR="${MUTATION_EVIDENCE_DIR:-artifacts/mutation/comprehensive}"

if [[ "$($MEWT_BIN --version 2>/dev/null)" != "mewt 3.0.1" ]]; then
  echo "mutation-comprehensive: set MEWT to the verified Mewt 3.0.1 executable" >&2
  exit 1
fi
mapfile -t targets < <(sed -E '/^[[:space:]]*(#|$)/d' "$TARGET_FILE")
mkdir -p "$EVIDENCE_DIR"
mutation_db="${MUTATION_DB:-$EVIDENCE_DIR/mewt.sqlite}"
"$MEWT_BIN" run --db "$mutation_db" --comprehensive "${targets[@]}" | tee "$EVIDENCE_DIR/campaign.log"
MUTATION_DB="$mutation_db" MUTATION_REUSE_OUTCOMES=true \
  MUTATION_EVIDENCE_FILE="$EVIDENCE_DIR/high-gate.json" MEWT="$MEWT_BIN" \
  ./scripts/mutation-high-gate.sh
