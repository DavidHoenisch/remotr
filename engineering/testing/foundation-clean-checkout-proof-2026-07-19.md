# Foundation clean-checkout proof — 2026-07-19

## Decision

The `establish-testing-and-performance-foundation` candidate is accepted from
a clean checkout. No required gate depends on the developer worktree, a global
Mewt installation, production credentials, or network-fetched Go modules.

The synthetic candidate was built from source revision
`9f209b5b92ebfb9619c4cad112f457245a9d696d`, the complete candidate patch, and
the candidate's intentional new files. Disposable Go caches and
`compose/runtime` state were excluded. It was committed in a fresh Git
repository as tree `eab3265f9fdcbebc3ba34b55a15557f6210c7397`;
`git status --short` was empty before and after verification.

## Offline verification

All Go verification set `GOPROXY=off`, used the repository vendor directory,
and ran through the project's configured Go 1.26.5 toolchain. The clean tree
passed:

- `make test`: all 45 discovered fuzz seed corpora and every Go package;
- `go vet -mod=vendor ./...`;
- test-policy and traceability linters;
- the zero-survivor mutation-baseline and performance-budget policy linters;
- YAML parsing for all 12 GitHub Actions workflows;
- `docker compose -f compose/docker-compose.yml config --quiet`;
- strict validation of this change and all seven active OpenSpec changes.

The verification commands left the checkout unchanged. The only failed probe
was the initial invocation before entering the configured toolchain, which
reported `go: command not found`; rerunning the identical offline command via
`mise exec` passed and did not alter the candidate.

## Expensive evidence boundaries

The final mutation campaign ran in a separate clean Git checkout with the same
checksum-verified Mewt 3.0.1 binary pinned by the repository installer. Its 29
adopted critical targets caught 1,207/1,207 high-severity mutants, with no
survivor, timeout, skip, accepted equivalent, or evidence exception. The
versioned survivor baseline is empty for current adopted scope.

Controlled native and Postgres benchmarks, authenticated 400-endpoint load,
agent resource-growth probes, the soak tracer, and bounded redacted profile
capture used the disposable local Compose environment. The final performance
gate passed every absolute and relative budget. No production endpoint,
credential, or datastore was used; the disposable stack was torn down after
collection and its runtime state is not part of the candidate.

The evidence-only acceptance notes added after this run do not change code,
workflow behavior, schemas, budgets, or the verified OpenSpec requirements.
