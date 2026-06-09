# HTTP API reference

Base URL: `https://<remotr-server>:8443`

All JSON endpoints use `Content-Type: application/json` unless noted.

## Authentication summary

| Endpoint | Auth |
|----------|------|
| `GET /healthz` | None |
| `GET /v1/ca.pem` | None (public Remotr CA certificate) |
| `POST /v1/enroll` | Enrollment token in JSON body |
| `POST /v1/sync` | mTLS client certificate (endpoint credential) |
| `POST /v1/admin/bootstrap` | Bootstrap token in JSON body |
| `GET/POST /v1/admin/*` | mTLS client certificate (operator credential) |
| `GET /v1/exports/audit/{path_key}` | mTLS operator credential **and** server-specific path key |
| `POST /v1/webhooks/git` | Optional `X-Remotr-Git-Webhook-Secret` header |

**Endpoint identity** is always derived from the TLS client certificate (SAN / fingerprint mapping). Request bodies must not carry a trusted endpoint ID.

---

## `GET /healthz`

Liveness probe. No authentication.

**Response:** `200 OK`, body `ok`

---

## `GET /v1/ca.pem`

Returns the Remotr **CA certificate** (PEM). No authentication. Used by the agent install script and operators to establish TLS trust before enrollment. The CA is public key material, not a secret.

**Response:** `200 OK`, `Content-Type: application/x-pem-file`, body is the CA PEM.

**Errors:** `503` if the server has no CA configured.

---

## `POST /v1/enroll`

Exchange a one-time enrollment token for an endpoint credential.

### Request

