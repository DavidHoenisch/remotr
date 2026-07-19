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
