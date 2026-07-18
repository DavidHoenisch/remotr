## Context

Remotr currently selects and serves compiled artifacts without a modern endpoint capability document in the authenticated Sync protocol. Provider facts exist locally and author-time capability validation exists, but the server cannot prove that the endpoint making the current request supports a schema, resource contract revision, or runtime backend. The foundation load audit therefore cannot exercise mixed schema/capability populations or a real `capability_blocked` outcome.

The umbrella already defines the behavioral direction in OS-AEC-016 through OS-AEC-026. This child turns those requirements into one versioned protocol and state machine while preserving current legacy agents and global Release advancement.

## Goals / Non-Goals

**Goals:**

- Authenticate, validate, bound, digest, persist, and report endpoint capability documents.
- Use only the current Sync document to select a compatible bounded artifact variant.
- Preserve the last processed artifact when a newer Release is incompatible, without claiming it is current.
- Make new incompatible endpoints explicitly unmanaged and blocked.
- Keep legacy-agent behavior conservative, deterministic, and testable.
- Provide mixed-version, malformed, reconnect, downgrade, persistence, and authenticated load evidence.

**Non-Goals:**

- Implement general conditional resource applicability or arbitrary fact expressions.
- Generate arbitrary per-endpoint artifact variants.
- Remove resources or fields from desired state to satisfy an endpoint.
- Infer runtime support from agent version alone for modern agents.
- Qualify a provider merely because its capability ID is reported.

## Decisions

### 1. Define one bounded current-Sync capability document

The request contains `documentVersion`, supported artifact schema versions, stable capability IDs with contract revisions, normalized provider facts, agent version metadata, and a SHA-256 digest of the canonical document body. Lists and maps are canonicalized and sorted before digesting. The request is bounded by maximum document bytes, capability count, fact count, identifier length, and revision grammar.

The document is authenticated by the endpoint's existing mTLS Sync identity; it is not a separate bearer assertion. The server recomputes the digest and rejects duplicate IDs, conflicting revisions, unsupported document versions, invalid normalized facts, and impossible schema declarations. The agent generates the document from registered resources/providers and current facts rather than a hand-maintained list.

Alternative considered: add one capability bitset keyed to agent release. Rejected because runtime providers and schema contracts evolve independently and a bitset is neither self-describing nor revision-safe.

### 2. Separate current selection evidence from persisted readiness state

The server persists the latest valid capability document with server receive time, endpoint identity, and digest for readiness preview and operator reporting. Artifact selection for a Sync uses only the valid document in that authenticated request. A persisted document is never substituted when a modern agent omits or corrupts current evidence.

The current and persisted documents may be compared for change reporting, but no wall-clock TTL determines delivery. An offline endpoint cannot receive anything; on reconnect it supplies current evidence before selection.

Alternative considered: trust the latest persisted document until a configurable TTL. Rejected because any TTL is arbitrary and can serve unsupported state after endpoint-local changes.

### 3. Compose and persist only bounded variants

Composition produces a versioned requirement set for every compiled variant. This change supports canonical schema 1 and a schema-0 compatibility variant only when conversion is behaviorally lossless. Variants are keyed by Fleet/override source, Release ref, source digest, schema version, and requirement-set digest and are stored through the existing compiled-artifact persistence path.

The server does not create combinations by deleting unsupported resources, fields, or providers. If no declared bounded variant satisfies the current capability document, selection blocks.

Alternative considered: compile an artifact tailored to each endpoint's capabilities. Rejected because it creates unbounded cache cardinality and allows silent policy erosion.

### 4. Make delivery an explicit state machine

For each endpoint the server distinguishes:

- `active`: the Release ref, schema, and digest of the last artifact the endpoint acknowledged as successfully processed;
- `target`: the current global or endpoint-override Release ref available for selection;
- `offered`: a compatible artifact sent in the current response but not yet acknowledged;
- `capability_blocked`: no bounded target variant is compatible, with exact missing requirements;
- `unmanaged`: a new endpoint has no active artifact and is capability-blocked.

