#!/usr/bin/env bash
# Apply Remotr registry schema (sql/schema.sql) to Postgres.
#
# Usage (production / Neon):
#   REMOTR_DATABASE_URL='postgres://...' make migrate
#
# Or resolve the connection string from Neon CLI:
#   REMOTR_NEON_PROJECT=my-remotr-app make migrate
#
# Optional:
#   REMOTR_NEON_DATABASE=remotr   Neon database name (default: remotr)
#   REMOTR_FLEET=default          upsert fleet_settings row after schema apply
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCHEMA="${ROOT}/sql/schema.sql"

log() { printf 'migrate: %s\n' "$*" >&2; }
die() { printf 'migrate: error: %s\n' "$*" >&2; exit 1; }

neon_cmd() {
	if command -v neon >/dev/null 2>&1; then
		echo neon
	elif command -v neonctl >/dev/null 2>&1; then
		echo neonctl
	else
		die "Neon CLI not found (install: npm install -g neonctl) and REMOTR_DATABASE_URL is unset"
	fi
}

normalize_db_url() {
	local url=$1
	url=${url/postgresql:/postgres:}
	if [[ "$url" != *sslmode=* ]]; then
		if [[ "$url" == *"?"* ]]; then
			url="${url}&sslmode=require"
		else
			url="${url}?sslmode=require"
		fi
	fi
	printf '%s' "$url"
}

neon_project_id_by_name() {
	local neon=$1 project=$2
	"$neon" --no-color --output json projects list | jq -r --arg name "$project" '
		(if type == "array" then . else .projects // [] end)
		| .[] | select(.name == $name) | .id' | head -1
}

neon_connection_uri() {
	local neon=$1 project_id=$2 db=$3
	local out
	out=$("$neon" --no-color --output json connection-string \
		--project-id "$project_id" \
		--database-name "$db")
	if echo "$out" | jq -e . >/dev/null 2>&1; then
		echo "$out" | jq -r '.connection_uri // .connection_uris[0].connection_uri // empty' | tr -d '\n\r'
	else
		echo "$out" | tr -d '\n\r'
	fi
}

resolve_database_url() {
	if [[ -n "${REMOTR_DATABASE_URL:-}" ]]; then
		normalize_db_url "$REMOTR_DATABASE_URL"
		return 0
	fi

	local project="${REMOTR_NEON_PROJECT:-}"
	[[ -n "$project" ]] || die "set REMOTR_DATABASE_URL or REMOTR_NEON_PROJECT"

	command -v jq >/dev/null 2>&1 || die "jq is required for Neon project lookup (https://jqlang.org/)"

	local neon db project_id url
	neon=$(neon_cmd)
	db="${REMOTR_NEON_DATABASE:-remotr}"

	if ! "$neon" me >/dev/null 2>&1; then
		die "Neon CLI not authenticated — run: $neon auth"
	fi

	project_id=$(neon_project_id_by_name "$neon" "$project")
	[[ -n "$project_id" && "$project_id" != null ]] || die "Neon project not found: ${project}"

	url=$(neon_connection_uri "$neon" "$project_id" "$db")
	[[ -n "$url" ]] || die "could not resolve Neon connection string for project ${project}"

	log "using Neon project ${project} (${project_id}), database ${db}"
	normalize_db_url "$url"
}

run_psql() {
	if command -v psql >/dev/null 2>&1; then
		psql "$@"
		return
	fi
	if ! command -v docker >/dev/null 2>&1; then
		die "need psql or Docker to apply ${SCHEMA}"
	fi
	docker run --rm -i postgres:17-alpine psql "$@"
}

run_psql_file() {
	local url=$1 file=$2
	if command -v psql >/dev/null 2>&1; then
		psql "$url" -v ON_ERROR_STOP=1 -f "$file"
		return
	fi
	if ! command -v docker >/dev/null 2>&1; then
		die "need psql or Docker to apply ${file}"
	fi
	docker run --rm -i -v "${file}:${file}:ro" postgres:17-alpine \
		psql "$url" -v ON_ERROR_STOP=1 -f "$file"
}

seed_fleet() {
	local url=$1 fleet=$2
	log "ensuring fleet_settings row for fleet ${fleet}"
	run_psql "$url" -v ON_ERROR_STOP=1 <<SQL
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('${fleet}', 'auto')
ON CONFLICT (fleet) DO NOTHING;
SQL
}

main() {
	[[ -f "$SCHEMA" ]] || die "schema not found: ${SCHEMA}"

	local url
	url=$(resolve_database_url)

	log "applying ${SCHEMA}"
	run_psql_file "$url" "$SCHEMA"

	if [[ -d "${ROOT}/sql/migrations" ]]; then
		local migration
		for migration in "${ROOT}"/sql/migrations/*.sql; do
			[[ -f "$migration" ]] || continue
			log "applying $(basename "$migration")"
			run_psql_file "$url" "$migration"
		done
	fi

	if [[ -n "${REMOTR_FLEET:-}" ]]; then
		seed_fleet "$url" "$REMOTR_FLEET"
	fi

	log "done"
}

main "$@"
