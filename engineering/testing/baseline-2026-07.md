# Testing and performance baseline — 2026-07-11

This report captures the repository state before the testing and performance
foundation is introduced. It is an observation, not a quality threshold.

## Environment and commands

- Revision: `54bb3f6` on `master`
- Host: Linux 7.0.10-arch1-1 x86_64
- Go: `go1.26.3 linux/amd64`
- Ordinary coverage command: `go test -count=1 -mod=vendor -coverpkg=./... -coverprofile=coverage.txt ./...`
- Race command: `go test -count=1 -mod=vendor -race ./...`

The commands were run with the Go test cache cleared. The coverage command
completed successfully in 5.24 seconds and reported 38.8% statement coverage
across repository packages. The race command completed successfully in 10.89
seconds. These timings are local reference observations only; they are not CI
budgets.

## Inventory

| Item | Observed count |
| --- | ---: |
| Go test files | 122 |
| Test functions | 380 |
| Native fuzz targets | 9 |
| Benchmark functions | 0 |
| E2E test functions | 7 |
| Godog feature files | 0 |

The E2E package is `test/e2e` and exercises health, Debian/Arch Sync, gzip
responses, bootstrap/operator administration, app package listing, and agent
enrollment. At this baseline it has no recorded clean-stack result: its CLI
helpers place global flags after subcommands, which is known to make the
operator flow fail. Task 1.6 repairs that harness before treating an E2E run as
baseline evidence.

## Existing CI cadence

- `.github/workflows/release.yml` runs ordinary Go tests only for version-tag
  releases.
- `.github/workflows/config-repo.yml` validates selected configuration changes
  on pushes and pull requests.
- There is no repository-wide pull-request workflow for formatting, vetting,
  ordinary tests, coverage, race detection, or OpenSpec validation.
- No scheduled fuzz, benchmark, load, soak, mutation, provider-matrix, or
  acceptance workflow exists.

## Known gaps and visible skips

- `scripts/fuzz-all.sh` references the removed
  `internal/store/postgres:FuzzUUIDFromString` target; the native target is
  `FuzzParseEndpointID`. Go reports a no-match fuzz expression as a successful
  warning, so the script can appear green while omitting a target.
- E2E tests can skip when a bootstrap token is unavailable or the stack/enroll
  endpoint is not ready. The clean-stack command must make those prerequisites
  available rather than silently treating the skips as proof.
- Conditional fixture/environment skips exist in the AI setup, hub catalog,
  operator configuration, and CLI upgrade tests. They are listed by the
  repository check introduced in task 3.5; this report does not classify them
  as approved quarantines.
- Generated database code under `internal/store/postgres/db` and generated
  runtime entrypoints are currently included in the broad package inventory.
  Task 1.8 documents the explicit coverage-exclusion policy before any
  coverage ratchet is applied.

## Interpretation

The fast deterministic unit and race suites are affordable as pull-request
gates on this host. Coverage is intentionally reported by package and as a
whole-repository observation, not as a blanket pass/fail percentage. Provider,
integration, and performance evidence remain to be added by later tasks.
