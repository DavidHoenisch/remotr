# TDD evidence log

## 2.1 — OS-AEC-088 canonical digest mismatch

- Public seam: capability-document model and validation.
- Red: `go test -mod=vendor ./internal/capabilitydoc -run '^TestCanonicalDigestMismatchIsRejected$' -count=1` failed to compile because `Document`, `Capability`, `Fact`, and `ErrDigestMismatch` did not exist.
- Green: the same focused selector passed after adding the versioned document model, deterministic set ordering, compact canonical JSON, SHA-256 digesting, and mismatch rejection.
