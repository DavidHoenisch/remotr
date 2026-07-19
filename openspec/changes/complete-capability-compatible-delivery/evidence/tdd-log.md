# TDD evidence log

## 2.1 — OS-AEC-088 canonical digest mismatch

- Public seam: capability-document model and validation.
- Red: `go test -mod=vendor ./internal/capabilitydoc -run '^TestCanonicalDigestMismatchIsRejected$' -count=1` failed to compile because `Document`, `Capability`, `Fact`, and `ErrDigestMismatch` did not exist.
- Green: the same focused selector passed after adding the versioned document model, deterministic set ordering, compact canonical JSON, SHA-256 digesting, and mismatch rejection.

## 2.2 — Registered contract and current-fact generation

- Public seam: modern capability-document generator.
- Red: `go test -mod=vendor ./internal/capabilitydoc -run '^TestGeneratorDerivesRegisteredContractsAndCurrentFacts$' -count=1` failed because `NewDefaultGenerator` did not exist.
- Green: the selector passed after deriving resource contracts from `resourceregistry`, fact-selected provider contracts and revisions from `providerregistry`, normalized allowlisted facts from the current endpoint snapshot, configured artifact schemas, and caller-supplied agent version metadata.

## 2.3 — OS-AEC-089 bounded validation

- Public seam: strict capability-document JSON decode and `Document.Validate`.
- Red: `go test -mod=vendor ./internal/capabilitydoc -run '^TestValidateRejectsMalformedOrUnboundedDocuments$' -count=1` failed because document/count/diagnostic bounds, `ValidationError`, and strict `Decode` did not exist.
- Green: table and strict-decode selectors passed for unsupported versions/schemas, oversized documents and entries, duplicate/conflicting capabilities and facts, malformed identifiers/revisions/values, unknown fields, and trailing JSON.
- Fuzz: `go test -mod=vendor ./internal/capabilitydoc -run '^$' -fuzz '^FuzzDocumentValidation$' -fuzztime=2s` passed 171,727 executions with 57 retained interesting inputs and bounded diagnostics.

## 2.4 — Authenticated Sync capability evidence

- Public seam: agent client and server `POST /v1/sync` over endpoint mTLS.
- Red (server): `TestSyncCapabilityDocumentBoundToMTLSEndpointIdentity` showed a tampered digest was ignored and still selected an artifact.
- Red (agent): `TestClientSendsCapabilityDocumentWithoutBearerCredential` failed because `sync.Request` had no capability-document field.
- Green: both focused selectors passed after modern run state generated current evidence for each request, the client serialized it without bearer authorization, and the server decoded and validated the bounded raw document only after deriving endpoint identity from the client certificate. The frozen legacy Sync fixture passed in the same focused run.

## 2.5 — Canonicalization and normalization fuzz properties

- Public seams: strict decode, canonical body/digest, duplicate validation, and default document generator.
- Red: the committed case-collision seed for `FuzzGeneratorFactNormalization` produced duplicate `desktop` facts after lowercase normalization.
- Green: committed seeds passed after normalized facts were sorted, deduplicated, signed, and validated before the generator returned them.
- Fuzz: two-second campaigns passed 116,929 canonicalization/digest/duplicate executions (18 interesting inputs) and 34,466 fact-normalization executions (25 interesting inputs). The earlier decode/validation campaign covers arbitrary bounded JSON input.

## 3.1 — Capability readiness persistence

- Public seam: authenticated Sync into the registry/Postgres persistence boundary.
- Red: `TestCapabilityDocumentPersistenceBindsAuthenticatedEndpoint` failed because no durable capability record, generated query model, or store methods existed.
- Green: the Postgres contract and authenticated Sync identity test passed after adding canonical-byte storage keyed by endpoint ID, digest and server receive time, an in-memory registry implementation, and fail-closed persistence before artifact selection.

## 3.2 — Unchanged capability write suppression

- Public seams: capability persistence contract and authenticated Sync observation.
- Red: `TestCapabilityDocumentPersistenceSkipsUnchangedDigest` showed a repeated identical digest was reported changed and rewrote the record.
- Green: an atomic conflict predicate now suppresses unchanged row updates; in-memory persistence mirrors it. The original durable receive time remains unchanged while the server's bounded current-request cache records the later authenticated observation for selection and reporting.

## 3.3 — OS-AEC-020/021 current evidence only

