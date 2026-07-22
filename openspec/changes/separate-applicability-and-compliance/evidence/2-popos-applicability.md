# Exact Pop!_OS applicability evidence

## Red

- OS identity seam: `TestReadIdentityPreservesExactPopOSWithDebianFamilyWithoutExactUbuntuIdentity` failed to compile because `types.PopOS` did not exist.
- Capability-document seam: `TestGeneratorPreservesPopOSIdentityWithoutInheritingDebianQualification` failed with `capability document fact_value at facts.value` because `popos` was not an allowed exact distribution fact.
- Configuration seam: `TestValidateStateAcceptsCanonicalPopOSTargetForPortableResource` failed with `invalid targetDistro; use one of Ubuntu, Debian, or Arch`.
- Authenticated Sync seam: `TestAuthenticatedSyncProjectsExactPopOSTargetWithoutUbuntuOrDebianRequirements` failed artifact resolution with `invalid target distro` because requirement predicates rejected `popos`.

## Green

Fact discovery now returns exact `PopOS` with Debian family lineage. Capability generation emits `distro=popos` and `distro-family=debian` without inheriting a same-release Debian portable-provider row. Configuration validation/rendering preserves canonical `PopOS`. Authenticated Sync selects only PopOS and portable requirements while returning the complete canonical artifact containing every authored target branch.
