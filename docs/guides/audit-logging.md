# Audit logging

When the server uses Postgres, every `/v1/*` API call is persisted as a structured audit event (action, actor, HTTP metadata, optional classified resource details). Detail fields retain their public, sensitive-metadata, or secret classification and only an approved value, metadata, reference, fingerprint, presence, or count projection reaches Postgres and review output. Legacy arbitrary detail maps are omitted when read.

## View recent events

```bash
remotr logs list --since 24h
remotr logs list --since 24h --action admin.endpoint.delete --json
```

Paginate with the `next_cursor` value from JSON output:

```bash
remotr logs list --since 24h --cursor "$CURSOR"
```

## Provision a SIEM collector credential

Export endpoints require mTLS. Create a dedicated operator credential for the collector host (do not copy your interactive operator cert):

```bash
remotr admin credential stamp \
  --label siem-collector \
  --role security_logger \
  --out /etc/remotr-siem
# writes cert.pem, key.pem, ca.pem, state.json
```

See [RBAC](rbac.md) for stamped credential installation steps.

## Discover the export URL

The export path includes a per-server random key (defense in depth, similar to a webhook URL):

```bash
remotr logs export-info
# export path: /v1/exports/audit/<path_key>
```

## Pull events into a SIEM

Example collector script (last 24 hours, paginated):

```bash
#!/usr/bin/env bash
set -euo pipefail

SERVER_URL="https://remotr.example:8443"
CERT_DIR="/etc/remotr-siem"
PATH_KEY="$(remotr logs export-info --json | jq -r .path_key)"
SINCE="$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)"
CURSOR=""

while true; do
  URL="${SERVER_URL}/v1/exports/audit/${PATH_KEY}?since=${SINCE}&limit=500"
  if [[ -n "${CURSOR}" ]]; then
    URL="${URL}&cursor=${CURSOR}"
  fi
  RESP="$(curl -fsS \
    --cert "${CERT_DIR}/cert.pem" \
    --key "${CERT_DIR}/key.pem" \
    --cacert "${CERT_DIR}/ca.pem" \
    "${URL}")"
  echo "${RESP}" | jq -c '.events[]' >> /var/log/remotr-audit.ndjson
  CURSOR="$(echo "${RESP}" | jq -r '.next_cursor // empty')"
  [[ -z "${CURSOR}" ]] && break
done
```

Ship `/var/log/remotr-audit.ndjson` to your SIEM with your existing log forwarder.

See [HTTP API reference — Audit logging](../reference/http-api.md#audit-logging) for full query parameters and response fields.
