## Context

The engineering fleet rollout exposed four independent implementations of “support” that currently disagree:

- the agent's default capability generator does not consume the checked-in Ubuntu Pro qualification manifest;
- composition aggregates requirements from Ubuntu/Debian and Arch branches into one unconditional set;
- local configuration validation and server Git-sync validation do not execute the same provider/target checks or return the same diagnostic;
- blocked-upgrade selection uses a hand-written synthetic version map and rejects an upgrade when the current endpoint lacks provider capabilities the upgrade is intended to discover.

The result is a release that validates and advances globally but is never offered to the endpoint. An older agent makes the state still less legible because it ignores the newer `capabilityBlocked` field and reports a missing artifact. The design must remain fail-closed: repairing delivery cannot turn a build version into proof of runtime support, manufacture qualification evidence, silently remove desired resources, or leak secrets in diagnostics.

## Goals / Non-Goals

**Goals:**

- Establish one frozen, release-owned source of truth for advertised provider and resource capabilities.
- Evaluate a mixed-platform release against only the requirements applicable to an endpoint's normalized target facts.
- Make local validation and server Git-sync validation reject the same unsupported target/provider combinations with actionable, safe diagnostics.
- Let an explicit, approved agent upgrade escape capability-blocked state while retaining the old active artifact until actual capabilities are reported and the new artifact is acknowledged.
- Qualify or truthfully reject the Ubuntu 26.04 amd64 rows exercised by the Ubuntu Pro engineering configuration.

**Non-Goals:**

- Global, fleet, or endpoint secret-scope changes; those remain in `add-global-secret-scope`.
- Treating an agent version, executable presence, distribution family, or successful composition as provider-conformance evidence.
- Server-side endpoint-specific removal of desired resources or fields.
- Live Canonical enrollment in automated qualification.
- Generalizing Ubuntu 26.04 evidence to another release, architecture, service tuple, or provider.

## Decisions

### 1. Build a frozen release capability catalog from checked-in evidence

The repository will define a validated capability-catalog source containing exact selectors and capability rows from passing provider qualification manifests. A deterministic generation step will produce a canonical embedded agent catalog and agent-release metadata from that source. The production default generator will evaluate the embedded catalog against normalized runtime facts; tests may inject an alternate catalog but production will not read fixtures from `test/` or fetch qualification data remotely.

Generation will fail for duplicate or conflicting rows, missing evidence selectors, unknown capability IDs, unsupported schema revisions, non-canonical output, or release metadata that does not match the embedded agent catalog. A release may publish only rows whose complete required selectors passed. The release pipeline will verify that regenerated outputs match checked-in or packaged outputs.

This replaces constructor-specific wiring and the synthetic server upgrade-profile map. Keeping handwritten maps was rejected because the maps already drifted from both qualification evidence and real published versions. Loading test manifests at runtime was rejected because deployment packages must be self-contained and immutable.

### 2. Project requirements by a bounded target predicate, not by endpoint identity

Composition will normalize each configuration's declared distribution, release, architecture, and provider target. It will build a bounded set of target variants. Each variant contains:

- a canonical target predicate derived only from authored target facts;
- the complete canonical desired artifact, without resource or field removal; and
- only the capability requirements of configurations whose target predicate can match that variant, plus portable requirements that apply to every variant.

The same artifact bytes and digest may back multiple requirement variants. On Sync, the server first matches a variant predicate against the current capability document's normalized facts and then evaluates that variant's requirements. It never assembles a bespoke endpoint artifact. Conflicting or under-specified target declarations that would make provider applicability ambiguous fail validation rather than becoming universal requirements.

This keeps endpoint-local target filtering as defense in depth while preventing an Ubuntu endpoint from being blocked by Pacman requirements authored solely for Arch. Aggregating every branch into one requirement set was rejected because it makes any heterogeneous fleet undeliverable. Removing unmatched resources on the server was rejected because it violates canonical artifact and no-partial-state guarantees.

Variant count and serialized requirement size remain bounded. A benchmark with a representative mixed-fleet fixture will guard the hot composition and Sync-selection paths.

### 3. Use one validation engine at the configuration CLI and Git-sync boundary

Target normalization, provider-catalog lookup, capability requirement derivation, ambiguity checks, and variant bounds will live behind one validation service used by both `remotr config validate` and server Git sync. The CLI validates every declared fleet target without needing a live endpoint. The server runs the same checks before advancing the Release ref.

