# State-report precedence evidence

## Red

Public seam: Postgres-backed State-report projection (`OS-AEC-118`).

Command:

`go test ./internal/store/postgres -run 'TestEndpointStateReportNewerComplianceSupersedesHistoricalApplyFailure$' -count=1`

Observed intended failure before implementation:

`current compliance = "apply_failed"/true, want compliant/true`

The authenticated Admin API regression then reproduced the same contradiction through `TestGetEndpointStateReportNewerComplianceDoesNotExposeHistoricalFailureAsCurrent`.

A boundary case then proved that a newer failure with no Release correlation was also incorrectly treated as current: `status/failure = "apply_failed"/true, want "compliant"/false`.

## Green

Focused Postgres cases now prove that a newer same-Release compliant report, a different-Release failure, and an uncorrelatable missing-Release failure do not expose a current apply failure, while a newer same-Release failure remains `apply_failed`. The authenticated Admin API returns the same current classification, and endpoint detail retains the historical failure.
