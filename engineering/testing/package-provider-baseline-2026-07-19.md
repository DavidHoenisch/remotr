# Core package-provider baseline — 2026-07-19

This note freezes the observable baseline immediately before the
`complete-core-package-providers` implementation slices. The expectations are
independent test assertions at configuration, provider, provider-matrix, and
capability-document seams; they are not derived from future implementation.

## Executed baseline

Go 1.26.5 executed the following suite successfully:

```text
go test -mod=vendor \
  ./internal/applicators/packages/... \
  ./internal/applicators/aptrepositories \
  ./internal/applicators/aptkeys \
  ./internal/providermatrix \
  ./internal/providerregistry \
  ./internal/capabilitydoc \
  ./internal/configrepo
```

## Frozen expectations

### APT package boundary

- `TestApplicator_ConformsForPresenceAndRemoval` proves the existing public
  provider-contract presence and removal cycle.
- `TestApplicator_applyPurgedUsesNativePurge`,
  `TestApplicator_exactVersionConvergence`,
  `TestApplicator_blocksUnapprovedDowngrade`, and
  `TestApplicator_convergesAptHold` freeze the current lifecycle and policy
  behavior.
- `TestApplicator_applyInstallUsesSafeExactArgv` and
  `TestApplicator_reportsRebootWithoutRebooting` freeze exact argv and
  activation reporting.

### Arch package and `yay` boundaries

- `TestApplicator_exactPacmanVersionConvergence` freezes the current defect:
  the implementation is located under `packages/aur`, reports itself as
  Pacman, checks a repository version, and then uses unversioned
  `pacman -S --noconfirm <name>`.
- `TestApplicator_rejectsUnavailablePacmanVersion` freezes the current
  repository-version error boundary.
- `TestSelectPackageApplicator_rejectsYayWithoutAURProvider` freezes the
  temporary authoring-time `yay` rejection until a truthful provider and its
  pinned-Arch evidence are complete.

### APT repository and signing-key boundaries

- `TestApplicator_writesCanonicalNamedRepositoryAndPriority`,
  `TestApplicator_absentRemovesOnlyOwnedRepositoryFragments`, and
  `TestApplicator_credentialReferenceDoesNotLeakIntoSourceOrCheck` freeze the
  owned APT repository, absence, and redaction boundaries.
- `TestApplicator_rejectsFingerprintMismatchWithoutReplacingKeyring`,
  `TestApplicator_installsScopedKeyringUsingProtectedGPGInput`, and
  `TestApplicator_removesOnlyItsNamedKeyring` freeze mismatch, scoped
  activation, and narrow removal behavior.

### Matrix and capability advertisement

- `TestRepositoryMatrixTracksCoreProviderFamiliesOnSupportedDistributions`
  freezes Debian 12, Ubuntu 24.04, and Arch 2026-07-06 amd64 container rows.
  Package and repository rows are `untested` and use only the aggregate
  `make:provider-matrix-containers` selector at this baseline.
- `TestAdvertisedRequiresMatchingPassingEvidence` freezes the rule that an
  `untested` row is not advertised and that every support-key field must match.
- `TestDefaultRegistryResolvesNormalizedBackends` and
  `TestGeneratorDerivesRegisteredContractsAndCurrentFacts` freeze current
  discovery-based advertisement: Ubuntu/24.04/amd64 with APT emits the APT
  package provider, excludes Pacman, and excludes deferred DNF.

Later slices may intentionally replace individual expectations, but the red
failure and replacement evidence must identify the behavior changed from this
baseline.