Diagnostics will carry a stable class, source file/resource address, target tuple, provider/capability, and safe explanation. The Admin API returns a bounded safe rendering; server logs retain the correlated internal error. Secret values, authenticated URLs, environment contents, and raw desired-state fragments remain excluded.

A differential test will feed the same corpus through both public seams and require matching accept/reject classifications and diagnostic identities. Sharing only error strings was rejected because it would allow the underlying rules to continue drifting.

### 4. Separate upgrade authorization from runtime provider compatibility

An agent-upgrade instruction is authorized by explicit fleet or endpoint desired-version state, the approved release catalog, platform/architecture eligibility for the agent binary, and existing upgrade integrity controls. It is not an artifact variant and does not assert that the target version will provide any particular runtime provider capability.

Therefore, when an endpoint is capability-blocked, the server will still include an eligible requested upgrade instruction alongside the blocked result. Legacy agents that understand `agentUpgrade` but ignore `capabilityBlocked` can consume the same instruction. After restart, the upgraded agent must report a valid current capability document. The server then reevaluates the target variant; it may remain blocked, and it keeps the prior active artifact until exact-digest acknowledgment succeeds.

The release catalog may expose expected schema/protocol support for operator explanation, but expected provider rows cannot suppress or authorize the upgrade. Requiring a version profile to satisfy all missing provider requirements was rejected because it creates an in-band upgrade deadlock and confuses build metadata with runtime evidence.

### 5. Promote Ubuntu 26.04 rows only from selected evidence

The implementation will inventory every requirement used by the representative Ubuntu Pro engineering fixture. For each Ubuntu 26.04 amd64 row, it will either:

- run the required provider contract and real-environment evidence, then add the exact passing catalog row; or
- keep the row unadvertised and make shared validation reject the configuration with an actionable source-specific diagnostic.

Ubuntu Pro qualification continues to use a pinned disposable Ubuntu VM, deterministic `/usr/bin/pro api` doubles, synthetic token canaries, cleanup verification, and explicit disclosure that no live Canonical entitlement was exercised. Core system providers receive their governing container or VM evidence; passing Ubuntu Pro evidence does not promote them transitively.

## Risks / Trade-offs

- **Variant growth for many target tuples** → Canonicalize and deduplicate predicates, include portable requirements by reference, enforce hard count/byte bounds, and benchmark representative heterogeneous fleets.
- **Generated release artifacts can become stale** → Make generation deterministic and fail CI/release packaging when regeneration produces a diff or metadata disagrees with the agent payload.
- **An upgrade succeeds but the endpoint remains blocked** → Preserve the prior active artifact, report actual post-upgrade missing requirements, and never describe the upgrade as artifact activation.
- **Legacy agents vary in upgrade-response support** → Add compatibility fixtures for supported legacy response decoders and truthfully report versions that require an out-of-band bootstrap.
- **More accurate validation rejects repositories that previously synced** → Run shared validation before deployment, provide file/resource/target diagnostics, and do not advance the Release ref on failure.
- **Qualification could accidentally overstate Ubuntu 26.04 support** → Require exact selectors and independent evidence per capability row; default to unadvertised on any missing selector.

## Migration Plan

1. Introduce the shared catalog schema and deterministic generator without changing advertised rows; verify packaged output parity.
2. Add target-aware requirement variants and shared validation behind compatibility-preserving readers, then validate the representative mixed Ubuntu/Arch repository.
3. Collect Ubuntu 26.04 evidence and publish only the exact passing rows; ensure unsupported rows fail before release creation.
4. Ship release metadata and blocked-upgrade delivery together so existing approved upgrade requests can escape the deadlock.
5. Deploy the server first, then the newly cataloged agent. Confirm an older compatible agent receives its upgrade, the upgraded endpoint reports actual capabilities, the Ubuntu Pro artifact is offered and acknowledged, and the prior active digest is retained until acknowledgment.
6. Roll back by disabling the target Release ref or desired agent version and restoring the previous server/agent package. Generated catalogs are immutable per release, so rollback does not rewrite capability claims.

## Open Questions

- During implementation, the Ubuntu 26.04 inventory may expose a core provider whose governing safety evidence cannot run in the current CI environment. That row will remain unadvertised and the corresponding configuration will remain rejected until its required fixture is available; it will not be waived into the catalog.
- The exact legacy-agent floor for in-band upgrade response compatibility must be established from released decoder behavior and recorded in the release catalog. Older versions below that floor require an explicitly documented out-of-band upgrade path.
