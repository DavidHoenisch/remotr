# TDD evidence log

## Task 2.1 — OS-AEC-080 restart recovery

- Public seam: system-safety recovery through `rollbackstore.New`, mutation
  admission, and `RecoverArmed`.
- Selected evidence: focused restart contract; corrupt-record and failed-provider
  negatives; rollback-store and network-state package regressions. Provider and
  Ubuntu VM interruption evidence remains owned by tasks 2.7 through 2.9 and
  scenario promotion remains blocked until task 5.6.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRecoversArmedTransactionAfterRestart -count=1`.
- Intended red observed: the test did not compile because `ErrArmedRecovery`,
  `Recovery`, and `RecoverArmed` did not exist.
- Green command: the same focused command passed after adding lifecycle version
  1, startup validation, per-resource blocking, recovery delivery, terminal
  transition, and payload cleanup.
- Negative evidence: the package suite proves corrupt legacy ciphertext blocks
  startup and a failed recovery callback remains armed, retryable, and absent
  from the safe returned diagnostic.

## Task 2.2 — OS-AEC-069 pre-mutation reservation

- Public seam: provider-contract reservation through `Store.Reserve` and the
  single-use `Reservation.Arm` handle.
- Selected evidence: focused configured-cap refusal, protected armed-record
  preservation, ciphertext/metadata/filesystem boundary cases, outstanding
  reservation accounting, and oversized-payload cleanup.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreReservesCompleteRecoveryBeforeMutation -count=1`.
- Intended red observed: the test did not compile because `AvailableBytes`,
  `ReservationRequest`, and `Store.Reserve` did not exist.
- Green command: the same focused command passed after reservation began
  accounting for AEAD overhead, encoded metadata, filesystem allowance,
  configured usage, available filesystem bytes, protected armed records, and
  concurrent outstanding reservations before `Arm` permits mutation.

## Task 2.3 — OS-AEC-081 deterministic retention

- Public seam: transaction retention through `Store.Cleanup` and the
  payload-free `Store.Records` metadata view.
- Selected evidence: exact attempt and successful-prior-state limits, exact
  age boundary, sensitive expiry, incomplete-attempt supersession, configured
  disk pressure, armed-record preservation, and a deterministic 0–20 attempt
  property loop under an injected clock.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRetentionPrunesAttemptsAndSuccessfulPayloads -count=1`.
- Intended red observed: the test did not compile because `Store.Cleanup` and
  `Store.Records` did not exist.
- Green command: the focused command passed after retention began keeping at
  most the configured attempt metadata and three successful non-secret prior
  payloads per resource.
- Boundary evidence: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRetention -count=1` passed for age, armed, sensitivity,
  supersession, disk, and bounded-count cases without wall-clock sleeps.

## Task 2.4 — OS-AEC-080 atomic transaction envelope

- Public seam: system-safety recovery through durable `Store.Save`, startup
  validation in `rollbackstore.New`, and `RecoverArmed` after an injected
  process stop.
