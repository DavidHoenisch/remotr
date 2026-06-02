# ADR 001: Vendored third-party allowlist

## Status

Accepted

## Context

Remotr targets reproducible production builds and a minimal attack surface. Contributors need clear rules for when a new dependency is acceptable.

## Decision

1. **Vendor all third-party modules.** Production builds use `go build -mod=vendor`; CI must not fetch modules at build time.
2. **Allowlist for v1:**
   - `github.com/go-chi/chi/v5` — HTTP routing
   - `gopkg.in/yaml.v3` — YAML parse/emit for deployable artifacts
   - `github.com/jackc/pgx/v5` — Postgres (server registry only)
   - `github.com/google/uuid` — UUID types for sqlc-generated queries
3. **Prefer stdlib** for everything else (TLS, crypto, JSON, `database/sql` patterns via pgx only where needed).
4. New dependencies require an ADR amendment and explicit review.

## Consequences

- Supply chain risk is bounded to a small, reviewed set.
- Security review and `gosec` scope stay manageable.
- Features that would normally pull heavy SDKs (Git hosting clients, cloud APIs) must use stdlib or subprocess (`git`) instead.
