## 1. Establish Protocol Traceability

- [x] 1.1 Register OS-AEC-088 through OS-AEC-092 and reconcile OS-AEC-016 through OS-AEC-026 in `test/traceability.yaml` with authenticated Sync, Admin API/CLI, compatibility, persistence, and load selectors.
- [x] 1.2 Freeze current legacy Sync/artifact delivery fixtures and independently known active-Release behavior before adding capability fields.
- [x] 1.3 Define and document capability-document, capability-ID, contract-revision, normalized-fact, document-size, entry-count, and diagnostic bounds.

## 2. Produce and Validate Endpoint Capability Documents

- [x] 2.1 For OS-AEC-088, write and observe a focused red canonical digest mismatch test, then implement the versioned document model and deterministic canonical encoding.
- [x] 2.2 Generate modern agent documents from registered resource/provider contracts, supported artifact schemas, current normalized facts, and agent version metadata rather than a handwritten capability list.
- [x] 2.3 For OS-AEC-089, write and observe table/fuzz red cases for oversized, duplicate, conflicting, malformed, and unsupported-version documents, then implement bounded agent and server validation.
- [ ] 2.4 Add the capability document additively to authenticated Sync and prove endpoint mTLS identity binds the submitted evidence without introducing a bearer credential.
- [ ] 2.5 Add bounded fuzz properties and committed seed regressions for document decoding, canonicalization, digesting, duplicate detection, and fact normalization.

## 3. Persist Readiness Without Reusing Stale Selection Evidence

- [ ] 3.1 Write and observe a focused red persistence test for one valid capability document, then store digest, canonical document, server receive time, and endpoint identity through the registry/Postgres boundary.
- [ ] 3.2 Suppress redundant database writes for unchanged digests while retaining current request evidence for selection and observable receive state.
- [ ] 3.3 For OS-AEC-020 and OS-AEC-021, write red modern-omission and offline-reconnect tests, then ensure selection never substitutes the persisted document for missing current evidence.
- [ ] 3.4 Prove capability persistence and readiness reporting survive server restart, malformed stored state fails closed, and no secret-bearing fact value enters logs or storage.

## 4. Compose Bounded Requirement-Aware Variants

- [ ] 4.1 Define a versioned artifact requirement-set model and deterministic digest covering schema, resource capability IDs/revisions, and provider requirements.
- [ ] 4.2 Write and observe red schema-1 and behaviorally lossless schema-0 composition cases, then persist bounded variants through the compiled-artifact store.
- [ ] 4.3 For OS-AEC-090, write a red field/resource-removal compatibility case, then enforce that composition cannot create endpoint-specific partial variants.
- [ ] 4.4 Add database and native allocation benchmarks proving variant count is bounded by declared schema variants rather than fleet or endpoint cardinality.

## 5. Implement Capability-Compatible Delivery State

- [ ] 5.1 For OS-AEC-022, write and observe a red known-legacy-agent selection test, then add a reviewed versioned server mapping to the minimal compatible schema-0 profile.
- [ ] 5.2 For OS-AEC-018 through OS-AEC-020, implement exact known-legacy, unknown-version, modern-missing, and modern-invalid document behavior without inferring modern runtime support from agent version.
- [ ] 5.3 For OS-AEC-023, write and observe a red existing-endpoint incompatibility test, then retain the active artifact and return structured `capability_blocked` while global Release advances.
- [ ] 5.4 For OS-AEC-024, write and observe a red new-endpoint incompatibility test, then retain explicit unmanaged/blocked state without sending partial desired state.
- [ ] 5.5 For OS-AEC-091, write and observe a red unacknowledged-offer test, then separate target, offered, and active artifact state so only successful exact-digest processing advances active state.
- [ ] 5.6 For OS-AEC-025, add optional compatible agent-upgrade instructions without marking the blocked target active or bypassing normal upgrade authorization.

## 6. Expose Active, Target, and Blocked State

- [ ] 6.1 For OS-AEC-026, add focused Admin API and CLI red tests, then expose target, offered, active Release/digest/schema, capability digest/receive time, unmanaged state, and bounded missing requirements additively.
- [ ] 6.2 Update desktop/API consumers and JSON compatibility fixtures so older records omit new fields safely and newer records never conflate active with target Release.
- [ ] 6.3 For OS-AEC-092, write a red pending-telemetry attribution test, then persist blocked-endpoint telemetry under its active artifact digest.
- [ ] 6.4 Treat `capability_blocked` as a successful authenticated protocol outcome that preserves bounded pending telemetry and stable jitter without rapid retry or overload semantics.

## 7. Verify Mixed Fleets and Close the Gate

- [ ] 7.1 Add mixed schema/capability integration fixtures covering legacy schema 0, modern schema 1, missing revisions, current fact changes, agent downgrade, and capability recovery on reconnect.
- [ ] 7.2 Extend the authenticated 400-endpoint load harness with compatible, blocked-existing, unmanaged-new, telemetry-carrying, and reconnecting populations; record latency, bytes, errors, cache cardinality, database behavior, and request-wave spread.
- [ ] 7.3 Run focused mutation tests for document validation, legacy mapping, requirement satisfaction, variant selection, active/offered transitions, and telemetry attribution with no unexplained relevant survivor.
- [ ] 7.4 Run focused tests after every red/green slice, then mixed-version integration, Postgres persistence, authenticated load, `make test`, strict OpenSpec validation, traceability validation, and documentation validation.
- [ ] 7.5 Promote OS-AEC-016 through OS-AEC-026 and OS-AEC-088 through OS-AEC-092 only after required selectors pass, complete foundation task 10.5, and close umbrella task 3.11 after this change is accepted.
