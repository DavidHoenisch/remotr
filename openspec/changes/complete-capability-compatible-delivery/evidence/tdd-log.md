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
