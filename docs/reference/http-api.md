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
| `POST /v1/secrets/resolve` | mTLS client certificate (endpoint credential) |
| `POST /v1/app-packages/download-url` | mTLS client certificate (endpoint credential) |
| `POST /v1/diagnostics/upload-url` | mTLS client certificate (endpoint credential) |
| `POST /v1/admin/bootstrap` | Bootstrap token in JSON body |
| All methods under `/v1/admin/*` | mTLS client certificate (operator credential), then RBAC authorization |
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

Supports `Accept-Encoding: gzip` — the complete JSON response is compressed
when the header contains `gzip`.

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
  "usernames": ["alice", "bob"],
  "agentUpgradeStatus": {
    "desired": "v0.1.15",
    "phase": "completed",
    "message": ""
  },
  "cronsDigest": "sha256hex...",
  "cronResults": [
    {
      "runId": "uuid",
      "cronName": "weekly-system-upgrade",
      "status": "success",
      "startedAt": "2026-06-18T00:00:05Z",
      "completedAt": "2026-06-18T00:01:22Z",
      "failures": []
    }
  ],
  "diagnosticResult": {
    "requestId": "uuid",
    "status": "ready",
    "sha256": "hex...",
    "sizeBytes": 12345
  },
  "changePreflights": [
    {
      "change_request_id": "uuid",
      "ready": true
    }
  ]
}
```

All fields except `lastDigest` are optional telemetry. `usernames` lists interactive Linux accounts (UID ≥ 1000) reported by the agent on each sync. `agentVersion` and `agentUpgradeStatus` support in-band agent upgrade reporting (server v0.1.13+). `cronResults` and `cronsDigest` report scheduled job outcomes from the previous sync cycle (requires Postgres migration `007_cron_executions.sql`).

`changePreflights` is the protocol seam for requesting an endpoint-specific
execution lease. The server derives `endpoint_id` from mTLS, ignores any
endpoint identity in the item, and issues a lease only for an authorized,
frozen target inside all rollout bounds. The current production agent does not
populate this field as part of its generic Apply pipeline.

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
  },
  "cronsDigest": "sha256hex...",
  "dueCrons": [
    {
      "runId": "uuid",
      "cronName": "weekly-system-upgrade",
      "scheduledFor": "2026-06-18T00:00:00Z",
      "specYaml": "crons:\n  - name: weekly-system-upgrade\n    ..."
    }
  ],
  "diagnosticCollection": {
    "requestId": "uuid",
    "collectors": ["system_info", "journal_kernel"],
    "since": "2026-06-24T12:00:00Z",
    "until": "2026-06-25T12:00:00Z"
  },
  "executionLeases": [
    {
      "id": "uuid",
      "change_request_id": "uuid",
      "endpoint_id": "uuid",
      "resource_hashes": {"network/office-uplink": "sha256:..."},
      "attempt": 1,
      "issued_at": "2026-07-15T18:42:00Z",
      "expires_at": "2026-07-15T18:47:00Z",
      "completed": false,
      "risk": "connectivity",
      "progress": "lease-issued",
      "updated_at": "2026-07-15T18:42:00Z"
    }
  ]
}
```

**New or updated artifact:**

```json
{
  "unchanged": false,
  "releaseRef": "commit-sha-or-label",
  "digest": "sha256:...",
  "artifactYaml": "Y29uZmlndXJhdGlvbnM6CiAgLSBuYW1lOiAuLi4K",
  "remediationPolicy": "auto",
  "agentUpgrade": {
    "version": "v0.1.15",
    "githubRepo": "DavidHoenisch/remotr"
  },
  "cronsDigest": "sha256hex...",
  "dueCrons": []
}
```