- Public seam: authenticated Sync across omission and offline reconnect.
- Red: `TestSyncModernAgentMissingOrInvalidCapabilityDocumentBlocks` showed an endpoint with persisted modern evidence could omit the current document and still receive the target artifact. The one-year reconnect selector already proved current valid evidence had no TTL dependency.
- Green: modern omission now clears current request evidence and returns successful structured `capabilityBlocked` without an artifact; persisted evidence is retained only for readiness. A reconnect one year later replaces both current and persisted evidence with the newly authenticated document.

## 3.4 — Restart-safe and secret-safe readiness

- Public seams: authenticated Sync, durable capability-document reads, and readiness after server reconstruction.
- Red: `TestCapabilityDocumentPersistenceRejectsMalformedStoredState` returned corrupt canonical bytes as usable readiness evidence, while `TestSyncRejectsSecretBearingCapabilityFactWithoutStorageOrDisclosure` accepted an arbitrary canary as an architecture fact and selected an artifact. `TestCapabilityPersistenceSurvivesServerRestart` established that a new server instance can read valid durable evidence without the process-local observation cache.
- Green: stored evidence now passes strict canonical decode, digest verification, protocol validation, and byte-for-byte canonical comparison before it can contribute readiness. Fact values are constrained to the documented normalized vocabulary, so arbitrary secret-bearing values receive a generic bounded 400 response and never reach storage or logs. All affected package tests pass.

## 4.1 — Versioned artifact requirement sets

- Public seam: the artifact requirement-set model used by composition, persistence keys, and delivery selection.
- Red: `go test -mod=vendor ./internal/artifactrequirements -run '^TestRequirementSetCanonicalDigestIsDeterministic$' -count=1` failed to compile because `Set` and `Requirement` did not exist.
- Green: the focused selector passes with a versioned, schema-explicit model that keeps exact resource and provider contract revisions separate, rejects malformed/duplicate/conflicting requirements, produces stable canonical JSON independent of input order, and freezes its SHA-256 digest.

## 4.2 — Canonical and lossless schema variants

- Public seams: repository composition, the additive artifact-variant writer, and Postgres requirement-evidence persistence.
- Red: `TestRenderArtifactVariantsIncludesCanonicalSchema1AndLosslessSchema0` failed because no variant renderer existed; `TestCompositionPersistsBoundedSchemaVariants` then showed composition persisted zero variants; `TestCompiledArtifactVariantPersistsRequirementEvidence` failed to compile before the generated-query-shaped persistence contract existed.
- Green: composition always emits canonical schema 1 and emits schema 0 only after parsing that encoding and proving it recanonicalizes byte-for-byte to schema 1. Both variants share a source digest, carry exact versioned resource/provider requirement sets, and have independent artifact and requirement digests. Composition stores them additively through a dedicated compiled-artifact variant table keyed by shared target, Release, artifact type, schema, and requirement-set digest; legacy compiled-artifact behavior remains unchanged.

## 4.3 — OS-AEC-090 whole-variant compatibility

- Public seam: compatibility selection over immutable composed variants and a current capability document.
- Red: `TestCompositionDoesNotCreateEndpointSpecificPartialVariant` failed to compile before whole-variant selection and bounded missing-requirement reporting existed.
- Green: the selector blocks when the document supports `resource:package` but lacks `provider:package/apt`, reports that exact revisioned requirement, and proves both shared schema variants still contain the complete package resource and authored provider field. Selection validates requirement-set digests and returns a defensive copy only when every schema, resource, and provider requirement is satisfied.

## 4.4 — Schema-bounded cardinality benchmarks

- Regression: `TestCompiledArtifactVariantsRemainSchemaBounded` composes exactly two shared variants and performs 400 compatible endpoint selections without increasing that cardinality.
- Native benchmark: `make benchmark-capability-variants` reported 400 endpoint selections over 2 variants at 2,782,136 ns/op, 1,012,328 B/op, and 14,809 allocs/op on the development host.
- Database benchmark: `BenchmarkCompiledArtifactVariantsDatabaseBoundedBySchema` performs transactional upsert/list cycles against a temporary Postgres table and asserts exactly two rows while reporting a 400-endpoint population. The benchmark is wired into the same Make target and skips safely when `REMOTR_BENCH_DATABASE_URL` is not configured; this run exercised the native benchmark and cardinality regression without an external benchmark database.

## 5.1 — OS-AEC-022 known legacy selection

