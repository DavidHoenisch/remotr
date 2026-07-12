# Benchmark fixtures

Native benchmarks use the deterministic generator in
[`test/benchmarkfixture`](../../test/benchmarkfixture). Its current shape is
`GeneratorVersion = "v1"` and it produces one valid desired-state
configuration containing independent package resources.

The pinned resource-count inputs are 10, 100, 500, and 1,000. `Artifact(size)`
is byte-for-byte deterministic; its test independently parses every generated
artifact and checks the configuration and resource count. Benchmarks consume
these known-valid inputs and report cost only. They must not derive expected
behavior, validation outcomes, or fixture shape from benchmark measurements.

When a fixture shape changes, increment `GeneratorVersion`, update this page,
and treat prior benchmark series as a different fixture family rather than a
silent comparison baseline.

## Repeated collection and comparison

`scripts/bench-collect.sh test/benchmarks/results/current.txt` runs each
selected native benchmark ten times by default; set `BENCH_COUNT` or
`BENCH_PATTERN` only when documenting why a focused collection is sufficient.
`scripts/bench-compare.sh before.txt after.txt out/` writes separate
`latency.txt`, `allocations.txt`, `payload.txt`, and `storage.txt` comparisons.

The comparison wrapper pins `golang.org/x/perf/cmd/benchstat` at
`v0.0.0-20260709024250-82a0b07e230d` and runs it with a temporary writable Go
module cache. A controlled runner may instead provide `BENCHSTAT_BIN`; no
developer-global installation is consulted implicitly. Native benchmarks emit
standard latency and allocation units. Sync and state-report benchmarks also
emit `payload_bytes`; Postgres storage benchmarks will emit `storage_bytes`
when the controlled database harness lands.
