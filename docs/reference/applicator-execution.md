# Applicator execution and safety reference

This page describes the common behavior shared by Remotr desired-state
resources. Use it when interpreting validation, endpoint reports, Apply
eligibility, rollback claims, or high-risk plans. Resource-specific fields and
provider limits remain in the [configuration format](configuration-format.md).

## Admission and identity

Canonical deployable artifacts declare `schemaVersion: 1`. Validation rejects
unknown resource kinds, unknown fields, invalid enum values, unsupported
field/provider combinations that are knowable at authoring time, duplicate
resource names, unknown dependencies, and dependency cycles.

Every resource has a stable `<configuration>/<resource-name>` address. Names
must be unique across all kinds in one configuration. `dependsOn` contains
complete resource addresses; when a dependency fails, is unsupported, or is
deferred, its dependents are blocked and are not applied.

An unversioned artifact is legacy schema 0 during the compatibility window.
Validation and discovery identify legacy input, and composition emits the
schema-1 equivalent only when the conversion preserves behavior. New
configuration should always use schema 1.

## Check outcomes and Apply eligibility

Each provider returns one Check status:

| Status | Meaning | Apply behavior |
| --- | --- | --- |
| `compliant` | Observed state matches every managed field. | No mutation. |
| `drifted` | The provider supports the intent but observed state differs. | Eligible only when policy, dependencies, authorization, and preflight permit it. |
| `unsupported` | The endpoint lacks the required provider capability. | Never applied as ordinary drift. |
| `check_failed` | State could not be observed safely. | Not applied. |
| `deferred` | Work must wait for a declared runtime condition. | Not applied in this cycle. |

Apply results distinguish `changed`, `no_change`, `deferred`, and `failed`.
The engine applies only `drifted` resources. Fleet remediation policy `report`
records drift without mutation; `auto` permits otherwise eligible work. A
successful Apply does not hide activation, reboot, rollback, or verification
outcomes.

Endpoint and Fleet reports retain separate `compliant`, `drifted`,
`unsupported`, `check_failed`, `deferred`, `apply_failed`, and `no_report`
buckets. See [endpoint management](../guides/endpoint-management.md#review-compliance-state)
and the [Admin API state-report schema](http-api.md#compliance-state-reports).

## Ownership, lifecycle, and ordering

Each resource declares a bounded ownership model: one named object, one
Remotr-owned fragment, or an authoritative set. Omitting a resource from a
later artifact does not delete prior endpoint state. Removal occurs only
through an explicit supported lifecycle or an authoritative-set contract.

`dependsOn` edges take precedence over default ordering. Resources that share
a native mutation boundary also share an exclusive lock domain—for example,
APT uses `package-manager:apt`, while Pacman packages, repositories, signing
trust, and AUR installation use `package-manager:pacman`. Lock waits are
bounded and cancellation is reported without starting a competing mutation.

Providers return activation as structured signals. The engine orders and
coalesces daemon reloads, reloads, restarts, logout requirements, next-boot
changes, trust-store refreshes, and reboot-required state instead of hiding
them inside a provider result.

## Risk and change control

Resource risk is one of `normal`, `sensitive`, `connectivity`, `access`,
`boot`, or `destructive`. High-risk kinds require their provider-specific
preflight and commonly require explicit `enforce: true`. A failed preflight
blocks activation without treating the resource as ordinary drift.

High-risk plans are derived from the composed registered resources. The
server computes canonical effective hashes, provider contract revisions,
dependencies, authorization groups, activation targets, predicted effects,
rollback classes, and baseline eligibility. Admin clients cannot supply an
authoritative desired-state hash or predicted effect.

Authorization binds exact hashes and a frozen target set. It never bypasses
schema or provider validation, current preflight, required rollback capacity,
redaction, or destructive identity safeguards. See [change control](../guides/change-control.md)
for the operator workflow.

## Rollback guarantees and storage

Every resource reports one rollback class:

| Class | Guarantee |
| --- | --- |
| `transactional` | The provider can restore its declared prior state after a failed mutation. |
| `best_effort` | Recovery is attempted but cannot be guaranteed for every failure. |
| `none` | No rollback is claimed; irreversible or event-like work must say so explicitly. |

The original Apply failure and rollback outcome are reported separately. A
rollback failure never replaces the mutation error.

Rollback-advertising providers reserve and arm recovery before mutation in the
root-owned agent transaction store. Records are keyed by resource address,
artifact digest, and attempt. The store retains at most 10 attempts per
resource and 30 days of metadata, at most three successful non-secret prior
states per resource and 30 days, and sensitive payloads only while armed or
unacknowledged with an absolute 24-hour limit. A global disk cap cannot prune
an armed recovery merely to admit new work.

Durable payloads use authenticated encryption under a versioned endpoint-local
key. A supported TPM provider may seal the key; otherwise Remotr reports the
reduced protection of a root-only key file. Startup validates armed records and
blocks rollback-requiring mutation when recovery state is corrupt, missing, or
cannot be decrypted.

## Classified output and effective hashes

Every accepted schema field is classified as `public`,
`sensitive-metadata`, or `secret`. Registration and repository validation fail
when a field has no classification. Generic reports, diagnostics, persistence,
backups, and rollback metadata accept only schema-approved safe projections;
they do not serialize arbitrary desired or observed resource objects.

Effective desired-state hashes are computed from normalized typed state,
defaults, provider identity and contract revision, and safe secret
provider/version identity. They exclude secret bytes, runtime observations,
timestamps, endpoint identity, and randomness. A change to an active secret
version changes the hash even when the secret bytes happen to be identical.

## Capability and support boundaries

Author-time validation rejects impossible platform/provider combinations.
Endpoint-local backend differences are determined from the current
authenticated capability document and can yield `unsupported` or
`capability_blocked`; Remotr never removes fields or resources to manufacture
compatibility. See [capability-compatible delivery](capability-compatible-delivery.md).

A registered kind or constructible provider is not by itself a support claim.
Published support requires exact passing traceability, provider-contract, and
real-environment evidence. The current release boundaries are listed in
[Ubuntu 24.04 applicator support](ubuntu-2404-applicator-support.md) and
[package provider qualification](../testing/package-provider-qualification.md).