| Field | Description |
|-------|-------------|
| `remediationPolicy` | `auto` or `report` for the endpoint's fleet |
| `artifactYaml` | Deployable YAML as JSON-encoded bytes (base64 on the wire); normal JSON decoders targeting a byte array decode it automatically |
| `agentUpgrade` | Present when operator tainted the endpoint/fleet; omitted when versions already match |
| `agentUpgrade.version` | Target Git tag (for example `v0.1.15`) |
| `agentUpgrade.githubRepo` | GitHub `owner/repo` for release assets (default `DavidHoenisch/remotr`) |
| `cronsDigest` | SHA256 of the resolved composed crons artifact (fleet or endpoint override) |
| `dueCrons` | Jobs the server wants the agent to run now (apply-only). Present even when `unchanged` is true |
| `dueCrons[].specYaml` | Single-job cron spec for the agent apply engine |
| `diagnosticCollection` | Operator-requested diagnostic job for this endpoint (when pending) |
| `diagnosticResult` | Agent-reported diagnostic bundle upload result (request body) |
| `executionLeases` | Five-minute, hash-bound grants produced from accepted `changePreflights`; current generic agent Apply does not consume them |

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

## Secret version lifecycle

All secret endpoints require operator mTLS. They return safe metadata only; secret material is never returned to operators.

### `POST /v1/admin/secrets/versions`

Upload an inactive version with `Content-Type: application/octet-stream`. Query parameters are `name` plus exactly one of `fleet` or `endpoint_id`. The response is `201 Created` with name, version, fingerprint, scope, lifecycle, and audit metadata.

### `GET /v1/admin/secrets?name=<logical-name>`

List safe version metadata. `GET /v1/admin/secrets/value` always returns `405 Method Not Allowed`; general plaintext readback is unsupported.

### `POST /v1/admin/secrets/activate`

Activate an exact version through audited rollout planning:

```json
{"name":"repositories/private","version":"2"}
```

High-risk resources following `@active` require an authorized Change rollout before endpoints can resolve the newly active material.

### `POST /v1/admin/secrets/revoke`

Block future resolution of an exact version using the same request shape. The response reports `resolutionBlocked: true` and `endpointCopyStatus: "rotation-or-removal-required"`; it does not claim that installed endpoint copies were erased.

### `POST /v1/secrets/resolve` (endpoint mTLS)

Resolve server-managed material for one exact use in the endpoint's active
artifact. This route is called by `remotr-agent`; it is not a general secret
read API.

```json
{
  "reference": "remotr:wifi/office@active",
  "artifactDigest": "sha256:...",
  "resourceAddress": "network/office-wifi",
  "purpose": "network-credential"
}
```

The server derives `endpointId` and `fleet` from the client certificate and
registry. It then requires the supplied digest to equal the endpoint's active
artifact and verifies that the named resource declares the same reference and
purpose. Unknown JSON fields are rejected. The request body is limited to 16
KiB.

**Response `200 OK`:**

```json
{
  "provider": "remotr",
  "version": "3",
  "fingerprint": "sha256:...",
  "material": "base64-encoded-by-JSON"
}
```

`material` is a JSON byte string and is therefore base64 encoded on the wire.
Resolved material is limited to 1 MiB. `403` means the certificate, artifact
binding, declared use, or change authorization does not permit this
resolution. `503` means the active artifact or secret provider is unavailable.

See [Manage server-side secrets](../guides/secret-management.md) for the
operator workflow and [Review and control high-risk changes](../guides/change-control.md)
for the `@active` high-risk gate.

---

## Change control

All change-control routes require operator mTLS and RBAC authorization. A
change request contains immutable release, artifact, resource-hash, and frozen
target evidence plus its mutable approval/lifecycle state.

!!! warning "Persistence does not make this a generic Apply gate"
    All change-control mutations are persisted in Postgres and restored on
    restart. Generic Git releases do not automatically create change
    requests, and the agent does not consume execution leases as a universal
    high-risk Apply gate.
    Read the [change-control workflow and support boundary](../guides/change-control.md#current-support-boundary)
    before using these routes operationally.

### `GET /v1/admin/change-requests`

Return every change request in the restored registry, ordered by creation time. There are no
server-side filters or pagination parameters.

### `GET /v1/admin/change-requests/{id}`

Return one request. `404` means the identifier is unknown. An ordinary
single-server restart restores known identifiers from Postgres.

Important response fields include `fleet`, `release_ref`, `artifact_digest`,
`authorization_group`, `risk`, `resources`, `resource_hashes`,
`frozen_targets`, `required_approvals`, `approvals`,
`authorization_state`, `outcomes`, and `audit_history`.

### `POST /v1/admin/change-requests/{id}/authorize`

Submit the authenticated operator's approval and proposed rollout bounds:

```json
{
  "valid_from": "2026-07-16T01:00:00Z",
  "valid_until": "2026-07-23T01:00:00Z",
  "attempt_limit": 1,
  "max_concurrency": 1,
  "execution_windows": [
    {
      "weekdays": [1, 2, 3, 4, 5],
      "start_minute_utc": 60,
      "duration": 7200000000000
    }
  ],
  "justification": "CHG-4821: controlled production rollout"
}
```

`weekdays` uses Go's numeric weekday values (`0` Sunday through `6`
Saturday). `start_minute_utc` is minutes after UTC midnight. `duration` is a
JSON integer in nanoseconds; the example is two hours. A window may cross UTC
midnight.

