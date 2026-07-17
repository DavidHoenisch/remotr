# Native fuzz target audit — 2026-07-11

This audit covers the nine repository-owned native `Fuzz*` targets discovered
by `scripts/fuzz-all.sh`. Vendor fuzz functions are excluded because they are
not Remotr verification evidence. Every target has an explicit input bound and
at least an empty, malformed, and representative seed where that input shape
permits it. Committed regression corpora are introduced by task 7.5.

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

The discovery command reports all nine functions (including the separate
`configrepo` targets), rather than counting source files. New
coverage for schema/version, capability documents, artifact selection,
authorization, secret handling, rollback metadata, and report bounds remains
tracked in tasks 7.2–7.4.
