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
- Green: every capability-blocked path now records a bounded, authenticated `lastReleaseRef`/`lastDigest` pair as active check-in evidence before returning. The incompatible target remains `release-target`, no artifact is sent, and target state is never written as active.

## 5.4 — OS-AEC-024 unmanaged new endpoint

- Public seam: first authenticated Sync from an endpoint that cannot satisfy any target variant and reports no active artifact.
- Red: `TestSyncNewEndpointCapabilityBlockedIsUnmanaged` returned `capabilityBlocked` without the required explicit `unmanaged` state.
- Green: blocked responses now derive unmanaged state only from durable last-check-in evidence or a bounded current active Release/digest pair. A new incompatible endpoint is explicitly unmanaged and receives no artifact, while the existing-endpoint selector remains managed on its retained active state.