Omitted validity starts immediately and ends 30 days later. Non-positive
attempt and concurrency values default to `1`. Distinct operator identities
are counted once. The default threshold is one approval except for
`destructive`, which requires two.

When the threshold is met, the response is the created rollout authorization.
Before it is met, the compatibility endpoint returns a zero-valued
authorization object; use `GET .../{id}` and inspect `approvals`,
`required_approvals`, and `authorization_state` rather than treating that
object as an authorization.

### Lifecycle routes

| Route | Effect |
| --- | --- |
| `POST /v1/admin/change-requests/{id}/pause` | Change state to `paused`. |
| `POST /v1/admin/change-requests/{id}/resume` | Return an existing rollout to `authorized`; fails if no rollout was ever created. |
| `POST /v1/admin/change-requests/{id}/revoke` | Change state to `revoked`; installed endpoint state is not rolled back. |

Each route accepts an empty body and returns the updated change request.
The lifecycle state and its audit entry are committed before the route reports
success and survive an ordinary server restart.

### `POST /v1/admin/change-requests/{id}/baseline`

Promote one eligible, successfully evidenced resource hash to a baseline:

```json
{
  "resource_address": "network/office-uplink",
  "acknowledge_exceptions": false
}
```

Set `acknowledge_exceptions` only after reviewing any incompatible or
preflight-incomplete frozen targets. The route returns a
`BaselineAuthorization`; validation failures return `400`.
Promotion and any exception acknowledgement commit atomically and survive
restart.

### `POST /v1/admin/fleets/{fleet}/baseline-adoptions`

Create a review request for explicitly supplied existing state. The path fleet
wins over any `fleet` field in the body.

```json
{
  "release_ref": "4c6ab63d15ce8f4de8b3a614bc84acfe0f2b4d62",
  "artifact_digest": "sha256:...",
  "targets": [
    {
      "endpoint_id": "endpoint-01",
      "compatible": true,
      "preflight_ready": true
    }
  ],
  "resources": [
    {
      "address": "network/office-uplink",
      "desired_hash": "sha256:...",
      "risk": "connectivity",
      "provider": "network-manager",
      "rollback_class": "transactional",
      "baseline_eligible": true
    }
  ]
}
```

