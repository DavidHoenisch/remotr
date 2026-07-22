## Why

Remotr can currently accept and compose a release that no targeted endpoint can receive: production agents omit checked-in qualification evidence, mixed-distribution artifacts aggregate mutually irrelevant provider requirements, and a capability-blocked endpoint can be denied the very agent upgrade intended to unblock it. The Ubuntu Pro rollout exposed this as a silent delivery failure after local validation had passed, so capability truth, validation, and upgrade recovery must be repaired before expanding secret scope.

## What Changes

- Make production capability documents derive from a frozen, release-owned qualification catalog, including only exact Ubuntu Pro and core-provider rows whose required evidence passed.
- Make artifact requirements target-aware so an endpoint is evaluated only against requirements applicable to its normalized distribution, release, and architecture; an Ubuntu endpoint will not be blocked by an Arch/Pacman branch in the same fleet artifact.
- Establish an evidence-backed Ubuntu 26.04 amd64 support boundary for the core providers used by the engineering configuration. Qualified rows are advertised; unsupported rows are rejected by configuration validation before Git sync.
- Make local `config validate` and server Git-sync composition use the same provider, target, and capability validation rules, with source-specific safe diagnostics preserved through the Admin API and server logs.
- Replace synthetic agent-upgrade capability profiles with release-owned capability metadata and allow an explicitly requested, approved agent upgrade to reach a capability-blocked endpoint without treating the upgrade as proof of runtime provider support.
- Preserve the endpoint's last active artifact until the upgraded agent reports a valid current capability document and a compatible artifact is successfully acknowledged.
- Add regression, differential, compatibility, provider, and redaction evidence for the Ubuntu Pro rollout and blocked-upgrade paths.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `applicator-execution-contract`: Make variant requirements target-aware, define capability-blocked upgrade escape behavior, and require validation and Git-sync diagnostics to agree at their public seams.
- `linux-provider-conformance`: Require production capability catalogs and release metadata to be generated from checked-in passing qualification evidence rather than hand-wired or synthetic maps.
- `package-and-repository-management`: Define the exact Ubuntu 26.04 native-package qualification boundary and reject unsupported target/provider rows before release creation.
- `ubuntu-pro-management`: Require production agents to publish exact passing Ubuntu Pro capabilities and prove end-to-end delivery for a qualified Ubuntu 26.04 endpoint.

## Impact

- Agent capability generation and embedded release assets.
- Artifact composition, target normalization, requirement extraction, variant selection, and authenticated Sync responses.
- Agent release metadata and fleet-upgrade compatibility handling, including legacy-agent response compatibility.
- Operator configuration validation, server Git-sync validation, Admin API error responses, and server diagnostics.
- Ubuntu 26.04 provider qualification fixtures, capability matrices, public configuration fixtures, and release/build checks.
- No secret-scope contract changes are included; `add-global-secret-scope` remains the follow-on change.