- Public seam: authenticated Sync from an exactly mapped pre-capability agent version.
- Red: `TestSyncKnownLegacyAgentSelectsLosslessSchema0` received canonical `schemaVersion: 1` because Sync did not consult bounded variants or any reviewed legacy profile.
- Green: versioned mapping 1 assigns only schema 0 and the minimal historical command contract to exact agent version `v0.1.12`; adjacent unknown versions do not inherit the mapping. On-demand and Postgres stores now expose additive variant readers, and Sync sends the complete lossless schema-0 variant through the same compatibility selector used by modern documents. The independently frozen legacy fixture and modern capability regressions pass.

## 5.2 — OS-AEC-018/019/020 version classification

- Public seam: authenticated Sync classification when current capability evidence is absent or invalid.
- Red: `TestSyncUnknownLegacyAgentUsesMinimalBaseline` received canonical schema 1 and a package artifact for an unrecognized version; the expanded `TestSyncModernAgentMissingOrInvalidCapabilityDocumentBlocks` received HTTP 400 for a known modern agent's bad digest and would serve a first-time modern omission before any evidence had been persisted.
- Green: exact known legacy versions retain their reviewed profile; every unknown no-document version receives only a fixed schema-0 command baseline and blocks on newer resource/provider contracts. Exact modern versions are classified only as requiring a valid current document—their version grants no runtime capability—and omission or invalid evidence now produces successful structured `capabilityBlocked` without artifact selection or persistence, including on first Sync.

## 5.3 — OS-AEC-023 blocked existing endpoint

- Public seam: an authenticated existing endpoint reporting its active artifact while the global target Release requires an unavailable provider contract.
- Red: `TestSyncExistingEndpointCapabilityBlockedRetainsActiveArtifact` returned the correct blocked target but dropped the endpoint's reported `release-active`/`digest-active` check-in entirely.
- Green: blocked delivery seeds its active state only from durable preexisting check-in evidence and retains `release-active`/`digest-active`; an unoffered self-report is not allowed to advance it. The incompatible target remains `release-target`, no artifact is sent, and target state is never written as active.

## 5.4 — OS-AEC-024 unmanaged new endpoint

- Public seam: first authenticated Sync from an endpoint that cannot satisfy any target variant and reports no active artifact.
- Red: `TestSyncNewEndpointCapabilityBlockedIsUnmanaged` returned `capabilityBlocked` without the required explicit `unmanaged` state.
- Green: blocked responses now derive unmanaged state only from durable last-check-in evidence or delivery state promoted from an exact stored offer acknowledgement. A new incompatible endpoint is explicitly unmanaged and receives no artifact, while the existing-endpoint selector remains managed on its retained active state.

## 5.5 — OS-AEC-091 exact offer acknowledgement

- Public seam: two authenticated Sync requests separated by server reconstruction, with the second reporting the exact Release/digest offered by the first.
- Red: `TestSyncUnacknowledgedOfferDoesNotAdvanceActiveArtifact` failed to compile because no delivery-state persistence seam existed; the prior server also wrote every selected target directly to check-in state before sending the response.
- Green: registry memory and Postgres now persist target, offered, active, schema versions, timestamps, blocked requirements, and unmanaged state separately. Sending bytes records only offered state. A later `lastReleaseRef`/`lastDigest` pair promotes active and updates check-in only when both exactly match the stored offer, then clears offered state. Arbitrary self-reports do not advance active, and the transition survives a new server instance.

## 5.6 — OS-AEC-025 compatible approved upgrade

- Public seam: a capability-blocked Sync for an endpoint with an operator-approved desired agent version.
- Red: `TestSyncCapabilityBlockedIncludesApprovedAgentUpgrade` returned the correct missing resource contract but omitted the approved upgrade because blocked paths returned before upgrade instruction evaluation.
- Green: a reviewed `v1.2.4` profile proves the agent-shipped `resource:package@package-v1` contract and capability-document/schema support, so normal desired-version authorization can add the existing upgrade instruction. The target remains blocked, no artifact or active digest is recorded, and a negative selector proves agent versions never satisfy missing runtime provider requirements.

## 6.1 — OS-AEC-026 Admin API and CLI state

- Public seams: authenticated `GET /v1/admin/endpoints/{id}`, the Admin client model, and human `remotr endpoint show` rendering.
- Red: `TestAdminEndpointReportsCapabilityDeliveryState` received only endpoint ID and Fleet; `TestEndpointOutputSeparatesTargetOfferedAndActiveRelease` failed to compile because the client model and dedicated renderer had no delivery-state vocabulary.
- Green: additive optional fields now expose target, offered, and active Release/digest/schema independently, preserving schema 0 through pointer encoding. Capability digest/receive time, blocked target, bounded revisioned missing requirements, and unmanaged state flow through the API/client. Human output labels each state explicitly while JSON remains complete and backward-compatible by omission.

