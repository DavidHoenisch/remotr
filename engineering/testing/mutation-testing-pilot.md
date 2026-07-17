# Mewt mutation-testing pilot

## Status and decision boundary

This is a pinned, local pilot. It collects evidence for a later adoption
decision; it is not a pull-request or release gate yet. The pilot is owned by
[@DavidHoenisch](https://github.com/DavidHoenisch) until that decision is
recorded.

The committed configuration is `mewt.toml`. Its SQLite
database is local-only, ignored by Git, and can be removed with `mewt purge`
or by deleting `test/mutation/mewt.sqlite*`.

## Pinned tool and verification

The pilot uses [Mewt v3.0.1](https://github.com/trailofbits/mewt/releases/tag/v3.0.1),
the upstream 2026-03-24 release. The source is the
[trailofbits/mewt v3.0.1 tag](https://github.com/trailofbits/mewt/tree/v3.0.1).
For Linux x86-64, download only this release artifact:

- `mewt-x86_64-unknown-linux-gnu.tar.xz`
- SHA-256: `4e4b589b1744bc30b2cbd9ca21f8e2ab2527bce56dbea856621ed804451f4703`

Use a temporary directory so a contributor or CI job does not need a
developer-global installation:

```sh
set -eu
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
cd "$workdir"
asset=mewt-x86_64-unknown-linux-gnu.tar.xz
base=https://github.com/trailofbits/mewt/releases/download/v3.0.1
curl --fail --location --output "$asset" "$base/$asset"
printf '%s *%s\n' \
  4e4b589b1744bc30b2cbd9ca21f8e2ab2527bce56dbea856621ed804451f4703 \
  "$asset" | sha256sum --check --status
tar -xf "$asset"
export MEWT="$workdir/mewt-x86_64-unknown-linux-gnu/mewt"
"$MEWT" --version # must report: mewt 3.0.1
```

The release also publishes a matching
[checksum file](https://github.com/trailofbits/mewt/releases/download/v3.0.1/mewt-x86_64-unknown-linux-gnu.tar.xz.sha256).
The explicit digest above protects the verification step from treating a
downloaded checksum as its own trust root.

## License review

Mewt is licensed under
[GNU AGPL-3.0](https://github.com/trailofbits/mewt/blob/v3.0.1/LICENSE).
This pilot runs the unmodified executable only as an isolated development or
CI test tool. Remotr does not vendor, link to, distribute, or ship it with a
Remotr binary or image. Adoption as a mandatory hosted CI or release tool is
pending the repository's license-policy review and is task 8.6's explicit
decision; no such adoption is implied by this pilot.

## Current scope

The target list deliberately covers implemented critical behavior only:

| Source target | Current behavior exercised | Fast test command |
| --- | --- | --- |
| `internal/configrepo/repo.go` | Endpoint override versus fleet artifact selection and path validation | `go test -mod=vendor ./internal/configrepo ./internal/scaffold` |
| `internal/capabilitymatrix/matrix.go` | Static and runtime provider capability selection | `go test -mod=vendor ./internal/capabilitymatrix ./internal/configrepo` |
| `internal/changecontrol/registry.go`, `lease.go` | Authorization grouping and endpoint execution leases | `go test -mod=vendor ./internal/changecontrol ./internal/server` |
| `internal/rbac/rbac.go` | Authorization rule grouping and path/method matching | `go test -mod=vendor ./internal/rbac` |
| `internal/agent/engine/engine.go` | Dependency graph ordering and activation-related engine construction | `go test -mod=vendor ./internal/agent/engine` |
| `internal/executor/activation.go` | Activation ordering and deduplication | `go test -mod=vendor ./internal/executor ./internal/agent/engine` |
| `internal/rollbackstore/store.go` | Encrypted rollback reservation, retention, pruning, and cleanup | `go test -mod=vendor ./internal/rollbackstore ./internal/agent/networkstate` |
| `internal/apppackages/manifest.go` | Manifest schema-version compatibility and validation | `go test -mod=vendor ./internal/apppackages` |
| `internal/secrets/envelope.go`, `registry.go`, `secret.go` | Secret envelopes, version lifecycle, rollback references, redaction-safe file references | `go test -mod=vendor ./internal/secrets ./internal/server` |

The expanded scope follows the landed public execution-lease, rollback, and
versioned-secret behavior. It does not add synthetic production concepts for
the sake of mutation coverage.

### Expanded mutant generation — 2026-07-15

Mewt 3.0.1 generated 2,347 current mutants across the seven newly eligible
targets. Generation used the checked-in mutator set and did not classify any
result as caught, surviving, or equivalent before tests ran.

| Target | High | Medium | Low | Total generated |
| --- | ---: | ---: | ---: | ---: |
| `internal/capabilitymatrix/matrix.go` | 46 | 98 | 206 | 350 |
| `internal/changecontrol/registry.go` | 47 | 111 | 140 | 298 |
| `internal/changecontrol/lease.go` | 16 | 38 | 63 | 117 |
| `internal/executor/activation.go` | 14 | 18 | 10 | 42 |
| `internal/rollbackstore/store.go` | 52 | 142 | 225 | 419 |
| `internal/secrets/envelope.go` | 54 | 130 | 271 | 455 |
| `internal/secrets/registry.go` | 67 | 191 | 408 | 666 |

### Focused high-severity campaign — 2026-07-15

The newly implemented critical targets were each run with their configured
cross-package test command. Mewt's high-severity error-replacement campaign
now reports no uncaught mutant in this expanded scope:

| Target | High tested | Caught | Uncaught |
| --- | ---: | ---: | ---: |
| `internal/capabilitymatrix/matrix.go` | 46 | 46 | 0 |
| `internal/changecontrol/registry.go` | 47 | 47 | 0 |
| `internal/changecontrol/lease.go` | 16 | 16 | 0 |
| `internal/executor/activation.go` | 14 | 14 | 0 |
| `internal/rollbackstore/store.go` | 59 | 59 | 0 |
| `internal/secrets/envelope.go` | 54 | 54 | 0 |
| `internal/secrets/registry.go` | 67 | 67 | 0 |

The first run exposed gaps in capability-provider branching, mixed-risk
authorization grouping, successful lease completion/window enforcement, and
unknown activation handling. Behavior-level tests at the public matrix,
registry, lease, and agent-execution seams killed those survivors. The
rollback campaign was regenerated after its cleanup fix; all 59 current
high-severity mutants are caught. The production tree was checked after every
campaign because interrupted mutation runs can leave a temporary edit in
place.

This closes the repeat-campaign prerequisite for the new execution-lease,
rollback-retention, and versioned-secret models. It does not reverse the pilot
decision below: medium/low mutants in the expanded scope and 127 historical
survivors remain unreviewed, and the AGPL CI policy decision remains open.

## Commands and timeouts

The checked-in configuration creates local SQLite state on first use; do not
run `mewt init`, which is intended for repositories without `mewt.toml`.
Generate or run a focused campaign from the repository root:

```sh
"$MEWT" mutate internal/rbac/rbac.go
"$MEWT" run internal/rbac/rbac.go
"$MEWT" results --target internal/rbac/rbac.go
```

`mewt.toml` assigns a 30-second timeout to each focused package test and a
90-second catch-all Go rule for the full suite. The catch-all is deliberately
a final per-target rule: Mewt 3.0.1 resolves a global `[test].cmd` before
per-target rules, so a global fallback would silently shadow the focused
commands. A timeout is a failed or inconclusive mutant outcome, never a retry
or a kill. The first campaign uses the branch, comparison, error, and logical
mutators listed in the configuration; broad operator-shuffle mutants are
deferred until the pilot measures their signal and cost. Campaigns use Mewt's
default severity short-circuiting. A reviewed full campaign uses
`--comprehensive`.

Each campaign records the Mewt version, target, test command, timeout, baseline
runtime, mutant count, result, and any cross-package rerun. Do not turn a
mutation score into functional evidence: surviving relevant mutants require a
test, an implementation correction, or reviewed equivalent-mutant metadata.

## Observed pilot evidence

The first focused run was executed on 2026-07-11 with Mewt 3.0.1 and the
committed configuration:

| Target | Generated | Caught | Survived | Timed out | Skipped | Focused-baseline time |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `internal/configrepo/repo.go` | 136 | 52 | 11 | 0 | 73 | 105 ms |
| `internal/rbac/rbac.go` | 112 | 54 | 28 | 0 | 30 | 43 ms |
| `internal/agent/engine/engine.go` | 223 | 115 | 35 | 1 | 72 | 91 ms |
| `internal/apppackages/manifest.go` | 209 | 71 | 43 | 0 | 95 | 85 ms |
| `internal/secrets/secret.go` | 41 | 21 | 10 | 0 | 10 | 44 ms |

These are cache-warm focused baseline measurements, using exactly the commands
in `mewt.toml`; they are a pilot cost measurement, not a performance budget.
The initial RBAC campaign completed in 11 seconds, artifact selection in 18
seconds, and secret-reference handling in 6 seconds. The database also retains
per-mutant test durations: engine 89.7 seconds, manifest 149.7 seconds,
artifact selection 28.4 seconds, RBAC 69.8 seconds, and secrets 9.3 seconds
across their recorded focused and fallback executions. The per-target duration
sum is the campaign-duration measure for this pilot because it excludes tool
download and makes focused and comprehensive reruns separately auditable.

The selected mutators produced 312 catches, 127 current survivors, one timeout,
and 280 short-circuited outcomes. Error replacement produced 97 catches and
18 survivors; conditional `IF` mutations produced 25 catches and 38 survivors;
comparison mutations produced 67 catches, 17 survivors, and the only timeout.
This is evidence that the chosen high- and medium-signal mutators are relevant,
not a score target. The confirmed equivalent-mutant rate is currently 0 of 127
(0%): all current survivors are intentionally *untriaged*, not silently
classified as equivalent.

Across all five targets, 128 focused-suite survivors were rerun against
`go test -mod=vendor ./...`. One cross-package kill was found: Mewt mutant
`563` removes the invalid-character loop in `ValidateFleetName`; it survives
`internal/configrepo` alone but is killed by
`internal/scaffold.TestInit_rejectsInvalidFleet`. The artifact-selection rule
therefore runs `internal/configrepo` and `internal/scaffold` together. A
direct rerun of mutant `563` with that command completed in one second and
killed it. The other 127 focused-suite survivors remained survivors under the
comprehensive fallback.

## Survivor and baseline metadata

`test/mutation/survivor-baseline.json`
is the versioned disposition format. A record identifies a mutant by Mewt
version, target path and SHA-256, mutation slug, byte offset, and hashes of the
old and new text. The Mewt database ID is retained only as a reproduction
shortcut; it is not the durable identity because a regenerated database can
renumber IDs.

Every survivor baseline entry has one of these dispositions:

- `untriaged`: no acceptance; blocks a mutation gate decision.
- `test-gap`: linked test or implementation work, owner, and expiry required.
- `equivalent`: invariant-based explanation, independent reviewer, and review
  date required.
- `intentional`: governing specification decision, owner, and expiry required.
- `tooling-failure`: reproducible tool issue, owner, and expiry required.

An `equivalent`, `intentional`, or `tooling-failure` record without its required
review metadata is invalid and must not be counted as accepted. The initial
record demonstrates a reproducible but untriaged survivor: Mewt 3.0.1 mutant
113 (`ER`, `WithSyncURL`) is rerun with `$MEWT test --ids 113` and is expected
to remain `Uncaught`. The remaining 126 survivors must be imported into this
format and reviewed before mutation testing can become a gate.

## Pilot decision

**Decision (2026-07-11): do not adopt Mewt as a merge, release, nightly, or
weekly gate yet.** The tool is retained as reproducible, on-demand evidence
for the five currently implemented targets. This is not a rejection of
mutation testing itself: it is a bounded pilot result.

Adoption requires all of the following:

1. A repository license-policy decision for isolated AGPL-3.0 CI use.
2. Import and review of all 127 surviving mutants using the committed metadata
   format, with no unexplained relevant survivor for newly added critical logic.
3. Resolution or reviewed disposition of the observed engine timeout.
4. A repeat campaign on newly implemented execution-lease, rollback-retention,
   and versioned-secret behavior once those public models exist. This was
   completed for current high-severity mutants on 2026-07-15; broader survivor
   review is still required by item 2.

Until then, no Mewt download or invocation is required by CI, releases, or a
developer's global environment. Focused and comprehensive commands remain
available for a contributor to attach mutation evidence to a critical slice.