```json
{
  "token": "hex-or-string-token",
  "csr_pem": "-----BEGIN CERTIFICATE REQUEST-----\n...\n-----END CERTIFICATE REQUEST-----"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `token` | yes | One-time enrollment token |
| `csr_pem` | no | PEM CSR. When present, agent keeps private key locally; response omits `key_pem`. **Preferred.** |

### Response `200 OK`

```json
{
  "endpoint_id": "uuid",
  "cert_pem": "-----BEGIN CERTIFICATE-----\n...",
  "key_pem": "-----BEGIN RSA PRIVATE KEY-----\n...",
  "ca_pem": "-----BEGIN CERTIFICATE-----\n..."
}
```

| Field | Description |
|-------|-------------|
| `endpoint_id` | Server-assigned endpoint UUID |
| `cert_pem` | Issued client certificate |
| `key_pem` | Present only in legacy server-key mode |
| `ca_pem` | Remotr CA certificate |

### Errors

| Status | Meaning |
|--------|---------|
| `400` | Missing token or invalid CSR |
| `401` | Invalid or expired enrollment token |
| `503` | Enrollment unavailable (no CA or registry) |

---

## `POST /v1/sync`

Agent check-in. Requires mTLS with endpoint credential.

Supports `Accept-Encoding: gzip` — artifact YAML may be gzip-compressed in the response.

### Request

```json
{
  "lastDigest": "sha256:...",
  "labels": {
    "site": "berlin",
    "role": "web"
  },
  "drift": {
    "releaseRef": "abc123",
    "digest": "sha256:...",
    "resources": ["cfg/curl"]
  },
  "applyFailure": {
    "releaseRef": "abc123",
    "resource": "cfg/sshd-config",
    "message": "pre-apply validation failed"
  },
  "agentVersion": "v0.1.15",
  "agentUpgradeStatus": {
    "desired": "v0.1.15",
    "phase": "completed",
    "message": ""
  }
}
```

All fields except `lastDigest` are optional telemetry. `agentVersion` and `agentUpgradeStatus` support in-band agent upgrade reporting (server v0.1.13+).

### Response `200 OK`

**Unchanged artifact:**

```json
{
  "unchanged": true,
  "digest": "sha256:...",
  "remediationPolicy": "auto",
  "agentUpgrade": {
    "version": "v0.1.15",
    "githubRepo": "DavidHoenisch/remotr"
  }
}
```

**New or updated artifact:**

```json
{
  "unchanged": false,
  "releaseRef": "commit-sha-or-label",
  "digest": "sha256:...",
  "artifactYaml": "configurations:\n  - name: ...",
  "remediationPolicy": "auto",
  "agentUpgrade": {
    "version": "v0.1.15",
    "githubRepo": "DavidHoenisch/remotr"
  }
}
```

| Field | Description |
|-------|-------------|
| `remediationPolicy` | `auto` or `report` for the endpoint's fleet |
| `artifactYaml` | Raw deployable artifact bytes (YAML) |
| `agentUpgrade` | Present when operator tainted the endpoint/fleet; omitted when versions already match |
| `agentUpgrade.version` | Target Git tag (for example `v0.1.15`) |
| `agentUpgrade.githubRepo` | GitHub `owner/repo` for release assets (default `DavidHoenisch/remotr`) |

### Errors

| Status | Meaning |
|--------|---------|
| `401` | mTLS failed or cert not mapped to endpoint |
| `403` | Unknown endpoint |
| `500` | Artifact resolution failure |

---

## `POST /v1/admin/bootstrap`

Exchange one-time bootstrap token for operator credential. No mTLS required (token is the secret).

### Request

```json
{
  "token": "bootstrap-secret"
}
```

### Response `200 OK`

```json
{
  "operator_id": "uuid",
  "cert_pem": "...",
  "key_pem": "...",
  "ca_pem": "..."
}
```

Bootstrap token is invalidated after success.

### Errors

| Status | Meaning |
|--------|---------|
| `401` | Invalid bootstrap token |
| `503` | Bootstrap unavailable |

---

## `POST /v1/admin/enroll-tokens`

Create enrollment token. Requires operator mTLS.

### Request

```json
{
  "fleet": "engineering",
  "ttl": "168h"
}
```

`ttl` is a Go duration string (for example `24h`, `168h`).

### Response `200 OK`

```json
{
  "token": "...",
  "fleet": "engineering",
  "expires_at": "2026-06-09T12:00:00Z"
}
```

---

## `GET /v1/admin/endpoints`

List enrolled endpoints. Requires operator mTLS.

### Response `200 OK`

```json
[
  {
    "id": "uuid",
    "fleet": "engineering",
    "cert_fingerprint": "sha256:...",
    "labels": {"site": "berlin"},
    "desired_agent_version": "v0.1.15",
    "reported_agent_version": "v0.1.14",
    "last_drift": {
      "release_ref": "abc123",
      "digest": "sha256:...",
      "reported_at": "2026-06-02T10:00:00Z"
    }
  }
]
```

---

## `GET /v1/admin/endpoints/{id}`

Get one endpoint. Requires operator mTLS. List fields plus optional `agent_upgrade`, `last_drift`, and `last_apply_failure` detail objects.

---

## Agent upgrade (operator taint)

Requires operator mTLS and Postgres migration `003_agent_upgrade.sql`.

### `POST /v1/admin/endpoints/{id}/agent-upgrade`

Set desired agent version for one endpoint. Body:

```json
{"version": "v0.1.15"}
```

**Response `200 OK`:** `{"version": "v0.1.15"}`

**Errors:** `400` invalid id/version, `404` endpoint not found

### `POST /v1/admin/fleets/{fleet}/agent-upgrade`

Set desired agent version for every endpoint in the fleet.

**Response `200 OK`:** `{"version": "v0.1.15", "endpoints": 12}`

---

## Deployment tokens

Reusable enrollment tokens for bulk provisioning. Requires operator mTLS.

### `POST /v1/admin/deployment-tokens`

Create token (secret returned once). Body: `label`, `fleet`, `ttl` (Go duration).

### `GET /v1/admin/deployment-tokens`

List token metadata (no secret).

### `GET /v1/admin/deployment-tokens/{label}`

Show one token.

### `DELETE /v1/admin/deployment-tokens/{label}`

Revoke token.

---

## `DELETE /v1/admin/endpoints/{id}`

Remove an enrolled endpoint from the server registry. Requires operator mTLS.

Deletes the endpoint row and cascaded telemetry (`endpoint_labels`, `drift_reports`, `apply_failures`). Does not stop the agent on the machine or remove Git config overrides.

**Response:** `204 No Content`

**Errors:** `400` invalid id, `404` not found, `503` admin unavailable

---

## Audit logging

Requires Postgres (`REMOTR_DATABASE_URL`). The server persists structured audit events for API activity and exposes them to operators and SIEM exporters.

Each request under `/v1/*` (except `/healthz`) is recorded with:

- `occurred_at`, `request_id`, HTTP method/path, status code
- Actor type (`operator`, `endpoint`, `anonymous`) and ID from mTLS when present
- Semantic `action` (for example `admin.endpoint.delete`, `agent.sync`)
- Optional `resource_type`, `resource_id`, and `details` JSON

Events are also written to server structured logs (`slog`) for operational visibility.

### `GET /v1/admin/audit-events`

List audit events. Requires operator mTLS.

**Query parameters:**

| Parameter | Description |
|-----------|-------------|
| `since` | RFC3339 timestamp (inclusive lower bound) |
| `until` | RFC3339 timestamp (inclusive upper bound) |
| `action` | Filter by action (for example `admin.git_sync`) |
| `actor_type` | Filter by `operator`, `endpoint`, or `anonymous` |
| `limit` | Page size (default `100`, max `1000`) |
| `cursor` | Opaque cursor from a previous response `next_cursor` |

**Response `200 OK`:**

```json
{
  "events": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "occurred_at": "2026-06-09T12:00:00Z",
      "request_id": "req-abc",
      "actor_type": "operator",
      "actor_id": "11111111-1111-1111-1111-111111111111",
      "action": "admin.endpoint.delete",
      "method": "DELETE",
      "path": "/v1/admin/endpoints/ep-1",
      "status_code": 204,
      "resource_type": "endpoint",
      "resource_id": "ep-1"
    }
  ],
  "next_cursor": "eyJ0IjoiMjAyNi0wNi0wOVQxMjowMDowMFoiLCJpZCI6IjU1MGU4NDAwLWUyOWItNDFkNC1hNzE2LTQ0NjY1NTQ0MDAwMCJ9"
}
```

### `GET /v1/admin/audit-export`

Return the secret export path for SIEM collectors. Requires operator mTLS.

**Response `200 OK`:**

```json
{
  "export_path": "/v1/exports/audit/7f3c9e2a1b4d8f6e0c5a9b2d4e6f8a1c3e5b7d9f1a2c4e6b8d0f2a4c6e8b0d2",
  "path_key": "7f3c9e2a1b4d8f6e0c5a9b2d4e6f8a1c3e5b7d9f1a2c4e6b8d0f2a4c6e8b0d2"
}
```

The `path_key` is generated once per server and stored in Postgres. Treat it like a webhook secret: do not publish it in public issue trackers.

### `GET /v1/exports/audit/{path_key}`

Export audit events for SIEM ingestion. Requires:

1. Valid **operator mTLS** client certificate (use a dedicated credential; see `POST /v1/admin/operator-credentials`)
2. Correct `{path_key}` from `GET /v1/admin/audit-export`

Supports the same query parameters and response shape as `GET /v1/admin/audit-events`.

**Example — last 24 hours:**

```bash
SINCE=$(date -u -d '24 hours ago' +%Y-%m-%dT%H:%M:%SZ)
curl --cert siem/cert.pem --key siem/key.pem --cacert siem/ca.pem \
  "https://remotr.example:8443/v1/exports/audit/${PATH_KEY}?since=${SINCE}&limit=500"
```

Wrong `path_key` returns `404` even with valid mTLS (defense in depth).

### `POST /v1/admin/operator-credentials`

Issue a new operator mTLS credential and register its fingerprint. Requires an existing operator mTLS session. Use this to provision SIEM export collectors or other automation without sharing your personal operator cert.

**Request (optional body):**

```json
{"label": "siem-collector"}
```

The `label` is recorded in audit metadata only; it is not stored as a server credential name.

**Response `200 OK`:**

```json
{
  "operator_id": "22222222-2222-2222-2222-222222222222",
  "label": "siem-collector",
  "cert_pem": "-----BEGIN CERTIFICATE-----\n...",
  "key_pem": "-----BEGIN RSA PRIVATE KEY-----\n...",
  "ca_pem": "-----BEGIN CERTIFICATE-----\n..."
}
```

---

## Git sync

### `POST /v1/webhooks/git`

Trigger immediate Git sync (for GitHub/forge webhooks).

**Headers:**

```text
X-Remotr-Git-Webhook-Secret: <REMOTR_GIT_WEBHOOK_SECRET>
```

Required when `REMOTR_GIT_WEBHOOK_SECRET` is set on the server.

**Response:** `200 OK`, body `ok`

**Errors:** `401` bad secret, `500` sync failure

### `POST /v1/admin/git-sync`

Trigger immediate Git sync as an operator. Requires operator mTLS (same as other `/v1/admin/*` routes).

**Response:** `200 OK`, body `ok`

**Errors:** `401`/`403` unauthorized, `500` sync failure

---

## CLI equivalents

| API | CLI |
|-----|-----|
| `POST /v1/admin/bootstrap` | `remotr bootstrap` |
| `POST /v1/admin/enroll-tokens` | `remotr enroll token create` |
| `POST /v1/admin/deployment-tokens` | `remotr deployment create` |
| `GET /v1/admin/deployment-tokens` | `remotr deployment list` |
| `GET /v1/admin/deployment-tokens/{label}` | `remotr deployment show` |
| `DELETE /v1/admin/deployment-tokens/{label}` | `remotr deployment revoke` |
| `POST /v1/admin/git-sync` | `remotr git sync` |
| `GET /v1/admin/endpoints` | `remotr endpoint list` |
| `GET /v1/admin/endpoints/{id}` | `remotr endpoint show` |
| `DELETE /v1/admin/endpoints/{id}` | `remotr endpoint remove` |
| `POST /v1/admin/endpoints/{id}/agent-upgrade` | `remotr endpoint agent upgrade` |
| `POST /v1/admin/fleets/{fleet}/agent-upgrade` | `remotr fleet agent upgrade` |
| `POST /v1/enroll` | `remotr-agent enroll` |
| `POST /v1/sync` | `remotr-agent` sync loop |
| `GET /v1/admin/audit-events` | `remotr logs list` |
| `GET /v1/admin/audit-export` | `remotr logs export-info` |
| `GET /v1/exports/audit/{path_key}` | SIEM collector (mTLS + path key) |
| `POST /v1/admin/operator-credentials` | `remotr admin credential stamp` |
