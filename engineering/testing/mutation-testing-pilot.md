# Mewt mutation-testing pilot

## Status and decision boundary

Mewt 3.0.1 was adopted on 2026-07-18 for an isolated high-severity critical
logic gate and a weekly comprehensive evidence campaign. New unexplained high
or otherwise security-relevant survivors block completion. Mutation score is
not a substitute for functional, provider, safety, or performance evidence.
The policy is owned by [@DavidHoenisch](https://github.com/DavidHoenisch).

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
The repository permits the verified, unmodified executable only as an isolated
development or CI test process. Remotr does not vendor, link to, modify,
distribute, or ship it with a Remotr binary or image, and does not expose it as
a network service. Changing that operational boundary requires a new license
review. CI downloads the pinned release into ephemeral runner storage, checks
the digest above, and deletes it with the runner workspace.

## Current scope

The target list deliberately covers implemented critical behavior only:

| Source target | Current behavior exercised | Fast test command |
| --- | --- | --- |
| `internal/configrepo/repo.go` | Endpoint override versus fleet artifact selection and path validation | `go test -mod=vendor ./internal/configrepo ./internal/scaffold` |
| `internal/capabilitymatrix/matrix.go` | Static and runtime provider capability selection | `go test -mod=vendor ./internal/capabilitymatrix ./internal/configrepo` |
| `internal/changecontrol/registry.go`, `lease.go` | Authorization grouping and endpoint execution leases | `go test -mod=vendor ./internal/changecontrol ./internal/server` |
| `internal/changecontrol/breakglass.go` | Canonical request binding and non-bypassable hash, dependency, redaction, preflight, and rollback-reservation safeguards | `go test -mod=vendor ./internal/changecontrol ./internal/server` |
| `internal/rbac/rbac.go` | Authorization rule grouping and path/method matching | `go test -mod=vendor ./internal/rbac` |
| `internal/agent/engine/engine.go` | Dependency graph ordering and activation-related engine construction | `go test -mod=vendor ./internal/agent/engine` |
| `internal/executor/activation.go` | Activation ordering and deduplication | `go test -mod=vendor ./internal/executor ./internal/agent/engine` |
| `internal/executor/safe_value.go` | Closed sensitivity/projection matrix, safe provider-error conversion, and JSON admission | `go test -mod=vendor ./internal/executor ./internal/resourceregistry` |
| `internal/effectivehash/hash.go` | Versioned canonicalization, unordered structures, defaults, provider revision, and safe secret identity | `go test -mod=vendor ./internal/effectivehash ./internal/resourceregistry ./internal/configcompose ./internal/changecontrol ./internal/server` |
| `internal/resourceregistry/fields.go` | Strict-schema leaf discovery and complete sensitivity/projection admission | `go test -mod=vendor ./internal/resourceregistry ./internal/configcompose` |
| `internal/resourceregistry/safe_projection.go` | Registered-resource projection across nested, sequence, wildcard, count, and presence fields | `go test -mod=vendor ./internal/resourceregistry ./internal/executor` |
| `internal/rollbackstore/reservation.go` | Complete encrypted-envelope capacity reservation, overcommit prevention, and single-use Arm ownership | `go test -mod=vendor ./internal/rollbackstore ./internal/agent/networkstate` |
| `internal/rollbackstore/store.go` | Encrypted rollback reservation, retention, pruning, and cleanup | `go test -mod=vendor ./internal/rollbackstore ./internal/agent/networkstate` |
| `internal/rollbackstore/retention.go` | Classified rollback metadata plus deterministic retention boundaries | `go test -mod=vendor ./internal/rollbackstore` |
| `internal/apppackages/manifest.go` | Manifest schema-version compatibility and validation | `go test -mod=vendor ./internal/apppackages` |
| `internal/diagnostics/bundle.go` | Closed diagnostic archive, manifest, and classified source-summary admission | `go test -mod=vendor ./internal/diagnostics ./internal/agent/diagnostics` |
| `internal/server/diagnostics.go` | Authenticated diagnostic-result lookup, classified object admission, rejection, and cleanup | `go test -mod=vendor ./internal/server` |
| `internal/store/postgres/diagnostics.go` | Classified failure validation before diagnostic persistence and after durable read | `go test -mod=vendor ./internal/store/postgres` |
| `internal/secrets/envelope.go`, `registry.go`, `secret.go` | Secret envelopes, version lifecycle, rollback references, redaction-safe file references | `go test -mod=vendor ./internal/secrets ./internal/server` |
| `internal/secrets/lifecycle.go` | Classified restored-key coverage and provider-error conversion | `go test -mod=vendor ./internal/secrets ./internal/store/postgres ./cmd/remotr-server` |

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

This closed the repeat-campaign prerequisite for the new execution-lease,
rollback-retention, and versioned-secret models. The later adoption decision
uses current regenerated high-severity identities as the blocking class;
medium/low outcomes and the 2026-07-11 database remain historical evidence.

### Classified projection and sink-admission campaign — 2026-07-18

OpenSpec task 3.6 ran the pinned Mewt 3.0.1 artifact against the complete
classification path: the closed safe-value model, registered-resource
projection, diagnostic archive admission, Postgres diagnostic persistence,
restored-key coverage, rollback metadata, and server-side object admission.
The Linux artifact digest matched the pinned SHA-256 above. No package manager,
developer-global installation, CI rule, or release gate was changed.

The campaign used the configured default severity short-circuiting. After the
initial runs exposed test gaps, focused behavioral tests killed every mutation
that could admit an unclassified value, omit required classified metadata,
retain provider error text, mark an unvalidated diagnostic object ready, or
skip rejected-object cleanup. Previously skipped server sink mutants were then
run directly with the focused `TestPersistDiagnosticResult` selector.

| Target | Current mutants | Caught | Uncaught | Skipped | Timeout | Unexplained redaction bypass |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `internal/executor/safe_value.go` | 376 | 155 | 5 | 216 | 0 | 0 |
| `internal/resourceregistry/safe_projection.go` | 151 | 82 | 8 | 61 | 0 | 0 |
| `internal/diagnostics/bundle.go` | 318 | 132 | 5 | 181 | 0 | 0 |
| `internal/store/postgres/diagnostics.go` | 284 | 45 | 54 | 185 | 0 | 0 |
| `internal/secrets/lifecycle.go` | 181 | 74 | 16 | 91 | 0 | 0 |
| `internal/rollbackstore/retention.go` | 437 | 212 | 63 | 161 | 1 | 0 |
| `internal/server/diagnostics.go` | 250 | 119 | 24 | 107 | 0 | 0 |
| **Total** | **1,997** | **819** | **175** | **1,002** | **1** | **0** |

The raw `Uncaught` count is deliberately not presented as accepted equivalent
metadata or as a mutation score. The task-scope survivors were inspected
individually:

- safe-value IDs 3694, 3563, 3604, 3577, and 3614 change behavior only for an
  equality case excluded by the enclosing non-equality branch, or remove a
  fast path whose fallthrough returns the same empty/valid value;
- projection IDs 3935, 3949, 3918, 3937, 4000, 3923, 3942, and 4036 are
  constrained by the successful marshal/unmarshal AST shape, the even mapping
  node invariant, registered struct-root schemas, fail-closed projector error
  return, or the identity of non-empty string comparison;
- bundle IDs 4244, 4109, 4342, 4344, and 4349 remain fail-closed through the
  subsequent exact-file loop, four-field shape check, and `SafeSummary`
  sensitivity/projection validator;
- persistence IDs 4459, 4460, and 4500 are revalidated by `SafeError.MarshalJSON`
  or discard an empty/invalid legacy failure on read; lifecycle IDs 4685 and
  4947 are likewise revalidated by the classified JSON sink;
- server IDs 5332, 5357, 5488, 5335, 5360, and 5508 alter only whether an
  already classified cleanup or persistence outcome is logged; they cannot
  change bundle admission, persisted status, digest, size, failure, or cleanup.

Those survivors and the other non-redaction lifecycle mutants remain
unaccepted comprehensive evidence; they are not added to the reviewed survivor
baseline and do not bypass the adopted high/relevant gate. The one timeout, rollback-retention ID
5038, changes cleanup-loop progress and is outside the classified serialization
invariant. It also remains unresolved pilot backlog rather than evidence for
this task.

### Applicator execution-contract high-severity campaign — 2026-07-18

OpenSpec task 5.5 ran every current high-severity mutant for rollback
reservation and retention, schema classification, canonical effective hashing,
derived-plan dependency closure, and break-glass bypass policy. The pinned Mewt
3.0.1 Linux artifact matched the SHA-256 above. Each target used its checked-in
focused cross-package command, and each baseline passed both before and after
the campaign.

| Target | Current high IDs | Selected | Caught | Uncaught | Timeout | Skipped |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| `internal/rollbackstore/reservation.go` | 8270–8317 | 48 | 48 | 0 | 0 | 0 |
| `internal/rollbackstore/retention.go` | 4826–4885 | 60 | 60 | 0 | 0 | 0 |
| `internal/resourceregistry/fields.go` | 8005–8036 | 32 | 32 | 0 | 0 | 0 |
| `internal/effectivehash/hash.go` | 7127–7166 | 40 | 40 | 0 | 0 | 0 |
| `internal/changecontrol/registry.go` | 6542–6613 | 72 | 72 | 0 | 0 | 0 |
| `internal/changecontrol/breakglass.go` | 6174–6211 | 38 | 38 | 0 | 0 | 0 |
| **Total** |  | **290** | **290** | **0** | **0** | **0** |

The first outcome query exposed seven uncaught error-replacement mutants.
Focused public-seam tests covered malformed canonical values, unregistered
field projections, non-serializing nested schema types, exact break-glass
targets, and replacement-capacity accounting. Field classification no longer
duplicates its closed sensitivity check, and rollback replacement accounting
now uses one filesystem traversal rather than a second race-prone walk. After
regeneration, all 290 current high-severity mutants were caught. The campaign
left the production tree clean and requires no evidence exception.

The final baseline lint also found that the pilot's one imported engine
survivor referred to an obsolete target hash. Regeneration preserved the same
`WithSyncURL` error-replacement mutation as current ID 8649. A composed-agent
test now proves the sync URL reaches enforced firewall control-path preflight
with exact process-boundary argv; ID 8649 changed from `Uncaught` to
`TestFail`. The resolved entry was removed from the survivor baseline, leaving
the 126 source-obsolete pilot outcomes as historical counts only. They were not
imported as accepted dispositions; the adopted current baseline contains zero
survivor.

### Capability-compatible delivery high-severity campaign — 2026-07-18

OpenSpec task 7.3 ran the current high-severity mutants for bounded capability
document validation, strict artifact requirement validation, whole-variant
requirement satisfaction, exact legacy mapping, compatible variant resolution,
and the Sync paths that admit current evidence and maintain target, offered,
active, blocked, and telemetry-attribution state. The pinned Mewt 3.0.1 Linux
artifact matched the SHA-256 above. The six targets generated 2,402 mutants in
the configured pilot mutator set; the campaign selected all high-severity
mutants in the four focused files and the exact task-relevant line ranges in
the two larger server files.

| Target | Current mutants | Selected high IDs | Selected | Caught | Uncaught | Timeout |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| `internal/capabilitydoc/validation.go` | 322 | 9915–9942 | 28 | 28 | 0 | 0 |
| `internal/artifactrequirements/set.go` | 194 | 9598–9624 | 27 | 27 | 0 | 0 |
| `internal/artifactvariant/compatibility.go` | 123 | 9792–9806 | 15 | 15 | 0 | 0 |
| `internal/server/legacy_capability_profiles.go` | 26 | 10679–10684 | 6 | 6 | 0 | 0 |
| `internal/server/composition.go` | 442 | 10271–10278 | 8 | 8 | 0 | 0 |
| `internal/server/server.go` | 1,295 | 10746–10752, 10765–10766, 10771–10788, 10804–10835 | 59 | 59 | 0 | 0 |
| **Total** | **2,402** |  | **143** | **143** | **0** | **0** |

The measured cache-warm focused baselines were 0.31 seconds for capability
documents plus their Postgres consumer, 6.73 seconds for artifact requirements
plus selection/composition/server consumers on the first cold run, 0.15
seconds for artifact selection plus server, and 0.13 seconds for the server
package. Every configured timeout remains 30 seconds.

The first outcome review exposed three persisted-document mutants because the
validator command stopped at the package boundary. Adding its real Postgres
consumer killed IDs 9924, 9925, and 9927. Artifact-requirement ID 9610 exposed
a direct test gap in the strict canonical decoder; the new regression proves it
accepts the exact canonical body and rejects JSONB-normalized key ordering.
That rerun killed the survivor. All focused package baselines passed after the
campaign, the production tree was clean, and no task-relevant survivor,
timeout, skip, or evidence exception remains.

## Adopted critical-scope closeout — 2026-07-19

The final candidate was copied to an isolated clean Git worktree and regenerated
with the checksum-verified Mewt 3.0.1 executable. The adopted target manifest
contained 29 current critical source files and selected 1,207 high-severity
mutants. Every selected identity produced `TestFail`; no survivor, timeout,
skip, accepted equivalent, or evidence exception remained.

The final reruns were deliberately bounded per target after an interrupted
multi-target invocation. Machine-readable evidence covered the unchanged
first 22 targets plus regenerated current identities for composition (56),
server Sync/routing (149), Postgres diagnostic persistence (38), secret
envelopes (55), secret lifecycle (19), secret registry (73), and secret
references (7). These partitions sum to the same 29-file manifest and
1,207/1,207 blocking result that the PR and weekly scripts enforce from a fresh
database.

The closeout campaign found and corrected observable gaps rather than adding
dispositions: diagnostic upload authorization/status/signing failures,
firewall-audit telemetry, unsupported artifact types, repository fallback,
cron-store failure isolation, and diagnostic persistence validation,
corruption, transition, expiry, and cleanup behavior. Separating artifact
reading from artifact storage also removed three silent no-op write methods;
the current cardinality is therefore three lower than the immediately prior
candidate. The survivor baseline remains empty for current adopted scope.

## Commands and timeouts

The checked-in configuration creates local SQLite state on first use; do not
run `mewt init`, which is intended for repositories without `mewt.toml`.
Generate or run a focused campaign from the repository root:

```sh
"$MEWT" mutate internal/rbac/rbac.go
"$MEWT" run internal/rbac/rbac.go
"$MEWT" results --target internal/rbac/rbac.go
```

The capability-delivery selector regenerates its six versioned targets,
derives the reviewed high-severity and line-scoped IDs, requires the current
143-mutant cardinality, verifies every outcome is `TestFail`, and confirms the
source tree is restored:

```sh
MEWT="$MEWT" make mutation-capability-selection
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
imported `WithSyncURL` survivor was regenerated as current ID 8649 and killed
by a public composed-agent regression on 2026-07-18, so it is no longer present
in the baseline. The remaining 126 outcomes are tied to the 2026-07-11 source
snapshot and are retained as a historical pilot count only. They are not
accepted equivalents and cannot satisfy or bypass the current regenerated
gate. Any current survivor in the adopted relevance class requires a stable
entry and reviewed disposition, or the gate remains red.

## Adoption decision

**Decision (2026-07-18): adopt Mewt 3.0.1 under the isolated-tool boundary
above.** The blocking campaign regenerates every file in
`test/mutation/critical-targets.txt`, selects every current high-severity
mutant, and requires `TestFail` for each identity. CI writes a machine-readable
zero-survivor result. Changed critical logic receives the same focused gate;
weekly CI also runs `--comprehensive` and retains the complete campaign log.

High severity is the initial blocking relevance class justified by the pilot's
error/fail-closed path results. Medium and low outcomes are review evidence,
not accepted equivalents and not a score target. If inspection shows that a
lower-severity mutant changes authorization, secret handling, rollback,
selection, ordering, or validation behavior, it is relevant and blocks until
killed or given a reviewed durable disposition.

The prior engine timeout was source-obsolete and its regenerated high mutation
was killed. Repeat campaigns for execution leases, rollback retention,
versioned secrets, classified sinks, applicator safety, and capability delivery
all completed before adoption. The pinned installer means neither contributors
nor release binaries depend on developer-global tooling.
