# Coverage reporting policy

Coverage is a navigation and regression signal, not a substitute for
behavioral, provider, safety, fuzz, mutation, or performance evidence. Remotr
publishes package and changed-line coverage as pull-request artifacts; it does
not impose a repository-wide percentage gate.

## Explicit exclusions

The report excludes `internal/store/postgres/db/**`. These files are sqlc
generated query and model bindings, as identified by their generated-file
headers. Their behavior is exercised through the hand-written Postgres store
package and integration tests, but line coverage for generated bindings would
distort coverage trends and encourage low-value tests.

No other production Go path is excluded. Adding an exclusion requires updating
both this document and `scripts/coverage-report.go` in the same review.

## Artifacts

The pull-request workflow retains:

- the raw Go coverprofile;
- the Go per-function summary;
- the generated package summary; and
- changed-line coverage against the pull request base revision when available.

Changed-line coverage counts only changed lines that intersect an instrumented
statement. Comments, declarations without executable code, and generated
sources are not denominators. The result is intentionally reported without a
blanket threshold; future risk-based floors belong to the individual critical
packages after their first complete verification slice.