- Selected evidence: deterministic injection after temporary-file creation,
  envelope write, envelope fsync, atomic activation, and directory fsync;
  authenticated metadata and ciphertext tamper negatives; legacy split-record
  migration; rollback-store and network-state package regressions.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreActivatesOneEnvelopeAtEveryDurabilityBoundary -count=1`.
- Intended red observed: the test did not compile because `DurabilityPoint`,
  its five durability-boundary constants, and `Options.CrashInjector` did not
  exist.
- Green command: the same focused command passed after payload and lifecycle
  metadata were sealed into one checksummed AEAD envelope, fsynced, atomically
  renamed, and followed by a parent-directory fsync. Stops before activation
  leave no recoverable transaction; stops after activation leave exactly one
  complete recoverable transaction.
- Negative and migration evidence: `TestEnvelopeAuthenticatesMetadataAndCiphertext`
  rejects modified headers and ciphertext at startup without leaking the
  payload, and `TestLegacySplitRecordCompatibilityFixture` migrates the frozen
  split-format fixture to one envelope while corrupt legacy ciphertext remains
  startup-blocking.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.

## Task 2.5 — OS-AEC-070 and OS-AEC-082 protected-key selection

- Public seam: protected-key provider selection through `KeyProvider`,
  `CapabilityKeyProvider`, the concrete `TPM2ToolsKeyProvider`, store startup,
  and the key-free `Store.Protection` report.
- Selected evidence: unsupported-TPM root fallback, selected-TPM failure and
  restart downgrade negatives, pre-marker upgrade compatibility, ambiguous
  provider-state refusal, versioned root-file persistence and raw-key migration,
  live TPM capability gating, exact TPM process argv, stdin-only sealing input,
  safe protection reports, and rollback/network-state package regressions.
  Real TPM VM and mutation evidence remains owned by tasks 5.2 and 5.6.
- First red command: `go test -mod=vendor ./internal/rollbackstore -run
  'TestStoreSelectsExplicitRootFallbackAndReportsReducedProtection|TestSelectedTPMFailureBlocksWithoutSilentRootDowngrade|TestRootKeyProviderPersistsVersionedRootOnlyMaterial'
  -count=1`.
- First intended red observed: the tests did not compile because `KeyMaterial`,
  `CapabilityKeyProvider`, protection classes, the blocking key-protection
  error, and `Store.Protection` did not exist.
- First green command: the same focused command passed after persisting the
  protection choice before provider use, adding a versioned root-only key
  envelope with legacy migration, binding transaction envelopes to a safe key
  identity, and exposing a key-free report with the explicit endpoint-root
  compromise limitation.
- Concrete-provider red command: `go test -mod=vendor ./internal/rollbackstore
  -run 'TestTPM2ToolsKeyProviderCapabilityGate|TestTPM2ToolsKeyProviderSealsAndReloadsWithoutArgumentLeakage|TestStoreReportsSelectedTPMClassWithoutKeyMaterial'
  -count=1`.
- Concrete-provider intended red observed: the tests did not compile because
  `NewTPM2ToolsKeyProvider` and `TPM2ToolsOptions` did not exist.
- Concrete-provider green command: the same focused command passed after the
  TPM2-tools provider began requiring a device, every required executable, and
  a live capability probe; sealing AES-256 key bytes through stdin; persisting
  only versioned TPM public/private blobs; unsealing on restart; flushing loaded
  contexts; and reporting only the safe TPM class and random key identity.
- Upgrade-boundary red command: `go test -mod=vendor
  ./internal/rollbackstore -run
  TestCapabilitySelectionPreservesPreexistingProtectionState -count=1`.
- Upgrade-boundary intended red observed: newly detected TPM capability won
  over a pre-marker root key, and ambiguous root-plus-TPM state reached the TPM
  provider instead of blocking.
- Upgrade-boundary green command: the same command passed after preexisting
  single-provider state became the initial durable selection and ambiguous
  state became startup-blocking.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.

## Task 2.6 — versioned key rotation and decrypt-only history

- Public seam: protected-key rotation through `Store.RotateKey`,
  `RotatingKeyProvider`, transaction-envelope key identities, startup
  validation, and retained-record `Store.Load`.
- Selected evidence: one armed recovery and one retained successful prior state
  encrypted before rotation, a newly armed recovery encrypted afterward,
  restart decryption through the historical identity, unavailable-history
  startup refusal, root keyring rotation, TPM-sealed keyring rotation, version 1
  migration for both provider files, and package regressions.
- Store red command: `go test -mod=vendor ./internal/rollbackstore -run
  'TestStoreKeyRotationRetainsHistoricalDecryptOnlyRecovery|TestStoreBlocksWhenReferencedHistoricalKeyIsUnavailable'
  -count=1`.
- Store intended red observed: the tests did not compile because
  `Store.RotateKey` did not exist.
- Store green command: the same focused command passed after rotation began
  binding legacy envelopes to the current identity, retaining the former key
  in a decrypt-only resolver, using only the new active identity for subsequent
  envelopes, and preserving the key-protection cause when missing history
  blocks restart recovery.
- Root-provider red command: `go test -mod=vendor ./internal/rollbackstore
  -run 'TestRootKeyProviderPersistsVersionedRootOnlyMaterial/rotation' -count=1`.
- Root-provider intended red observed: `RootKeyProvider.Rotate` and
  `RootKeyProvider.LoadByID` did not exist.
- Root-provider green command: the same focused command passed after the
  version 2 root-only keyring began naming one active key while retaining prior
  identities for decryption, including migration from raw and version 1 files.
- TPM-provider red command: `go test -mod=vendor ./internal/rollbackstore
  -run TestTPM2ToolsKeyProviderRotationRetainsSealedHistory -count=1`.
- TPM-provider intended red observed: `TPM2ToolsKeyProvider.Rotate` and
  `TPM2ToolsKeyProvider.LoadByID` did not exist.
- TPM-provider green command: the same focused command passed after the
  version 2 TPM keyring began retaining separately sealed historical blobs,
  selecting one active identity, and unsealing a requested old identity only
  for decryption.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.

## Task 2.7 — firewall and network-profile transaction handles

- Public seams: provider contract through firewall, NetworkManager, netplan,
  and systemd-networkd `ApplyResult`/`Check`; authenticated network transaction
  acknowledgement and timeout recovery through `networkstate`; destructive
  connectivity recovery through the isolated Vagrant VM safety fixture.
- Selected evidence: capacity refusal before mutation; one protected handle for
  every backend; process restart; protected checkpoint/snapshot use; plaintext
  state redirection resistance; legacy NetworkManager checkpoint migration;
  orphaned-state startup refusal; authenticated acknowledgement; timeout and
  explicit rollback; payload cleanup; second Check; exact protected nftables
  stdin; and real route, DNS, firewall, and interface loss/recovery.
- First red command: `go test -mod=vendor ./internal/agent/networkstate -run
  'TestStoreArmsNetworkManagerRecoveryHandleAcrossRestart|TestStoreRefusesNetworkTransactionWhenRecoveryReservationUnavailable'
  -count=1`.
- First intended red observed: the tests did not compile because
  `networkstate.Options.RollbackOptions` did not exist; before the implementation
  NetworkManager also had no transaction envelope and plaintext checkpoint
  state selected the rollback target.
- First green command: the same focused command passed after `Prepare` began
  reserving complete capacity and arming one encrypted versioned recovery
  handle for every backend before provider mutation. Restart and reconciliation
  now recover backend parameters and snapshots only from that handle, and
  acknowledgement or completed rollback drops its payload.
- Startup-coherence red command: `go test -mod=vendor
  ./internal/agent/networkstate -run
  'TestStoreBlocksOrphanedArmedRecoveryAfterStateLoss|TestStoreMigratesLegacyNetworkManagerIntentToProtectedHandle'
  -count=1`.
- Startup-coherence intended red observed: startup accepted an armed nftables
  recovery after its state file was removed, and legacy NetworkManager intent
  remained without a protected transaction envelope.
- Startup-coherence green command: the same command passed after startup began
  requiring a unique match between awaiting status and armed recovery, blocking
  orphaned or ambiguous state, and migrating one valid legacy NetworkManager
  checkpoint before reconciliation.
- Provider commands: `go test -mod=vendor ./internal/applicators/firewall -run
  TestApplicator_EnforcedNftablesArmsTimedRollback -count=1`, `go test
  -mod=vendor ./internal/applicators/networkmanager -run
  TestProfileApplyRestartAcknowledgementCleanupAndSecondCheck -count=1`, and
  `go test -mod=vendor ./internal/applicators/networkfiles -run
  TestFileBackedProfilesConvergeThroughArmedRollbackTransaction -count=1` all
  passed. Together they prove Apply, reconstructed-store restart, authenticated
  acknowledgement, timeout/explicit rollback, cleanup, and compliant or
  correctly drifted second Check at the provider seam.
- Relevant regression command: `go test -mod=vendor
  ./internal/agent/networkstate ./internal/applicators/firewall
  ./internal/applicators/networkmanager ./internal/applicators/networkfiles
  ./cmd/remotr-agent -count=1` passed.
- VM safety command: `mise exec go -- make
  provider-matrix-vm-network-recovery` passed against the disposable isolated
  Debian VM. The fixture observed failed control probes during real route, DNS,
  nftables, and interface disruption; restored every path; completed the
  authenticated acknowledgement; and destroyed the VM and libvirt network.

## Task 2.8 — file, download, certificate, and trust transaction handles

- Public seams: file, download, certificate, and trust-anchor provider
  `Apply`/`Rollback`/second-`Check` behavior through reconstructed providers;
  structured Apply failure reporting through `executor.ApplyState`; and the
  protected rollback-store reservation/envelope boundary.
- Selected evidence: reservation and arm before managed-state mutation;
  restart reconstruction; exact prior bytes, mode, and ownership metadata;
  absence of `.remotr.bak`; sensitive certificate/private-key encryption and
  24-hour retention admission; rollback cleanup; drifted second Check; honest
  `none` advertisement when no transaction handle is supplied; and separate
  original-Apply-error and rollback-outcome reporting.
- File red command: `go test -mod=vendor ./internal/applicators/files -run
  TestApplicatorProtectedRollbackSurvivesRestartWithoutAdjacentBackup
  -count=1`.
- File intended red observed: the test did not compile because
  `files.Applicator.ConfigureRollback` did not exist, and the provider still
  created `<path>.remotr.bak`.
- File green command: the same focused command passed after the file provider
  began arming a versioned path-bound snapshot through `rollbackstore.Handle`
  before removal, replacement, or metadata mutation and restoring it after
  provider reconstruction without an adjacent backup.
- Download red command: `go test -mod=vendor
  ./internal/applicators/downloads -run
  TestApplicator_ProtectedRollbackSurvivesRestartWithoutAdjacentBackup
  -count=1`.
- Download intended red observed: the test did not compile because
  `downloads.Applicator.ConfigureRollback` did not exist, and the provider
  still depended on `<destination>.remotr.bak`.
- Download green command: the same focused command passed after destination
  content, mode, and ownership became a protected attempt payload and Revert
  began resolving the armed handle after restart.
- Reservation regression red command: `go test -mod=vendor
  ./internal/rollbackstore -run
  TestReservationEstimateCoversTimestampEncodingGrowthBeforeArm -count=1`.
- Reservation intended red observed: a complete reserved payload was refused
  at Arm because fractional timestamp JSON grew between the estimate and the
  final envelope.
- Reservation green command: the same command passed after the complete-footprint
  estimate gained an explicit bound for variable-width metadata; repeated
  download package runs no longer failed intermittently.
- Certificate red command: `go test -mod=vendor
  ./internal/applicators/certificates -run
  TestApplicatorProtectedSensitiveRollbackSurvivesRestart -count=1`.
- Certificate intended red observed: the test did not compile because the
  certificate provider had no transaction-handle configuration and its
  certificate/private-key snapshot existed only in process memory.
- Certificate green command: the same focused command passed after the pair
  snapshot became a sensitive path-bound protected payload. Raw transaction
  files excluded the private-key canary, a reconstructed provider restored
  both files, and the second Check reported expected renewal drift.
- Trust-anchor red command: `go test -mod=vendor
  ./internal/applicators/trustanchors -run
  TestApplicatorProtectedRollbackSurvivesRestart -count=1`.
- Trust-anchor intended red observed: the test did not compile because the
  provider had no transaction handle and prior anchor state was process-local.
- Trust-anchor green command: the same focused command passed after the named
  anchor's bytes, mode, and ownership were protected and restart-restorable.
- Failure-reporting command: `go test -mod=vendor
  ./internal/applicators/files -run
  TestApplicatorReportsOriginalApplyErrorSeparatelyFromProtectedRollback
  -count=1` passed. A real procfs mutation refusal remained the primary Apply
  error while the armed recovery was independently reported as `reverted`.
- Relevant regression command: `go test -mod=vendor ./internal/applicators/...
  ./internal/agent/engine ./internal/resourceregistry
  ./internal/rollbackstore ./internal/executor -count=1` passed.
