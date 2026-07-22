## Why

Remotr currently lets an old apply failure override a newer compliant State report, so operators can see the contradictory result `in_compliance: true` with `status: apply_failed`. Pop!_OS is also collapsed into the exact Debian distribution identity, which makes Debian-targeted configuration appear applicable while exact release qualification correctly withholds every provider capability.

## What Changes

- Classify compliance from the newest applicable evidence: a later State report supersedes an older apply failure while the historical failure remains available as endpoint history.
- Report capability-blocked delivery independently from active-artifact compliance in the Admin API and desktop presentation.
- Preserve exact Pop!_OS identity in endpoint capability facts while separately recording its Debian distribution family.
- Add explicit `PopOS` configuration targeting so authors deliberately include the derivative instead of inheriting every exact-Debian policy.
- Continue to exclude Ubuntu-only resources and Ubuntu Pro from Pop!_OS, and continue to fail closed for Pop!_OS provider rows without exact qualification evidence.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `desktop-fleet-visibility`: Separate active compliance, target delivery readiness, and historical apply failures in endpoint inventory and detail.
- `applicator-execution-contract`: Define exact Pop!_OS target predicates and their effect on artifact requirements and capability blocking.
- `linux-provider-conformance`: Preserve exact Pop!_OS identity independently from Debian-family compatibility without inheriting unqualified provider support.

## Impact

This affects State-report classification and Admin API output, desktop endpoint status presentation, operating-system fact normalization, configuration schema/validation/rendering, artifact target predicates, capability documents, documentation, and public-interface tests. Existing `targetDistros: [Debian]` remains exact Debian targeting; authors must include `PopOS` explicitly when derivative applicability is intended.
