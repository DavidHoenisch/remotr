# Native fuzz regression corpus

Every committed minimized corpus input has a durable reference, an owning
target, and a reproduction command. These corpus inputs run under the ordinary
seed-corpus target and must remain after the defect is repaired.

| Regression | Verification reference | Target and corpus input | Reproduction | Fixed behavior |
| --- | --- | --- | --- | --- |
| `FUZZ-2026-07-11-001` | `OS-TQG-011` | `internal/configrepo/testdata/fuzz/FuzzFleetArtifact/c4bc339a1557bf00` | `go test -mod=vendor ./internal/configrepo -run='FuzzFleetArtifact/c4bc339a1557bf00' -count=1` | The fuzzer now uses `ValidateFleetName`, so whitespace-only fleet names are rejected rather than treated as writable artifacts. |
| `FUZZ-2026-07-15-002` | `OS-TQG-011` | `internal/registry/testdata/fuzz/FuzzStateReportJSONRoundTrip/48509de87cdffbf6` | `go test -mod=vendor ./internal/registry -run='FuzzStateReportJSONRoundTrip/48509de87cdffbf6' -count=1` | The round-trip property now accounts for JSON's defined replacement of invalid UTF-8 while continuing to require valid bounded output and exact preservation of valid text. |
| `FUZZ-2026-07-15-003` | `OS-TQG-011` | `internal/registry/testdata/fuzz/FuzzStateReportJSONRoundTrip/716685d144e8f01b` | `go test -mod=vendor ./internal/registry -run='FuzzStateReportJSONRoundTrip/716685d144e8f01b' -count=1` | Consecutive invalid UTF-8 bytes remain a regression seed; serialization must produce valid UTF-8, while byte-for-byte equality remains required for valid source text. |
| `FUZZ-2026-07-18-004` | `OS-AEC-081` | `internal/rollbackstore/testdata/fuzz/FuzzRetentionCleanupPreservesArmedAndBoundsTerminalRecords/c1cf314d0c9c9fa6` | `go test -mod=vendor ./internal/rollbackstore -run='FuzzRetentionCleanupPreservesArmedAndBoundsTerminalRecords/c1cf314d0c9c9fa6' -count=1` | The retention oracle counts only successful non-secret prior payloads toward the three-state limit; the newest incomplete staged payload may coexist while attempt metadata and armed recovery remain independently bounded. |

When a campaign discovers another failure, commit its minimized input under
the target's `testdata/fuzz/<target>/` directory and add a row here before the
repair is considered complete.