The server does not discover this evidence for the caller. See
[Baseline adoption](../guides/change-control.md#baseline-adoption) for the full
review procedure and complete plan fields.

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
    "usernames": ["alice", "bob"],
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

## `GET /v1/admin/fleets`

List fleet names known to the endpoint registry. Requires operator mTLS.

```json
["engineering", "production"]
```

This is registry inventory, not a list of directories found in the current
configuration repository.

## Compliance state reports

State reports expose the latest structured, redacted check/apply evidence
reported by agents. A successful HTTP response does not necessarily mean the
endpoint is compliant; inspect `status` and the detailed outcome fields.

The mutually exclusive endpoint status values are `compliant`, `drifted`,
`unsupported`, `check_failed`, `deferred`, `apply_failed`, and `no_report`.

### `GET /v1/admin/endpoints/{id}/state-report`

```json
{
  "endpoint_id": "11111111-1111-1111-1111-111111111111",
  "fleet": "engineering",
  "release_ref": "4c6ab63",
  "digest": "sha256:...",
  "reported_at": "2026-07-15T18:42:00Z",
  "in_compliance": false,
  "status": "drifted",
  "items": [
    {
      "address": "base/curl",
      "name": "curl",
      "provider": "apt",
      "status": "drifted",
      "reasonCode": "state_drift",
      "desiredSummary": {
        "fields": [
          {
            "path": "present",
            "sensitivity": "public",
            "projection": "value",
            "text": "true"
          }
        ]
      },
      "observedSummary": {
        "fields": [
          {
            "path": "status",
            "sensitivity": "public",
            "projection": "value",
            "text": "drifted"
          },
          {
            "path": "reasonCode",
            "sensitivity": "public",
            "projection": "value",
            "text": "state_drift"
          }
        ]
      }
    }
  ],
  "apply": [],
  "schedule_runtime": []
}
```

`items` are check outcomes. `apply` contains redacted mutation outcomes and
may include activation, reboot, rollback, and diagnostic details.
Desired, observed, and diagnostic summaries are classified field lists. A
field carries its schema-owned sensitivity and approved projection; secret raw
values are not a representable API shape. Older unclassified summary strings
are omitted when a legacy report is read.
`schedule_runtime` is operational execution history and does not itself
determine compliance. `reboot_required` carries durable reboot intent and
boot-ID-verified completion evidence when present. `apply_failure` is the
latest top-level failure summary.

An enrolled endpoint with no stored report returns `200` with `status:
"no_report"`; an unknown endpoint returns `404`.

### `GET /v1/admin/fleets/{fleet}/state-report`

Return all endpoint reports plus an aggregate summary:

```json
{
  "fleet": "engineering",
  "summary": {
    "total": 12,
    "compliant": 8,
    "drift": 1,
    "unsupported": 1,
    "check_failed": 0,
    "deferred": 1,
    "apply_failed": 0,
    "no_report": 1
  },
  "endpoints": []
}
```

See [Review compliance state](../guides/endpoint-management.md#review-compliance-state)
for operator interpretation and CLI exit behavior.

---

## Endpoint labels

Operator-managed key/value metadata stored in Postgres (`endpoint_labels`). Agent sync may upsert the same keys when the agent reports inventory labels.

### `PUT /v1/admin/endpoints/{id}/labels/{key}`

Set or update one label. Body:

```json
{"value": "berlin"}
```

**Response `200 OK`:**

```json
{
  "key": "site",
  "value": "berlin",
  "labels": {"site": "berlin", "role": "web"}
}
```

**Errors:** `400` invalid id/key/value, `404` endpoint not found

### `DELETE /v1/admin/endpoints/{id}/labels/{key}`

Remove one label.

**Response `204 No Content`**

**Errors:** `400` invalid id/key, `404` endpoint or label not found

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

## Endpoint diagnostics (operator-triggered)

Pull-based diagnostic bundles from endpoints. Requires Postgres migration `010_diagnostics.sql`, S3/MinIO (`REMOTR_S3_*`), and operator role `diagnostics_collector` or `global_admin`.

Collection uses a fixed allowlist of collectors (network state, journal logs, dmesg, system info, agent state). Operators choose collectors and a bounded time range; the agent never runs arbitrary commands or reads arbitrary paths.

The downloaded archive is metadata-only. Each collector produces a validated
classified summary containing byte and line counts, collection presence, and a
SHA-256 fingerprint. Raw journal, network, system-information, kernel, and
agent-state bytes are not placed in the archive. The agent validates this
closed structure before upload, and the server validates it again before
marking the request ready or returning a download.

### `POST /v1/admin/endpoints/{id}/diagnostics/collect`

Queue a diagnostic collection job for the endpoint's next sync.

**Request body (all fields optional):**

```json
{
  "collectors": ["system_info", "journal_kernel", "network_state"],
  "since": "2026-06-24T12:00:00Z",
  "until": "2026-06-25T12:00:00Z"
}
```

Defaults: all v1 collectors, last 24 hours. Maximum span: 7 days.

**Response `200 OK`:** diagnostic request object with `id`, `status` (`pending`), `spec`, `expires_at`.

**Errors:** `400` invalid spec, `404` endpoint not found, `409` active request already exists, `503` diagnostics or S3 unavailable

### `GET /v1/admin/diagnostics/{requestId}`

Poll collection status (`pending`, `dispatched`, `running`, `ready`, `failed`, `expired`).

### `GET /v1/admin/diagnostics/{requestId}/download`

Download the completed `application/gzip` tar bundle when `status` is `ready`.

### `POST /v1/diagnostics/upload-url` (endpoint mTLS)

Agent requests a presigned S3 PUT URL after collecting diagnostics.

**Request:** `{"requestId": "uuid"}`

**Response `200 OK`:** `{"url": "...", "key": "diagnostics/{endpoint}/{requestId}.tar.gz", "expires_at": "..."}`

Sync request/response extensions:

- Response may include `diagnosticCollection: { requestId, collectors, since, until }`
- Request may include `diagnosticResult: { requestId, status, sha256, sizeBytes, failure }`.
  `failure` is a validated classified error containing only a stable reason
  code, operation, cancellation state, and optional classified details; raw
  command, storage, or provider error text is not accepted.

---

## Cron job status

Server-managed scheduled jobs from composed crons artifacts. Requires Postgres migration `007_cron_executions.sql`.

### `GET /v1/admin/endpoints/{id}/cron-report`

Cron execution status for one endpoint: defined jobs, applicability (from labels), last run status, and messages.

**Response `200 OK`:**

```json
{
  "endpoint_id": "uuid",
  "fleet": "engineering",
  "crons_digest": "sha256hex...",
  "jobs": [
    {
      "name": "system-upgrade-debian",
      "schedule": "0 0 * * 0",
      "applicable": true,
      "last_status": "success",
      "last_scheduled_for": "2026-06-15T00:00:00Z",
      "last_completed_at": "2026-06-15T00:02:10Z"
    }
  ]
}
```

**Errors:** `404` endpoint not found, `503` cron reports unavailable (no Postgres)

### `GET /v1/admin/fleets/{fleet}/cron-report`

Aggregate cron status for all endpoints in a fleet.

**Response `200 OK`:**

```json
{
  "fleet": "engineering",
  "summary": {
    "total": 12,
    "applicable": 12,
    "success": 10,
    "failed": 1,
    "running": 0,
    "never_run": 1
  },
  "endpoints": []
}
```

See [Crons format reference](crons-format.md) for authoring cron sources.

---

## Firewall audit

### `GET /v1/admin/endpoints/{id}/firewall-audit`

Returns the latest firewall audit log reported by an endpoint. The agent writes this log when firewall resources are processed in **audit** mode (rules are validated but not applied). Requires operator mTLS.

**Response `200 OK`:**

```json
{
  "endpoint_id": "uuid",
  "digest": "sha256hex...",
  "reported_at": "2026-07-03T12:34:56Z",
  "report": [
    {
      "timestamp": "2026-07-03T12:34:55Z",
      "ruleName": "allow-ssh",
      "action": "allow",
      "backend": "firewalld",
      "wouldHave": "add service ssh to zone public",
      "enforced": false
    }
  ]
}
```

`report` is a JSON array of audit entries (or JSON Lines when raw). Each entry includes:

| Field | Meaning |
|-------|---------|
| `timestamp` | When the rule was processed |
| `ruleName` | Firewall rule name from the manifest |
| `action` | Rule action (`allow`, `deny`, `reject`) |
| `backend` | Detected backend (`firewalld`, `nftables`) |
| `wouldHave` | Human-readable description of what would have been done |
| `enforced` | `true` if the rule was actually applied; `false` in audit mode |

**Errors:** `404` endpoint or firewall audit not found, `503` firewall audit unavailable (no Postgres).

---

## Custom application packages

Catalog administration requires operator mTLS and the `package_manager` or
`global_admin` role. The catalog requires Postgres; upload and download URL
generation also require the configured S3-compatible object store.

### `POST /v1/admin/app-packages/upload`

Validate and publish a complete package zip. Send the zip directly with
`Content-Type: application/zip`, or as the `package` or `file` field in a
multipart form. The upload limit is 256 MiB.

The zip must contain a valid `remotr-package.yaml`. The server computes the
SHA-256, uploads the immutable object under its canonical key, and registers
the manifest in the catalog. An optional `s3_key` query parameter is accepted
only when it exactly matches that canonical key. Existing name/version pairs
return `409`.

**Response `200 OK`:**

```json
{
  "id": "uuid",
  "name": "acme/hello",
  "version": "1.2.3",
  "s3_key": "app-packages/acme/hello/1.2.3/acme_hello-1.2.3.zip",
  "sha256": "0123456789abcdef...",
  "manifest": {
    "SchemaVersion": 1,
    "Name": "acme/hello",
    "Version": "1.2.3",
    "Install": {
      "Mode": "binary",
      "Files": [
        {
          "Src": "bin/hello",
          "Dest": "/usr/local/bin/hello",
          "Mode": "0755",
          "Arch": ""
        }
      ],
      "Script": null,
      "Build": null
    },
    "Check": {"VersionFile": "", "Command": null, "Expect": ""},
    "Uninstall": null
  },
  "created_at": "2026-07-15T18:42:00Z"
}
```

The embedded manifest is encoded from Go structs and currently uses exported
field names in JSON. Treat it as descriptive catalog metadata; use the package
zip's YAML manifest as the authoring contract.

### `POST /v1/admin/app-packages`

Register metadata for an object that already exists in the object store. This
route does **not** upload or verify the object bytes.

```json
{
  "name": "acme/hello",
  "version": "1.2.3",
  "s3_key": "app-packages/acme/hello/1.2.3/acme_hello-1.2.3.zip",
  "sha256": "0123456789abcdef...",
  "manifest": {
    "SchemaVersion": 1,
    "Name": "acme/hello",
    "Version": "1.2.3",
    "Install": {
      "Mode": "binary",
      "Files": [
        {
          "Src": "bin/hello",
          "Dest": "/usr/local/bin/hello",
          "Mode": "0755",
          "Arch": ""
        }
      ],
      "Script": null,
      "Build": null
    },
    "Check": {"VersionFile": "", "Command": null, "Expect": ""},
    "Uninstall": null
  }
}
```

`name` and `version` must match the embedded manifest. Duplicate catalog keys
return `409`.

### Catalog query and deletion routes

| Route | Behavior |
| --- | --- |
| `GET /v1/admin/app-packages?name=<prefix>` | List records; `name` is an optional prefix filter. |
| `GET /v1/admin/app-packages/detail?name=<name>&version=<version>` | Return one exact record. |
| `DELETE /v1/admin/app-packages/detail?name=<name>&version=<version>` | Delete catalog metadata and return `204`. |
| `DELETE /v1/admin/app-packages/detail?name=<name>&version=<version>&delete_object=true` | Also attempt to delete the object; object deletion failure is logged after catalog deletion. |

### `POST /v1/app-packages/download-url` (endpoint mTLS)

Called by an endpoint to obtain a short-lived URL and expected digest:

```json
{"name": "acme/hello", "version": "1.2.3"}
```

```json
{
  "url": "https://object-store.example/...?signature=...",
  "sha256": "0123456789abcdef...",
  "expires_at": "2026-07-15T19:12:00Z"
}
```

The server derives and audits the endpoint identity from mTLS. The current
route authorizes an authenticated enrolled endpoint to request any exact
catalog name/version; desired-state applicability is enforced by the agent's
package application path, not by this URL endpoint.

See [Custom package manifest reference](custom-package-format.md) and
[Publish custom application packages](../guides/custom-app-packages.md).

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

Deletes the endpoint row and cascaded telemetry (`endpoint_labels`, `drift_reports`, `apply_failures`, `cron_last_run`, `cron_executions`). Does not stop the agent on the machine or remove Git config overrides.

**Response:** `204 No Content`

**Errors:** `400` invalid id, `404` not found, `503` admin unavailable

---

## Role-based access control (RBAC)

Requires Postgres. Operator mTLS alone is not sufficient: each request is authorized against the operator's assigned roles.

### Built-in roles

| Role | Access |
|------|--------|
| `global_admin` | Full access to all `/v1/admin/*` routes and `/v1/exports/audit/*` |
| `read_only` | `GET` on all `/v1/admin/*` routes |
| `security_logger` | `GET /v1/admin/audit-events`, `GET /v1/admin/audit-export`, `GET /v1/exports/audit/*` |
| `package_manager` | All methods on `/v1/admin/app-packages*` (list, publish, upload, delete) |
| `diagnostics_collector` | `POST /v1/admin/endpoints/*/diagnostics/collect`, `GET /v1/admin/diagnostics/*` |

The first operator created via bootstrap receives `global_admin`. Issue additional operators with explicit roles using `POST /v1/admin/operator-credentials` or `remotr admin credential stamp --role ...`.

Custom roles can be created with additional method/path rules. Built-in role rules are compiled into the server and cannot be modified at runtime.

### `GET /v1/admin/me`

Return the authenticated operator ID and assigned roles. Requires operator mTLS.

**Response `200 OK`:**

```json
{
  "operator_id": "11111111-1111-1111-1111-111111111111",
  "roles": ["global_admin"]
}
```

### `GET /v1/admin/rbac/roles`

List all roles with effective rules. Requires `global_admin`.

### `POST /v1/admin/rbac/roles`

Create a custom role. Requires `global_admin`.

```json
{
  "name": "fleet_observer",
  "description": "Read endpoint inventory only"
}
```

### `GET /v1/admin/rbac/roles/{name}`

Show one role. Requires `global_admin`.

### `DELETE /v1/admin/rbac/roles/{name}`

Delete a custom role. Built-in roles cannot be deleted. Requires `global_admin`.

### `POST /v1/admin/rbac/roles/{name}/rules`

Add a rule to a custom role. Requires `global_admin`.

```json
{
  "method": "GET",
  "path_pattern": "/v1/admin/endpoints/*"
}
```

`method` may be `*` for any HTTP method. `path_pattern` supports a trailing `*` prefix match.

### `DELETE /v1/admin/rbac/roles/{name}/rules/{ruleID}`

Remove a custom rule. Requires `global_admin`.

### `GET /v1/admin/operators`

List active operators with assigned roles. Requires `global_admin`.

### `PUT /v1/admin/operators/{operator_id}/roles`

Replace role assignments for an operator. Requires `global_admin`.

```json
{
  "roles": ["read_only"]
}
```

### `POST /v1/admin/operator-credentials`

Issue a new operator mTLS credential, register its fingerprint, and assign
optional roles. Requires an existing `global_admin` operator session.

```json
{
  "label": "siem-collector",
  "roles": ["security_logger"]
}
```

**Response `200 OK`:**

```json
{
  "operator_id": "22222222-2222-2222-2222-222222222222",
  "label": "siem-collector",
  "roles": ["security_logger"],
  "cert_pem": "-----BEGIN CERTIFICATE-----\n...",
  "key_pem": "-----BEGIN RSA PRIVATE KEY-----\n...",
  "ca_pem": "-----BEGIN CERTIFICATE-----\n..."
}
```

The private key is returned only in this response. The label is audit metadata,
not a credential lookup key.

**Example — SIEM collector credential:**

```bash
remotr admin credential stamp \
  --label siem-collector \
  --role security_logger \
  --out /etc/remotr-siem
```

The command writes `cert.pem`, `key.pem`, `ca.pem`, and `state.json`. To use stamped credentials with the operator CLI on another computer, rename the PEM files to `operator.crt`, `operator.key`, and `ca.crt` under your state directory. See [Using stamped credentials on a new computer](../guides/rbac.md#use-stamped-credentials-on-a-new-computer).

Unauthorized requests return `403 Forbidden` and are recorded as `authz.denied` audit events.

---

## Audit logging

Requires Postgres (`REMOTR_DATABASE_URL`). The server persists structured audit events for API activity and exposes them to operators and SIEM exporters.

Each request under `/v1/*` (except `/healthz`) is recorded with:

- `occurred_at`, `request_id`, HTTP method/path, status code
- Actor type (`operator`, `endpoint`, `anonymous`) and ID from mTLS when present
- Semantic `action` (for example `admin.endpoint.delete`, `agent.sync`)
- Optional `resource_type`, `resource_id`, and classified `details` fields.
  Each detail carries its path, sensitivity, and approved projection; arbitrary
  nested JSON is not accepted by the durable audit sink. Existing legacy maps
  remain as historical events but their unclassified detail values are omitted.

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
      "resource_id": "ep-1",
      "details": {
        "fields": [
          {
            "path": "value",
            "sensitivity": "sensitive-metadata",
            "projection": "presence",
            "present": true
          }
        ]
      }
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
| `GET /v1/admin/fleets` | `remotr fleet list` |
| `GET /v1/admin/endpoints` | `remotr endpoint list` |
| `GET /v1/admin/endpoints/{id}` | `remotr endpoint show` |
| `GET /v1/admin/endpoints/{id}/state-report` | `remotr endpoint state report` |
| `GET /v1/admin/fleets/{fleet}/state-report` | `remotr fleet state report` |
| `PUT /v1/admin/endpoints/{id}/labels/{key}` | `remotr endpoint label set` |
| `DELETE /v1/admin/endpoints/{id}/labels/{key}` | `remotr endpoint label unset` |
| `DELETE /v1/admin/endpoints/{id}` | `remotr endpoint remove` |
| `POST /v1/admin/endpoints/{id}/agent-upgrade` | `remotr endpoint agent upgrade` |
| `POST /v1/admin/endpoints/{id}/diagnostics/collect` | `remotr diagnostics collect` |
| `GET /v1/admin/diagnostics/{requestId}` | (poll during `remotr diagnostics collect`) |
| `GET /v1/admin/diagnostics/{requestId}/download` | (download during `remotr diagnostics collect`) |
| `POST /v1/diagnostics/upload-url` | `remotr-agent` diagnostic upload |
| `POST /v1/secrets/resolve` | `remotr-agent` secret provider |
| `POST /v1/app-packages/download-url` | `remotr-agent` custom package provider |
| `POST /v1/admin/fleets/{fleet}/agent-upgrade` | `remotr fleet agent upgrade` |
| `GET /v1/admin/endpoints/{id}/cron-report` | `remotr endpoint cron report` |
| `GET /v1/admin/fleets/{fleet}/cron-report` | `remotr fleet cron report` |
| `GET /v1/admin/endpoints/{id}/firewall-audit` | `remotr firewall logs` |
| `GET /v1/admin/change-requests` | `remotr change list` |
| `GET /v1/admin/change-requests/{id}` | `remotr change show` or `remotr change watch` |
| `POST /v1/admin/change-requests/{id}/authorize` | `remotr change authorize` |
| `POST /v1/admin/change-requests/{id}/pause` | `remotr change pause` |
| `POST /v1/admin/change-requests/{id}/resume` | `remotr change resume` |
| `POST /v1/admin/change-requests/{id}/revoke` | `remotr change revoke` |
| `POST /v1/admin/change-requests/{id}/baseline` | `remotr change baseline-promote` |
| `POST /v1/admin/fleets/{fleet}/baseline-adoptions` | `remotr change baseline-adopt` |
| `POST /v1/admin/secrets/versions` | `remotr secret upload` |
| `GET /v1/admin/secrets` | `remotr secret list` |
| `POST /v1/admin/secrets/activate` | `remotr secret activate` |
| `POST /v1/admin/secrets/revoke` | `remotr secret revoke` |
| `POST /v1/admin/app-packages/upload` | `remotr app publish` or `remotr package build --push` |
| `POST /v1/admin/app-packages` | No direct CLI; use `remotr app publish` for verified upload and registration |
| `GET /v1/admin/app-packages` | `remotr app list` |
| `GET /v1/admin/app-packages/detail` | `remotr app show` |
| `DELETE /v1/admin/app-packages/detail` | `remotr app delete` |
| `POST /v1/enroll` | `remotr-agent enroll` |
| `POST /v1/sync` | `remotr-agent` sync loop |
| `GET /v1/admin/audit-events` | `remotr logs list` |
| `GET /v1/admin/audit-export` | `remotr logs export-info` |
| `GET /v1/exports/audit/{path_key}` | SIEM collector (mTLS + path key) |
| `POST /v1/admin/operator-credentials` | `remotr admin credential stamp` |
| `GET /v1/admin/me` | (use API or `remotr rbac` management commands) |
| `GET/POST/DELETE /v1/admin/rbac/*` | `remotr rbac ...` |
| `GET /v1/admin/operators` | `remotr rbac operator list` |
| `PUT /v1/admin/operators/{id}/roles` | `remotr rbac operator set-roles` |
