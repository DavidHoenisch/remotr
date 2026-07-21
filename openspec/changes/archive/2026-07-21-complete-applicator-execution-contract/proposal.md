## Why

The applicator umbrella has broad execution and change-control code, but its protected rollback storage, schema-driven sensitivity, and automatically derived high-risk plans remain partial. Those gaps prevent the umbrella from making a trustworthy safety claim and block provider qualification and CMMC-oriented high-risk resources.

## What Changes

- Complete one agent-wide protected transaction store for every provider that advertises transactional or best-effort rollback, including durable reservation, count/age/disk bounds, atomic activation, checksums, encryption, TPM preference, root-key fallback reporting, restart recovery, and terminal cleanup.
- Replace resource-level-only sensitivity labels with schema-field classification consumed by generic serialization and output sinks before values can enter logs, reports, diagnostics, persistence, backups, or rollback metadata.
- Generate canonical effective desired-state hashes from composed typed resources, including safe secret provider/version identity without secret bytes.
- Generate non-enforcing high-risk plans from the same composed resources rather than accepting caller-constructed hashes or effects, and integrate dependency blocking, authorization grouping, baseline invalidation, and restricted break-glass behavior.
- Verify secret canaries and recovery behavior through public agent, Sync, Postgres, Admin API, CLI, diagnostic, backup, restart, and Ubuntu VM seams selected by risk.
- Close umbrella tasks 2.9, 2.10, and 2.11 only after this change's implementation and evidence are accepted.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `applicator-execution-contract`: Refine the umbrella's rollback-storage, typed-redaction, effective-hash, and reviewable-plan requirements so their cross-provider integration and observable completion conditions are explicit.

## Impact

- Agent execution, rollback storage, provider factories, resource registry, composed-resource hashing, dependency processing, and restart recovery.
- Change-control plan construction, authorization/baseline matching, persistence, Admin API/CLI review output, and authenticated Sync execution leases.
- Schema metadata and every output or storage sink that can receive desired, observed, provider, error, diagnostic, backup, or rollback data.
- Traceability and evidence for the governing umbrella verification IDs; this child remains linked to the active umbrella and is not archived ahead of the umbrella capability baseline.
