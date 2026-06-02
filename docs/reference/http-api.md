# HTTP API reference

Base URL: `https://<remotr-server>:8443`

All JSON endpoints use `Content-Type: application/json` unless noted.

## Authentication summary

| Endpoint | Auth |
|----------|------|
| `GET /healthz` | None |
| `POST /v1/enroll` | Enrollment token in JSON body |
| `POST /v1/sync` | mTLS client certificate (endpoint credential) |
| `POST /v1/admin/bootstrap` | Bootstrap token in JSON body |
| `GET/POST /v1/admin/*` | mTLS client certificate (operator credential) |
| `POST /v1/webhooks/git` | Optional `X-Remotr-Git-Webhook-Secret` header |

**Endpoint identity** is always derived from the TLS client certificate (SAN / fingerprint mapping). Request bodies must not carry a trusted endpoint ID.

---

## `GET /healthz`

Liveness probe. No authentication.

**Response:** `200 OK`, body `ok`

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
  }
}
```

All fields except `lastDigest` are optional telemetry.

### Response `200 OK`

**Unchanged artifact:**

```json
{
  "unchanged": true,
  "digest": "sha256:...",
  "remediationPolicy": "auto"
}
```

**New or updated artifact:**

```json
{
  "unchanged": false,
  "releaseRef": "commit-sha-or-label",
  "digest": "sha256:...",
  "artifactYaml": "configurations:\n  - name: ...",
  "remediationPolicy": "auto"
}
```

| Field | Description |
|-------|-------------|
| `remediationPolicy` | `auto` or `report` for the endpoint's fleet |
| `artifactYaml` | Raw deployable artifact bytes (YAML) |

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

Get one endpoint. Requires operator mTLS. Same object shape as list entries.

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
| `POST /v1/admin/git-sync` | `remotr git sync` |
| `GET /v1/admin/endpoints` | `remotr endpoint list` |
| `GET /v1/admin/endpoints/{id}` | `remotr endpoint show` |
| `POST /v1/enroll` | `remotr-agent enroll` |
| `POST /v1/sync` | `remotr-agent` sync loop |
