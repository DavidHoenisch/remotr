# Remotr CA and certificate rotation

Operational guide for rotating the Remotr CA and re-issuing endpoint and operator credentials.

See also: [Agent deployment](../guides/agent-deployment.md#re-enrollment), [Production deployment](../guides/production-deployment.md), [Environment variables](../reference/environment-variables.md).

## Components

| Credential | Issued by | Stored on |
|------------|-----------|-----------|
| Remotr CA | Operator (offline or init script) | Server: `REMOTR_CA_CERT`, `REMOTR_CA_KEY` |
| Server TLS | Signed by Remotr CA | Server: `REMOTR_TLS_CERT`, `REMOTR_TLS_KEY` |
| Endpoint | `POST /v1/enroll` (CSR preferred) | Agent: `/var/lib/remotr/` |
| Operator | Bootstrap or existing operator mTLS | Operator: `~/.config/remotr/` |

## Before rotation

1. Export current endpoint list: `remotr endpoint list --json`
2. Schedule a maintenance window; agents cannot sync until re-enrolled or re-issued.
3. Generate a new CA key pair (2048-bit RSA minimum) and server certificate.

## CA rotation (full)

1. Stop `remotr-server` and agents.
2. Replace CA and server cert files on the server host.
3. Clear or archive `operator_credentials` and endpoint cert fingerprints in Postgres if fingerprints will change (full re-enroll).
4. Start server with empty operator table to emit a new bootstrap token, or pre-seed operator cert signed by the new CA.
5. Run `remotr bootstrap` with the new bootstrap token.
6. Create enrollment tokens and re-enroll each endpoint (`remotr-agent enroll --force`).
7. Verify: `remotr endpoint list`, agent sync logs, `GET /healthz`.

## Endpoint cert rotation (CA unchanged)

When the CA is stable but an endpoint cert nears expiry (~825 days):

1. Remove or note the old endpoint (`remotr endpoint remove <id>`) if your process requires it.
2. Issue a new enrollment token for the same fleet.
3. On the endpoint: `remotr-agent enroll --token … --force`
4. Confirm sync and labels in `remotr endpoint show <id>`.

## Operator cert rotation

1. An existing operator creates a new operator credential via future issuance API, or repeat bootstrap in controlled break-glass scenarios.
2. Replace files under `REMOTR_OPERATOR_STATE_DIR`.
3. Revoke old operator fingerprint in `operator_credentials.revoked_at` (manual SQL until revoke API exists).

## Rollback

Keep the previous CA and server certs for one release cycle. If rotation fails, restore old files and restart services before agents pick up the new CA trust bundle.
