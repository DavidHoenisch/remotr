# Verification foundation operations

This reference is for Remotr contributors and release maintainers operating the testing and performance foundation. The default owner is [`@DavidHoenisch`](https://github.com/DavidHoenisch); a maintainer taking over a failed controlled-runner job records the handoff in its linked issue or pull request.

## Run the narrowest useful check

Use the focused command that matches the changed behavior before running the repository suite.

| Change area | Focused command |
| --- | --- |
| Ordinary Go behavior | `go test -mod=vendor ./path/to/package` |
| Repository deterministic suite | `make test` |
| Race behavior | `go test -mod=vendor -race ./...` |
| Seed corpus or active fuzzing | `make test-fuzz-seeds FUZZ_PACKAGES=./path/to/package` or `make fuzz-short FUZZ_PACKAGES=./path/to/package` |
| Provider containers | `make provider-matrix-containers` |
| VM safety fixture | `make provider-matrix-vm-system-safety` |
| Benchmarks | `./scripts/bench-collect.sh artifacts/benchmarks/local.txt` |
| Authenticated 400-endpoint reference load | `make load-steady-400` with the explicit disposable `REMOTR_LOAD_*` environment |

The [load harness guide](load-harness.md) documents the required disposable environment variables and controlled fault commands. Do not add a production database URL, production CA, or endpoint credentials to shell history, CI variables, benchmark artifacts, or issue comments.

## CI ownership and cadence

| Evidence | Workflow or command | Cadence | Triage owner |
| --- | --- | --- | --- |
| Changed-target and scheduled fuzzing | `Scheduled fuzz campaigns` | PR plus nightly/weekly | `@DavidHoenisch` |
| Deterministic tests, coverage, race, and OpenSpec lint | `Pull request quality gate` | Every PR | Change author, then `@DavidHoenisch` |
| Provider container matrix, Vagrant safety, reference load | `Nightly environment verification` | Nightly | `@DavidHoenisch` and the labelled-runner maintainer |
| 4,000-endpoint comparison | `Fleet load headroom` | Weekly | `@DavidHoenisch` |
| Benchmark comparison | `Benchmark comparisons` | PR advisory and controlled manual run | `@DavidHoenisch` |
| Mutation pilot | Documented Mewt campaign | Evidence-only until policy approval | `@DavidHoenisch` |

The `remotr-benchmark` and `remotr-vagrant` runners are controlled environments. A job waiting for one is runner work, not a reason to rerun an expensive workload on an arbitrary developer host.

## Triage a failure

1. Preserve the job URL, revision, command, runner label, and produced JSON or text artifact before retrying.
2. Reproduce a deterministic failure with the narrowest command. Keep a new fuzz crash input in the target seed corpus before changing the parser or test.
3. For provider or Vagrant failures, retain the fixture artifacts and identify the provider, image, and safety boundary. Do not relax a safety assertion to make a transient environment issue disappear.
4. For load failures, first inspect each named wave's errors, overload count, latency, bytes, start spread, and database deltas. Controlled fault targets must confirm their service was recovered before any retry.
5. File or update an issue when a result is not reproducible, is flaky, or needs runner repair. Include a narrow rerun command and expiry if any gate is temporarily quarantined.

## Change baselines deliberately

Benchmark, load, mutation, and coverage observations are evidence, not values to overwrite after a regression. A baseline or release-blocking budget change requires:

1. A controlled-runner result with the complete command, revision, Go version, and runner metadata retained.
2. An explanation of the changed workload or intentional regression.
3. Review by `@DavidHoenisch` and an OpenSpec update when a requirement or release budget changes.

The 4,000-endpoint job remains headroom evidence until a separate approved SLO states otherwise. Shared-runner latency remains advisory.

## Manage exceptions safely

Every manual, not-applicable, equivalent-mutant, or quarantined item is a record in `test/evidence-exceptions.yaml`. It needs an owner, issue, reason, and expiry; expired records are invalid. Renew by adding a reviewed replacement rationale rather than silently moving an expiry date. An exception never makes an unimplemented public behavior verified.

## Operate controlled environments

Load commands refuse to run without `--allow-load`. Commands that pause a Compose service also require `--allow-faults` and are limited to the declared disposable Compose file and service. The harness restores paused services and temporary release refs, but the operator still verifies health after an interrupted command.

VM validation uses the Vagrant targets in the repository; do not substitute a Docker container for a VM safety result. Teardown a completed disposable stack with `make compose-down` and a completed Vagrant fixture with `make provider-matrix-vm-destroy`.
