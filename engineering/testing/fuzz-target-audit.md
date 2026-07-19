# Native fuzz target audit

## Current discovery status — 2026-07-18

The root-module discovery in `scripts/fuzz-all.sh` now owns 42 repository-native
`Fuzz*` targets. The five additional native targets in the nested `desktop`
module are intentionally excluded from root vendored test invocation. Vendor
fuzz functions are never Remotr verification evidence.

Task 5.3 of `complete-applicator-execution-contract` added these bounded
properties. Each target has malformed, boundary, and representative seeds and
runs its corpus under ordinary tests.

| Target | Verification reference | Durable property | Bound | Active campaign |
| --- | --- | --- | --- | --- |
| `rollbackstore.FuzzTransactionEnvelopeDecoder` | `OS-AEC-080` | Arbitrary envelopes either fail strict decoding or retain an identical, valid canonical form; unknown fields and trailing values reject. | Raw envelope ≤64 KiB. | 10 seconds, 1,130,192 executions, pass. |
| `rollbackstore.FuzzRetentionCleanupPreservesArmedAndBoundsTerminalRecords` | `OS-AEC-081` | Cleanup is idempotent, keeps armed recovery, bounds per-resource attempts and successful payloads, expires eligible metadata, and preserves deterministic ordering. | At most 20 generated terminal attempts plus one armed recovery; attempts limit 1–10; injected clock. | Minimized oracle regression retained; corrected 10-second campaign, 9,983 executions, pass. |
| `resourceregistry.FuzzSchemaClassificationRejectsIncompleteOrInvalidPolicies` | `OS-AEC-083` | A fixed strict composite schema registers exactly when every independently enumerated leaf has a valid sensitivity/projection pair and no unknown descriptor exists. | Six accepted leaves; extra path ≤128 bytes. | 10 seconds, 1,152,047 executions, pass. |
| `effectivehash.FuzzCanonicalHashIsOrderInvariantAndSecretSafe` | `OS-AEC-085` | Map, set, default, and secret-identity order cannot change canonical output; list order and safe secret-version identity remain significant; digest matches SHA-256 of valid canonical JSON. | Two generated values ≤128 bytes each and fixed-size typed structures. | 10 seconds, 1,239,969 executions, pass. |
| `changecontrol.FuzzPlanDependencyGraphIncludesExactNormalClosure` | `OS-AEC-087` | Every transitive normal prerequisite appears exactly once; cycles terminate; unknown and cross-group dependencies reject without partial persistence. | One to eight nodes and at most 64 edge bytes. | 10 seconds, 3,388,324 executions, pass. |

The affected-package discovery command found and ran nine seed-corpus targets,
including these five and the four pre-existing targets in the same packages.
The minimized retention-oracle input is indexed in
`engineering/testing/fuzz-regressions.md` and remains part of ordinary tests.

## Original baseline — 2026-07-11

The original audit covered the nine root-module targets present at that time.
Every target had an explicit input bound and at least an empty, malformed, and
representative seed where that input shape permitted it.

| Target | Durable property | Bound and seed diversity | Review result |
| --- | --- | --- | --- |
| `configrepo.FuzzFleetArtifact` | A valid fleet returns the exact artifact and its SHA-256; invalid names cannot resolve an artifact. | Fleet ≤128 bytes, content ≤64 KiB; valid YAML and malformed content seeds. | Strengthened in this audit. |
| `configrepo.FuzzFleetArtifactPathTraversal` | Empty, separator-bearing, and traversal names are rejected. | Fleet ≤256 bytes; `..`, `../escape`, and nested traversal seeds. | Retained. |
| `identity.FuzzEndpointIDFromCert` | A parseable permitted endpoint URI round-trips its identifier; nil certificates are safe. | ID ≤512 bytes; UUID, empty, and traversal seeds; reserved URI characters excluded before construction. | Retained. |
| `identity.FuzzFingerprintFromCertRoundTrip` | Fingerprint equals the SHA-256 hex encoding of certificate raw bytes. | Raw bytes ≤64 KiB; empty, text, and high-bit byte seeds. | Replaced per-input RSA generation with a deterministic bounded property. |
| `pki.FuzzIssueEndpointCredential` | A successfully issued credential preserves the endpoint ID when read from its certificate. | ID ≤256 bytes; UUID, empty, and non-UUID string seeds. | Retained. |
| `server.FuzzHandleSync` | Authenticated Sync always writes an HTTP status for bounded JSON input. | Body ≤64 KiB; valid JSON, malformed JSON, and empty seeds. | Retained. |
| `server.FuzzHandleEnroll` | Enrollment always writes an HTTP status for bounded request input. | Body ≤64 KiB; valid token, empty token, and non-JSON seeds. | Retained. |
| `postgres.FuzzParseEndpointID` | Any accepted ID is canonical under a second parse. | ID ≤512 bytes; UUID, legacy slug, empty, malformed, and zero UUID seeds. | Retained. |
| `models.FuzzParseState` | Any accepted state has stable YAML canonical output after a parse/marshal/parse cycle. | Payload ≤1 MiB; representative YAML, malformed YAML, and empty seeds. | Strengthened in this audit. |

The original discovery command reported all nine functions (including the
separate `configrepo` targets), rather than counting source files. Subsequent
target additions are covered by current source discovery and focused change
evidence rather than retroactively rewriting that baseline table.
