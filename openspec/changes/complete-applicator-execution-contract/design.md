## Context

The active applicator umbrella specifies centralized rollback storage, typed redaction, stable desired hashes, and non-enforcing high-risk plans. The repository contains useful pieces—`internal/rollbackstore`, resource-level sensitivity labels, redacted result types, and caller-supplied Change-control plans—but they do not yet form one enforced contract.

The current rollback store is used principally by network recovery, does not enforce its configured per-resource attempt count, has no concrete TPM implementation, and cannot reserve a complete transaction before mutation. Sensitivity is attached to whole resource kinds and is not a generic field-level policy consumed by every output sink. Change control stores desired hashes and plans supplied by callers rather than deriving them from the same composed typed resources the agent executes.

This is safety-critical work. It crosses agent storage, resource schemas, execution, server planning, Sync, Postgres, Admin APIs, CLI output, diagnostics, backups, and provider recovery. The governing umbrella verification IDs and approved public seams remain the source of truth.

## Goals / Non-Goals

**Goals:**

- Make rollback promises durable, bounded, encrypted, crash-recoverable, and available to every provider that advertises rollback.
- Prevent secret values from reaching unsafe sinks through schema-field policy and safe output types rather than string scrubbing.
- Compute one canonical effective hash for authorizations, baselines, plans, leases, execution, and reporting.
- Derive non-enforcing high-risk plans from composed state and current endpoint evidence.
- Close umbrella tasks 2.9–2.11 with public-seam, negative, mutation, persistence, cleanup, restart, and VM recovery evidence.

**Non-Goals:**

- Add new package, distribution, storage, desktop, browser, or CMMC resource kinds.
- Implement capability-compatible artifact delivery, which belongs to `complete-capability-compatible-delivery`.
- Turn Remotr into a general backup product or retain rollback payloads on the server.
- Claim protection against an attacker that already controls endpoint root when the root-file key fallback is active.
- Weaken provider-specific safety checks or replace risk-specific acknowledgement with a generic transaction abstraction.

## Decisions

### 1. Use one crash-safe transaction record and explicit lifecycle

The agent transaction store owns records keyed by resource address, artifact digest, and attempt. A record moves through `reserved`, `staged`, `armed`, and one terminal state: `acknowledged`, `rolled_back`, `expired`, `superseded`, or `abandoned`. The store persists the complete encrypted record envelope under a same-filesystem temporary path, fsyncs data and directory metadata, and atomically renames it into place. Incomplete temporary records are never considered recoverable state and are cleaned on startup.

Reservation occurs before provider mutation. It accounts for the encrypted payload, metadata, filesystem overhead allowance, global disk bound, and retention state. Armed records are never pruned to make space. Count, age, and successful-prior-state limits are evaluated per resource; cleanup is deterministic under an injected clock. Startup scans and validates all active records before enforcement begins and surfaces corrupt or undecryptable armed records as a safety block.

Providers receive a transaction handle rather than constructing paths or writing adjacent backups. A provider that advertises rollback but cannot reserve and arm its required recovery state returns failed preflight before mutation.

Alternative considered: retain the current two-file metadata/payload layout and check capacity only at Save. Rejected because interruption can expose mismatched files and a late capacity check cannot honor a pre-mutation rollback promise.

### 2. Keep key protection provider-backed and observable

The store uses a `KeyProvider` contract with a concrete supported TPM-sealing implementation selected only when endpoint capability evidence proves it is usable. The root-only AES-256 key file remains an explicit fallback. The selected protection class and safe key identity are reported; raw keys and TPM authorization material are not.

Key-provider failure blocks new rollback-requiring mutation. Existing armed records remain recoverable through the key identity stored in authenticated metadata. Key rotation does not orphan armed records: historical decrypt-only key identities remain until no protected record references them.

Alternative considered: silently attempt TPM and fall back on every error. Rejected because transient TPM failure must not downgrade an endpoint's protection without an observable policy decision.

### 3. Make sensitivity part of each schema field contract

Every accepted resource field is classified `public`, `sensitive-metadata`, or `secret` in the same typed descriptor used for strict decode and validation. Registration fails when a field has no classification. Composite and collection fields inherit no permissive default; their elements are classified explicitly.

Raw desired and observed resource values remain inside the execution boundary. Logs, reports, diagnostics, persistence, backup, and rollback-metadata APIs accept safe summary or classified serialization types, not arbitrary resource values. Secret fields can emit an approved reference, version, safe fingerprint, or health projection defined by their descriptor; otherwise they are omitted. Error values crossing a provider boundary are converted into typed safe reason and diagnostic values before they reach generic infrastructure.

Alternative considered: keep a resource-level sensitivity enum and add more canary tests. Rejected because one public field can coexist with one secret field, and tests cannot make an unsafe generic serializer structurally impossible.

### 4. Canonical hashes come from effective typed desired state

