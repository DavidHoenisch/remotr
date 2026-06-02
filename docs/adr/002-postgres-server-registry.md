# ADR 002: Postgres for server registry

## Status

Accepted

## Context

The server registry holds enrollment tokens, endpoint records, operator credentials, drift telemetry, and release-ref metadata. It must support concurrent agents and admin CLI access.

## Decision

Use **Postgres** as the v1 server registry backend via **sqlc**-generated queries and **pgx** connection pooling.

SQLite is not supported in v1 because:

- Compose and production deployments already standardize on Postgres for multi-writer admin + agent sync telemetry.
- sqlc + pgx gives typed queries without an ORM.
- Fleet settings, enrollment token consumption, and label upserts need row-level concurrency semantics Postgres provides.

The **configuration repository** remains Git-only; Postgres never stores desired state YAML.

## Consequences

- Operators run Postgres (or the provided Docker Compose stack) alongside `remotr-server`.
- `REMOTR_DATABASE_URL` is required for enrollment, admin API, drift storage, and dynamic release ref.
- Memory registry remains for unit tests only.