## 6.2 — desktop and JSON compatibility

- Public seams: frozen legacy/modern Admin endpoint JSON fixtures, desktop workspace mapping, generated desktop bindings, and endpoint inventory/detail rendering.
- Red: `TestWorkspaceServiceLoadsCompleteAndSectionForbiddenResults` failed to compile because the desktop row had no target/offered/active fields; the focused frontend test then could not find distinct Active, Target, or Offered Release labels.
- Green: legacy API records decode with zero optional delivery state and re-encode without inventing new fields. Modern records preserve all three Release refs and schema 0. Desktop rows carry the additive delivery/capability fields end to end, keep `releaseRef` solely as an active-state compatibility alias, and render the inventory column and detail evidence with explicit Active, Target, and Offered labels. Admin, desktop Go, all 73 frontend tests, frontend type-checking, and lint pass.

## 6.3 — OS-AEC-092 active telemetry attribution

- Public seam: an authenticated, existing endpoint submits a bounded state report while its current target is blocked by a missing runtime provider contract.
- Red: `TestSyncBlockedEndpointTelemetryRemainsAttributedToActiveDigest` returned successful `capabilityBlocked` but persisted no state report because the blocked selection path returned before telemetry handling.
- Green: after recording blocked delivery state, the server resolves the endpoint's durable acknowledged active Release/digest, binds the admitted report to that identity, and persists it without advancing check-in or active state. The target remains `release-target`; telemetry remains under `release-active`/`digest-active`. The full server package suite passes.

## 6.4 — successful blocked outcome and load shaping

- Public seams: malformed-current-document blocking with bounded pending telemetry, agent HTTP response classification, and the injected polling-delay decision.
- Red: `TestSyncCapabilityBlockedPreservesBoundedPendingTelemetry` showed labels, usernames, system evidence, state report, and apply failure were all dropped on the early invalid-document block; `TestCapabilityBlockedSuccessKeepsStablePollingCadence` failed to compile because the loop had no directly testable delay boundary.
- Green: request telemetry and control intents are validated before capability selection, every successful blocked path persists admitted incoming telemetry against durable active state, and blocked responses acknowledge valid reboot/network intents. The agent decodes HTTP 200 `capabilityBlocked` without permanent/overload classification, logs it as an expected outcome, retains its active artifact, clears accepted telemetry normally, resets transient backoff, and uses stable endpoint-derived success jitter. Full server, agent Sync, and agent command suites pass.

## 7.1 — mixed schema/capability fixtures

- Public seam: one authenticated server/registry population driven by lossless command and provider-revision package desired-state fixtures.
- Red: five mixed endpoint histories passed except modern-to-known-legacy downgrade; the server saw persisted modern readiness evidence and blocked the explicit reviewed `v0.1.12` current version instead of using its conservative schema-0 profile.
- Green: persisted documents remain forbidden as current selection evidence, while an explicit exact known-legacy current version may use only its reviewed schema-0 mapping. The fixture proves legacy schema 0, modern schema 1, exact missing provider revision, current normalized fact replacement, downgrade with schema-1 active/schema-0 offered separation, and recovery on reconnect with a new valid current document. The full server suite passes.

## 7.2 — authenticated mixed-capability load

- Public seam: `make load-capability-mixed-400` against the disposable Compose server/Postgres stack with 400 unique enrolled mTLS identities and five equal populations.
- Red: the first real collection produced 1,120 shared HTTP 500 responses because Postgres JSONB normalized requirement-set key order while the reader required byte-exact canonical JSON. A focused decoder regression then failed to compile until a JSONB-aware strict persisted decoder existed.
- Green: JSONB-normalized evidence is strictly decoded and verified against its separately indexed canonical digest; unknown fields and digest mismatch remain fail-closed. The fresh collection completed every wave without error: mixed target 400/400 success, 240 blocked, 80 unmanaged, 3,209,520 request bytes, 13,600 artifact bytes, and 309.1/334.2/341.2 ms p50/p95/max. Reconnect was 80/80 unchanged with a 2.981-second spread and peak eight starts per 100 ms. Cache growth was exactly four variants; database and process counters plus 80-row telemetry readbacks are recorded in `engineering/testing/capability-delivery-load-evidence-2026-07-18.md`.