The composition layer produces a canonical hash input for each stable resource address after schema normalization, defaults, provider selection, and secret-reference resolution to safe version identity. Maps and sets are sorted, omitted/unmanaged fields are distinguished from explicit values, and provider contract revision is included. Runtime observations, timestamps, endpoint identity, random values, and secret bytes are excluded.

The SHA-256 hash is computed by one shared package and carried unchanged through Change requests, baseline authorization, Execution leases, agent verification, and reports. Every consumer rejects a supplied hash that does not match recomputation at its trusted boundary.

Alternative considered: continue accepting caller-provided hashes. Rejected because authorization would then bind an assertion from the caller rather than the actual composed state.

### 5. Plans are derived, non-enforcing, and dependency-complete

The server plan builder iterates composed registered resources and provider descriptors to produce resource address, canonical hash, provider and contract revision, risk, authorization group, dependencies, activation targets, predicted effect codes, rollback class, and baseline eligibility. Provider predictions use bounded typed effect codes and safe parameters; free-form provider output is not plan evidence.

Current authenticated endpoint capability and non-enforcing agent Check/preflight evidence are joined to the server plan before target freeze. The plan includes the normal dependency closure for each high-risk component. A failed or missing prerequisite blocks the dependent high-risk resource. Break glass may bypass only the approval, window, and concurrency fields already permitted by the umbrella; it cannot bypass current preflight, dependency failure, redaction, rollback reservation, or hash verification.

Alternative considered: let Admin API clients construct `FleetPlan`. Rejected because clients cannot be trusted to reproduce composition, provider selection, dependency closure, or effective secret-version hashing.

### 6. Migrate behind comparison and fail-closed gates

During migration, tests may compare legacy caller-constructed plans with derived plans, but production authorization is enabled only for derived plans. Existing authorizations whose hashes cannot be recomputed remain visible and non-enforcing; operators create replacement requests rather than silently rebinding them. Existing rollback records are migrated only when their metadata and ciphertext validate; incompatible armed records block affected high-risk resources and produce recovery guidance.

### 7. Evidence follows risk at public seams

Each behavioral slice names its umbrella verification IDs and approved seam before the red test. Deterministic store tests inject clock, randomness, filesystem capacity, key provider, and crash points. Secret-canary evidence crosses agent logs, Sync, Postgres, Admin API, CLI, diagnostics, generic backup/restore, and rollback recovery. Access, connectivity, boot, and secret-bearing provider adoption includes the relevant Ubuntu VM interruption/recovery fixture. Focused mutation targets cover retention, reservation, classification, canonicalization, dependency closure, and bypass policy.

Verification IDs remain canonically owned by the umbrella capability while this child refines it. The central prefix registry explicitly authorizes this child as a modifier. Traceability inventory treats one authorized `ADDED` or `MODIFIED` delta as lineage for the canonical capability, retains the umbrella source in the manifest, and rejects unregistered or competing child refinements.

## Risks / Trade-offs

- [One shared store becomes a failure domain] → Validate on startup, isolate records by stable key, fail only affected rollback-requiring resources where safe, and retain explicit repair/abandon workflows.
- [Field descriptors drift from Go schemas] → Generate or lint descriptor coverage from strict schema types and fail registration/tests for unclassified accepted fields.
- [Canonical encoding changes invalidate authorizations] → Version the canonical hash contract and include its revision in plans and persisted authorization.
- [TPM differences create fragile support claims] → Advertise only passing provider/environment rows and retain an explicit root-key fallback with reduced-protection reporting.
- [Plans reveal sensitive intent] → Use safe typed effect codes and classified projections; never include secret values or credential-bearing argv.
- [Migration strands current high-risk work] → Keep legacy requests visible but non-enforcing and provide an explicit regeneration path from current composed state.

## Migration Plan

1. Add focused red tests for transaction lifecycle, canonical hashing, field classification, and derived plans without changing production selection.
2. Introduce versioned transaction envelopes and safe schema descriptors alongside current interfaces.
3. Migrate network recovery to the shared transaction handle and validate restart/acknowledgement behavior.
4. Migrate each rollback-advertising provider; refuse advertisement until its recovery evidence passes.
5. Route every generic output/persistence/backup sink through classified safe serialization and run the full canary path.
6. Add derived server plans and comparison diagnostics, then disable caller-constructed production authorization.
7. Recompute or replace legacy authorizations explicitly; never silently bind them to a new hash.
8. Run mutation, performance, persistence, cleanup, and Ubuntu VM evidence, update traceability, and close umbrella tasks 2.9–2.11.

Rollback of this change disables new high-risk enforcement while retaining validated recovery records and safe read-only reporting. It does not revert to caller-constructed authorization or unsafe generic serialization.

## Open Questions

None. Provider-specific adoption order is selected by existing advertised risk and the Ubuntu qualification dependency; it does not change this common contract.