An existing blocked endpoint continues checking its active artifact locally and may continue sending telemetry for that artifact. The server never labels that telemetry as target-Release evidence. A later compatible Sync receives the current target variant. An offered artifact becomes active only after the existing processing acknowledgement/digest contract succeeds.

Alternative considered: advance the endpoint's active ref when the server offers an artifact. Rejected because delivery is not proof of parse, validation, or successful activation.

### 5. Keep legacy behavior in a reviewed server mapping

Known pre-capability agent versions map to a conservative schema-0 profile maintained and tested on the server. Unknown versions receive only the minimal legacy baseline. A version known to implement capability documents that omits or sends an invalid document is capability-blocked and retains its active artifact.

Agent version may select the legacy compatibility mapping; it never augments a valid modern document with inferred runtime providers. The mapping is versioned, bounded, and removal follows the umbrella compatibility policy.

Alternative considered: let missing documents always mean all legacy capabilities. Rejected because unknown or regressed modern agents would receive behavior they did not prove.

### 6. Report capability and Release state additively

Endpoint and Fleet reporting expose target Release, active Release, offered Release when present, active schema/digest, current capability digest, latest persisted document receive time, blocked missing requirement IDs/revisions, and whether the endpoint is unmanaged. Human output is concise; JSON remains complete and stable. Diagnostics contain bounded identifiers and never include secret-bearing fact values.

Desktop consumers use the Admin API contract rather than reading persistence directly. Existing readers tolerate absent new fields during the compatibility window.

### 7. Preserve pending telemetry and load shaping

Capability-blocked responses are successful authenticated protocol outcomes, not overload or credential failures. Agents retain and submit bounded pending telemetry for their active artifact, keep the ordinary successful polling cadence with stable jitter, and do not enter rapid retry loops. Malformed local capability generation is a permanent local validation failure with bounded diagnostics.

The authenticated load harness adds mixed schema/capability populations, blocked existing endpoints, unmanaged new endpoints, capability changes on reconnect, and Release fan-out. It measures variant-cache cardinality, response classification, payload bytes, latency, database behavior, and request-wave shaping.

## Risks / Trade-offs

- [Capability documents become self-asserted support claims] → Generate from registered contracts/current facts and still require provider-matrix evidence before advertisement.
- [Variant cache grows without bound] → Permit only schema-defined bounded variants and reject field-dropping combinations.
- [Blocked endpoints appear compliant with a new Release] → Track active and target refs separately and bind telemetry to the active digest.
- [Legacy mapping becomes permanent hidden policy] → Version it, test exact versions, emit migration visibility, and retain the umbrella removal gate.
- [Capability churn amplifies database writes] → Digest unchanged documents and suppress redundant persistence while recording current request evidence in memory for selection.
- [Mixed fleets synchronize retries] → Treat blocked delivery as a normal response and preserve deterministic jitter/backoff policy.

## Migration Plan

1. Add protocol model and canonical document tests without changing delivery selection.
2. Have modern agents send valid capability documents while the server validates, persists, and reports them in observation-only mode.
3. Generate and persist explicit requirement sets for current schema-1 and lossless schema-0 variants.
4. Compare legacy and capability-aware selections in tests and diagnostics; resolve every mismatch explicitly.
5. Enable capability-aware selection for modern agents and conservative mapping for known legacy versions.
6. Add active/target/offered/blocked reporting and update CLI/desktop consumers additively.
7. Run mixed-version integration, downgrade, reconnect, persistence, and authenticated load evidence; update traceability and foundation task 10.5.
8. Close umbrella task 3.11 only after all OS-AEC-016 through OS-AEC-026 evidence required for advertised behavior passes.

Rollback disables capability-aware target selection while retaining persisted documents and additive reporting. It must not mark an unacknowledged offered Release active or discard an endpoint's last processed artifact.

## Open Questions

None. General conditional applicability remains a separate future OpenSpec change.
