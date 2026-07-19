#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo "usage: $0 OUTPUT_DIR SERVER_BINARY COMPOSE_FILE [LOAD_REPORT]" >&2
  exit 2
fi

output_dir=$1
server_binary=$2
compose_file=$3
load_report=${4:-}
profile_seconds=${REMOTR_PROFILE_SECONDS:-10}
max_bytes=${REMOTR_PROFILE_MAX_BYTES:-1048576}

if [[ -z "$output_dir" || "$output_dir" == "/" || ! -f "$server_binary" || ! -f "$compose_file" ]]; then
  echo "performance capture requires a bounded output directory, server binary, and Compose file" >&2
  exit 2
fi

mkdir -p "$output_dir"
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

sanitize() {
  local input=$1
  local output=$2
  if [[ -f "$input" ]]; then
    go run -mod=vendor ./scripts/performance-sanitize.go -limit "$max_bytes" "$input" > "$output"
  fi
}

compose_fetch() {
  local path=$1
  local output=$2
  docker compose -f "$compose_file" exec -T remotr-server \
    wget -qO- "http://127.0.0.1:6060$path" > "$output"
}

if compose_fetch "/debug/pprof/profile?seconds=$profile_seconds" "$scratch/cpu.pb"; then
  go tool pprof -top -nodecount=40 "$server_binary" "$scratch/cpu.pb" > "$scratch/cpu.txt" 2>&1 || true
  sanitize "$scratch/cpu.txt" "$output_dir/cpu-top.txt"
fi

if compose_fetch "/debug/pprof/heap" "$scratch/heap.pb"; then
  go tool pprof -top -nodecount=40 "$server_binary" "$scratch/heap.pb" > "$scratch/heap.txt" 2>&1 || true
  sanitize "$scratch/heap.txt" "$output_dir/heap-top.txt"
fi

if compose_fetch "/debug/pprof/goroutine?debug=1" "$scratch/goroutines.txt"; then
  sanitize "$scratch/goroutines.txt" "$output_dir/goroutines.txt"
fi

if compose_fetch "/debug/pprof/trace?seconds=$profile_seconds" "$scratch/trace.out"; then
  go tool trace -pprof=sched "$scratch/trace.out" > "$scratch/trace-sched.pb" 2>/dev/null || true
  if [[ -s "$scratch/trace-sched.pb" ]]; then
    go tool pprof -top -nodecount=40 "$server_binary" "$scratch/trace-sched.pb" > "$scratch/trace.txt" 2>&1 || true
    sanitize "$scratch/trace.txt" "$output_dir/trace-scheduler-top.txt"
  fi
fi

docker compose -f "$compose_file" exec -T postgres \
  psql -U remotr -d remotr -Atc \
  "SELECT json_build_object('database',current_database(),'backends',numbackends,'commits',xact_commit,'rollbacks',xact_rollback,'blocks_read',blks_read,'blocks_hit',blks_hit,'temp_files',temp_files,'temp_bytes',temp_bytes,'deadlocks',deadlocks) FROM pg_stat_database WHERE datname=current_database()" \
  > "$scratch/query.json" 2>&1 || true
sanitize "$scratch/query.json" "$output_dir/query-summary.json"

mapfile -t container_ids < <(docker compose -f "$compose_file" ps -q)
if [[ ${#container_ids[@]} -gt 0 ]]; then
  docker stats --no-stream --format '{{json .}}' "${container_ids[@]}" > "$scratch/system.jsonl" 2>&1 || true
  sanitize "$scratch/system.jsonl" "$output_dir/system-containers.jsonl"
fi

if [[ -n "$load_report" && -f "$load_report" ]]; then
  sanitize "$load_report" "$output_dir/workload.json"
fi

revision=$(git rev-parse HEAD 2>/dev/null || echo unknown)
go_version=$(go version 2>/dev/null || echo unknown)
printf '{"schemaVersion":1,"revision":"%s","goVersion":"%s","profileSeconds":%s,"maxRetainedBytes":%s}\n' \
  "$revision" "$go_version" "$profile_seconds" "$max_bytes" > "$output_dir/manifest.json"
